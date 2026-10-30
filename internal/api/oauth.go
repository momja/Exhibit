package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

// A minimal OAuth 2.1 authorization server for MCP clients (Exh-rhlq), so a
// client that insists on OAuth needs no pasted token. It is the whole of
// what such a client requires and nothing more:
//
//   - dynamic client registration (RFC 7591), open, because an MCP client
//     arrives with no prior relationship to this instance;
//   - the authorization code grant with PKCE (S256 only), for public
//     clients, no client secrets;
//   - a consent page on which the signed-in owner picks the one artifact the
//     client may edit.
//
// The access token it issues IS an owner-issued agent grant (Exh-l2xj): one
// artifact, listed on that artifact's page beside the pasted ones, revoked
// there, resolved by the same verifier. It has no expiry and there is no
// refresh token, because a grant lives until its owner revokes it; a client
// whose token is revoked meets a 401 and starts the flow again. Nothing here
// authorizes a *user*, so no token from this server is ever library-wide.
//
// It authenticates the person with the existing session, whichever login put
// it there (local, OIDC or hosted Clerk), so one implementation serves every
// deployment and no identity provider has to support per-resource scoping.
//
// What it keeps:
//
//   - Nothing for clients. A client id is the client's registration (its
//     name and redirect URIs), signed with a key derived from the server
//     secret, so open registration cannot fill a table and nothing about a
//     client outlives the grants it was given.
//   - Authorization codes in memory, single-use and minutes-lived, like the
//     SSE tickets: a restart drops them and the client asks again.

const (
	authServerMetadataPath = "/.well-known/oauth-authorization-server"
	oauthAuthorizePath     = "/oauth/authorize"
	oauthTokenPath         = "/oauth/token"
	oauthRegisterPath      = "/oauth/register"

	oauthClientIDPrefix = "exclient_"
	oauthClientKeyUse   = "oauth-client-id"
	oauthCodeTTL        = 5 * time.Minute
	// maxOAuthCodes bounds the codes waiting to be exchanged. Only a signed-in
	// owner's consent mints one, so this is a backstop, not a rate limit.
	maxOAuthCodes      = 1024
	maxRedirectURIs    = 5
	maxRedirectURIRune = 2048
	// consentArtifactLimit is how many of the owner's artifacts the consent
	// page lists, newest first.
	consentArtifactLimit = 200
)

// oauthClient is a registration: all a client is, here.
type oauthClient struct {
	Name         string   `json:"n"`
	RedirectURIs []string `json:"r"`
}

func oauthClientKey(box *secrets.Box) [32]byte {
	if box == nil {
		var k [32]byte
		_, _ = rand.Read(k[:])
		return k
	}
	return box.DeriveKey(oauthClientKeyUse)
}

func (ro *Router) mintClientID(c oauthClient) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, ro.oauthClientKey[:])
	mac.Write(payload)
	return oauthClientIDPrefix + base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (ro *Router) readClientID(id string) (oauthClient, bool) {
	var c oauthClient
	rest, ok := strings.CutPrefix(id, oauthClientIDPrefix)
	if !ok {
		return c, false
	}
	encoded, sig, ok := strings.Cut(rest, ".")
	if !ok {
		return c, false
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(encoded)
	got, err2 := base64.RawURLEncoding.DecodeString(sig)
	if err1 != nil || err2 != nil {
		return c, false
	}
	mac := hmac.New(sha256.New, ro.oauthClientKey[:])
	mac.Write(payload)
	if !hmac.Equal(got, mac.Sum(nil)) || json.Unmarshal(payload, &c) != nil {
		return c, false
	}
	return c, true
}

// --- discovery and registration ------------------------------------------------

type authServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	AuthorizationResponseISSSupported bool     `json:"authorization_response_iss_parameter_supported"`
}

func (ro *Router) authServerMetadata(w http.ResponseWriter, r *http.Request) {
	o := ro.cfg.AppOrigin
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, authServerMetadata{
		Issuer:                            o,
		AuthorizationEndpoint:             o + oauthAuthorizePath,
		TokenEndpoint:                     o + oauthTokenPath,
		RegistrationEndpoint:              o + oauthRegisterPath,
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		AuthorizationResponseISSSupported: true,
	})
}

type registrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

func writeOAuthError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

// registerOAuthClient is open dynamic client registration (RFC 7591). It
// records nothing: the response's client id is the registration.
func (ro *Router) registerOAuthClient(w http.ResponseWriter, r *http.Request) {
	var req registrationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TokenEndpointAuthMethod != "" && req.TokenEndpointAuthMethod != "none" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata",
			"only public clients are supported: token_endpoint_auth_method must be none")
		return
	}
	if len(req.RedirectURIs) == 0 || len(req.RedirectURIs) > maxRedirectURIs {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri",
			fmt.Sprintf("register between 1 and %d redirect URIs", maxRedirectURIs))
		return
	}
	for _, u := range req.RedirectURIs {
		if err := validRedirectURI(u); err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
			return
		}
	}
	name := strings.Join(strings.Fields(req.ClientName), " ")
	if r := []rune(name); len(r) > agentGrantLabelMax {
		name = string(r[:agentGrantLabelMax])
	}
	if name == "" {
		name = "MCP client"
	}
	c := oauthClient{Name: name, RedirectURIs: req.RedirectURIs}
	id, err := ro.mintClientID(c)
	if err != nil {
		serverError(w, r, "mint oauth client id", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, registrationResponse{
		ClientID: id, ClientIDIssuedAt: time.Now().Unix(), ClientName: name, RedirectURIs: c.RedirectURIs,
		GrantTypes: []string{"authorization_code"}, ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none",
	})
}

// validRedirectURI admits what OAuth 2.1 allows a public client: an https
// URL, an http URL on the loopback interface (RFC 8252 §7.3), or a native
// app's private-use scheme (§7.1). Never a fragment, and never a scheme a
// browser would execute or read locally.
func validRedirectURI(raw string) error {
	if len([]rune(raw)) > maxRedirectURIRune {
		return errors.New("redirect URI too long")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return fmt.Errorf("redirect URI %q is not an absolute URI", raw)
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return fmt.Errorf("redirect URI %q has a fragment", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		if u.Host == "" {
			return fmt.Errorf("redirect URI %q has no host", raw)
		}
		return nil
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("redirect URI %q: plain http is allowed only on the loopback interface", raw)
		}
		return nil
	case "javascript", "data", "file", "vbscript", "about", "blob", "filesystem":
		return fmt.Errorf("redirect URI %q uses a scheme that is not allowed", raw)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redirectURIMatches compares a requested redirect URI to a registered one:
// exactly, except that a loopback http URI may use any port (RFC 8252 §7.3),
// since a native client picks a free one at run time.
func redirectURIMatches(registered, requested string) bool {
	if registered == requested {
		return true
	}
	a, err1 := url.Parse(registered)
	b, err2 := url.Parse(requested)
	if err1 != nil || err2 != nil || a.Scheme != "http" || b.Scheme != "http" ||
		!isLoopbackHost(a.Hostname()) || a.Hostname() != b.Hostname() {
		return false
	}
	return a.Path == b.Path && a.RawQuery == b.RawQuery
}

// --- authorization ------------------------------------------------------------

// authzRequest is a validated authorization request, carried through the
// consent page in hidden fields and re-validated when the form comes back.
type authzRequest struct {
	ClientID      string
	Client        oauthClient
	RedirectURI   string
	State         string
	CodeChallenge string
	Resource      string
}

// authzError is a refused authorization request. redirect says whether the
// refusal may go back to the client: not when the client or its redirect URI
// is what failed, because then the URI is not one to trust with anything.
type authzError struct {
	code, description string
	redirect          bool
}

func (ro *Router) parseAuthzRequest(v url.Values) (*authzRequest, *authzError) {
	req := &authzRequest{ClientID: v.Get("client_id"), State: v.Get("state"),
		CodeChallenge: v.Get("code_challenge"), Resource: v.Get("resource")}
	c, ok := ro.readClientID(req.ClientID)
	if !ok {
		return nil, &authzError{"invalid_client", "unknown client; register at " + ro.cfg.AppOrigin + oauthRegisterPath, false}
	}
	req.Client = c
	req.RedirectURI = v.Get("redirect_uri")
	if req.RedirectURI == "" && len(c.RedirectURIs) == 1 {
		req.RedirectURI = c.RedirectURIs[0]
	}
	registered := false
	for _, u := range c.RedirectURIs {
		if redirectURIMatches(u, req.RedirectURI) {
			registered = true
			break
		}
	}
	if !registered {
		return nil, &authzError{"invalid_request", "redirect_uri is not one this client registered", false}
	}
	switch {
	case v.Get("response_type") != "code":
		return req, &authzError{"unsupported_response_type", "response_type must be code", true}
	case req.CodeChallenge == "" || v.Get("code_challenge_method") != "S256":
		return req, &authzError{"invalid_request", "PKCE is required: send code_challenge with code_challenge_method=S256", true}
	case req.Resource != "" && req.Resource != ro.mcpResource():
		return req, &authzError{"invalid_target", "this server issues tokens only for " + ro.mcpResource(), true}
	}
	return req, nil
}

// redirectWith sends the browser back to the client with params, plus the
// issuer (RFC 9207) so a client talking to several servers can tell whose
// answer this is.
func (ro *Router) redirectWith(w http.ResponseWriter, r *http.Request, req *authzRequest, params url.Values) {
	if req.State != "" {
		params.Set("state", req.State)
	}
	params.Set("iss", ro.cfg.AppOrigin)
	sep := "?"
	if strings.Contains(req.RedirectURI, "?") {
		sep = "&"
	}
	http.Redirect(w, r, req.RedirectURI+sep+params.Encode(), http.StatusFound)
}

func (ro *Router) answerAuthzError(w http.ResponseWriter, r *http.Request, req *authzRequest, e *authzError) {
	if e.redirect {
		ro.redirectWith(w, r, req, url.Values{"error": {e.code}, "error_description": {e.description}})
		return
	}
	ro.renderConsent(w, r, http.StatusBadRequest, oauthConsentData{Error: e.description})
}

// oauthConsentData feeds the consent page. ClientName is chosen by whoever
// registered the client, so the page presents it as a claim, beside the
// redirect host the browser will actually be sent to.
type oauthConsentData struct {
	Header       appHeaderView
	Error        string
	ClientName   string
	RedirectHost string
	Artifacts    []oauthConsentArtifact
	Request      *authzRequest
	// Selected is the artifact chosen on a submit that has to be shown again.
	Selected string
}

type oauthConsentArtifact struct {
	ID, Title string
}

func (ro *Router) renderConsent(w http.ResponseWriter, r *http.Request, status int, data oauthConsentData) {
	data.Header = authPageHeader("Connect an agent")
	page, err := renderPage("oauthConsent", data)
	if err != nil {
		serverError(w, r, "render oauth consent", err)
		return
	}
	// The consent page grants authority with one click, so it must never be
	// framed under somebody else's page.
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, page)
}

func redirectHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return u.Scheme + ":"
}

// authorizePage is the consent page. sessionGate has already sent a visitor
// without a session to sign in and back here.
func (ro *Router) authorizePage(w http.ResponseWriter, r *http.Request) {
	req, e := ro.parseAuthzRequest(r.URL.Query())
	if e != nil {
		ro.answerAuthzError(w, r, req, e)
		return
	}
	ro.showConsent(w, r, http.StatusOK, req, "", "")
}

func (ro *Router) showConsent(w http.ResponseWriter, r *http.Request, status int, req *authzRequest, selected, problem string) {
	data := oauthConsentData{ClientName: req.Client.Name, RedirectHost: redirectHost(req.RedirectURI),
		Request: req, Selected: selected, Error: problem}
	if !ro.mayIssueAgentGrant(r) {
		data.Error = "Sign in to connect an agent to your library."
		ro.renderConsent(w, r, http.StatusForbidden, data)
		return
	}
	artifacts, err := ro.cfg.Store.ListArtifacts(r.Context(), store.ListOptions{
		OwnerID: ownerIDFromCtx(r.Context()), Limit: consentArtifactLimit})
	if err != nil {
		serverError(w, r, "list artifacts for oauth consent", err)
		return
	}
	for _, a := range artifacts {
		data.Artifacts = append(data.Artifacts, oauthConsentArtifact{ID: a.ID, Title: a.Title})
	}
	ro.renderConsent(w, r, status, data)
}

// authorizeDecision is the consent form's submit. The session cookie is
// SameSite=Lax, so a cross-site page cannot submit it on the owner's behalf
// (security.md §1.4); the page cannot be framed, so it cannot be clicked
// through either.
func (ro *Router) authorizeDecision(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", bodyReadStatus(err))
		return
	}
	req, e := ro.parseAuthzRequest(r.PostForm)
	if e != nil {
		ro.answerAuthzError(w, r, req, e)
		return
	}
	if r.PostForm.Get("decision") != "allow" {
		ro.redirectWith(w, r, req, url.Values{"error": {"access_denied"}, "error_description": {"the owner declined"}})
		return
	}
	if !ro.mayIssueAgentGrant(r) {
		ro.showConsent(w, r, http.StatusForbidden, req, "", "")
		return
	}
	ownerID := ownerIDFromCtx(r.Context())
	artifactID := r.PostForm.Get("artifact_id")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, artifactID)
	if err != nil {
		serverError(w, r, "oauth consent artifact lookup", err)
		return
	}
	if a == nil {
		ro.showConsent(w, r, http.StatusBadRequest, req, "", "Choose one of your artifacts for this agent to edit.")
		return
	}
	code, err := ro.oauthCodes.issue(oauthCode{
		clientID: req.ClientID, label: req.Client.Name, redirectURI: req.RedirectURI,
		challenge: req.CodeChallenge, ownerID: ownerID, artifactID: a.ID,
	})
	if err != nil {
		serverError(w, r, "issue oauth code", err)
		return
	}
	ro.redirectWith(w, r, req, url.Values{"code": {code}})
}

// --- token -------------------------------------------------------------------

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// exchangeOAuthCode is the token endpoint: a code and its PKCE verifier for
// an agent grant over the artifact the owner chose.
func (ro *Router) exchangeOAuthCode(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeBodyTooLarge(w, tooLarge.Limit)
			return
		}
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "could not read the form")
		return
	}
	f := r.PostForm
	if f.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	code, ok := ro.oauthCodes.redeem(f.Get("code"))
	if !ok {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "the code is unknown, used or expired")
		return
	}
	// A public client names itself in the form, or, as some OAuth libraries
	// do by default, as the user of a Basic header with an empty secret.
	clientID := f.Get("client_id")
	if user, _, ok := r.BasicAuth(); ok && clientID == "" {
		clientID, _ = url.QueryUnescape(user)
	}
	switch {
	case clientID != code.clientID:
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "the code was issued to another client")
		return
	case f.Get("redirect_uri") != "" && f.Get("redirect_uri") != code.redirectURI:
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	case !pkceVerifies(f.Get("code_verifier"), code.challenge):
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match the code_challenge")
		return
	case f.Get("resource") != "" && f.Get("resource") != ro.mcpResource():
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "this server issues tokens only for "+ro.mcpResource())
		return
	}
	token, digest, err := agentscope.NewPersistedToken()
	if err != nil {
		serverError(w, r, "mint oauth agent grant", err)
		return
	}
	if _, err := ro.cfg.Store.CreateAgentGrant(r.Context(), code.ownerID, code.artifactID,
		normalizeAgentGrantLabel(code.label), digest); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "the artifact no longer exists")
			return
		}
		serverError(w, r, "create oauth agent grant", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: token, TokenType: "Bearer"})
}

// pkceVerifies checks an S256 code verifier (RFC 7636 §4.6).
func pkceVerifies(verifier, challenge string) bool {
	if n := len(verifier); n < 43 || n > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

// --- codes -------------------------------------------------------------------

type oauthCode struct {
	clientID, label, redirectURI, challenge string
	ownerID                                 int64
	artifactID                              string
	expires                                 time.Time
}

type oauthCodeStore struct {
	now   func() time.Time
	mu    sync.Mutex
	codes map[string]oauthCode
}

func newOAuthCodeStore() *oauthCodeStore {
	return &oauthCodeStore{now: time.Now, codes: map[string]oauthCode{}}
}

func (s *oauthCodeStore) issue(c oauthCode) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	value := base64.RawURLEncoding.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, v := range s.codes {
		if now.After(v.expires) {
			delete(s.codes, k)
		}
	}
	if len(s.codes) >= maxOAuthCodes {
		return "", errors.New("too many authorization codes awaiting exchange")
	}
	c.expires = now.Add(oauthCodeTTL)
	s.codes[value] = c
	return value, nil
}

// redeem spends a code: found or not, it cannot be used again.
func (s *oauthCodeStore) redeem(value string) (oauthCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[value]
	delete(s.codes, value)
	if !ok || s.now().After(c.expires) {
		return oauthCode{}, false
	}
	return c, true
}
