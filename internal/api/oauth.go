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
	"unicode"

	"golang.org/x/net/idna"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

// The optional MCP authorization server issues library grants through the
// existing owner session and grant store. Public clients register stateless,
// HMAC-signed client IDs and use authorization code + PKCE S256. Codes live in
// bounded memory for five minutes; the resulting grant lives until revoked in
// Settings → Connected agents. The separate MCP service verifies that grant
// through the core API on every request.

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
	maxOAuthCodes         = 1024
	maxOAuthCodesPerOwner = 16
	maxRedirectURIs       = 5
	maxRedirectURIRune    = 2048
	// Leave room for redirect, resource, state and cookies under the 64 KiB
	// HTTP header/request-line limit. Registration checks the encoded ID.
	maxOAuthClientIDBytes = 16 << 10
	maxOAuthURIBytes      = 8192
	maxOAuthStateBytes    = 4096
	maxOAuthScopeBytes    = 1024
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
	if len(id) > maxOAuthClientIDBytes {
		return c, false
	}
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
	if err1 != nil || err2 != nil ||
		base64.RawURLEncoding.EncodeToString(payload) != encoded ||
		base64.RawURLEncoding.EncodeToString(got) != sig {
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
	if !limitOAuthBody(w, r) {
		return
	}
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
	if strings.ContainsFunc(req.ClientName, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client_name must not contain control or format characters")
		return
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
	if len(id) > maxOAuthClientIDBytes {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "encoded client_id exceeds 16 KiB; shorten the client name or redirect URIs")
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
	if len(raw) > maxOAuthURIBytes || len([]rune(raw)) > maxRedirectURIRune {
		return errors.New("redirect URI too long")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return fmt.Errorf("redirect URI %q is not an absolute URI", raw)
	}
	if u.User != nil {
		return errors.New("redirect URI must not contain credentials")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return fmt.Errorf("redirect URI %q has a fragment", raw)
	}
	// Require the ASCII spelling rather than silently rewriting redirect URIs:
	// exact matching and the destination shown to the owner must agree. In
	// particular, IDNA mapping must not erase a format/control character.
	host := u.Hostname()
	for _, r := range host {
		if r > unicode.MaxASCII || unicode.IsControl(r) {
			return errors.New("redirect host must be ASCII; use a valid punycode hostname")
		}
	}
	if host != "" && net.ParseIP(host) == nil {
		if _, err := idna.Registration.ToASCII(strings.ToLower(host)); err != nil {
			return errors.New("redirect host is not a valid IDNA hostname")
		}
		// IDNA permits context-valid joiners. Even when encoded as punycode,
		// format/control characters are outside our redirect-host policy.
		decoded, err := idna.Registration.ToUnicode(strings.ToLower(host))
		if err != nil || strings.ContainsFunc(decoded, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
			return errors.New("redirect host must not contain control or format characters")
		}
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		if u.Hostname() == "" {
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
	if validRedirectURI(requested) != nil {
		return false
	}
	if registered == requested {
		return true
	}
	a, err1 := url.Parse(registered)
	b, err2 := url.Parse(requested)
	if err1 != nil || err2 != nil || a.Scheme != "http" || b.Scheme != "http" ||
		!isLoopbackHost(a.Hostname()) || a.Hostname() != b.Hostname() {
		return false
	}
	return a.EscapedPath() == b.EscapedPath() && a.RawQuery == b.RawQuery && a.ForceQuery == b.ForceQuery
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

// authzError is answered locally. Open registration does not make a redirect
// trustworthy before the person has explicitly approved or denied consent.
type authzError struct {
	code, description string
}

func (ro *Router) parseAuthzRequest(v url.Values) (*authzRequest, *authzError) {
	// Reject before rendering or redirecting, so errors cannot reflect oversized
	// state. Unknown fields are ignored and never retained by a pending code.
	for _, field := range []struct {
		name  string
		limit int
	}{
		{"redirect_uri", maxOAuthURIBytes}, {"resource", maxOAuthURIBytes},
		{"state", maxOAuthStateBytes}, {"scope", maxOAuthScopeBytes},
	} {
		if len(v.Get(field.name)) > field.limit {
			return nil, &authzError{"invalid_request", field.name + " is too long"}
		}
	}

	req := &authzRequest{ClientID: v.Get("client_id"), State: v.Get("state"),
		CodeChallenge: v.Get("code_challenge"), Resource: v.Get("resource")}
	c, ok := ro.readClientID(req.ClientID)
	if !ok {
		return nil, &authzError{"invalid_client", "unknown client; register at " + ro.cfg.AppOrigin + oauthRegisterPath}
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
		return nil, &authzError{"invalid_request", "redirect_uri is not one this client registered"}
	}
	switch {
	case v.Get("response_type") != "code":
		return req, &authzError{"unsupported_response_type", "response_type must be code"}
	case !validPKCEChallenge(req.CodeChallenge) || v.Get("code_challenge_method") != "S256":
		return req, &authzError{"invalid_request", "PKCE is required: send code_challenge with code_challenge_method=S256"}
	case len(v["resource"]) != 1 || req.Resource != ro.cfg.MCPURL:
		return req, &authzError{"invalid_target", "this server issues tokens only for " + ro.cfg.MCPURL}
	}
	return req, nil
}

// redirectWith sends the browser back to the client with params, plus the
// issuer (RFC 9207) so a client talking to several servers can tell whose
// answer this is.
func (ro *Router) redirectWith(w http.ResponseWriter, r *http.Request, req *authzRequest, params url.Values) {
	w.Header().Set("Cache-Control", "no-store")
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

func (ro *Router) answerAuthzError(w http.ResponseWriter, r *http.Request, e *authzError) {
	ro.renderConsent(w, r, http.StatusBadRequest, oauthConsentData{Error: e.description})
}

// oauthConsentData feeds the consent page. ClientName is chosen by whoever
// registered the client, so the page presents it as a claim, beside the
// redirect host the browser will actually be sent to.
type oauthConsentData struct {
	Clerk        *clerkView
	Header       appHeaderView
	Error        string
	ClientName   string
	RedirectHost string
	Request      *authzRequest
}

func (ro *Router) renderConsent(w http.ResponseWriter, r *http.Request, status int, data oauthConsentData) {
	data.Header = authPageHeader("Connect an agent")
	data.Clerk = ro.clerkView(r)
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
		ro.answerAuthzError(w, r, e)
		return
	}
	ro.showConsent(w, r, req)
}

func (ro *Router) showConsent(w http.ResponseWriter, r *http.Request, req *authzRequest) {
	data := oauthConsentData{ClientName: req.Client.Name, RedirectHost: redirectHost(req.RedirectURI), Request: req}
	status := http.StatusOK
	if !ro.mayManageAgentGrants(r) {
		data.Error = "Sign in to connect an agent to your Gallery."
		data.Request = nil
		status = http.StatusForbidden
	}
	ro.renderConsent(w, r, status, data)
}

// authorizeDecision is the consent form's submit. The session cookie is
// SameSite=Lax, so a cross-site page cannot submit it on the owner's behalf
// (security.md §1.4); the page cannot be framed, so it cannot be clicked
// through either.
func (ro *Router) authorizeDecision(w http.ResponseWriter, r *http.Request) {
	if !limitOAuthBody(w, r) {
		return
	}
	// SameSite cookies protect logged-in instances; explicit origin checks also
	// protect no-login deployments and same-site sibling origins.
	if origins := r.Header.Values("Origin"); len(origins) > 0 {
		if len(origins) != 1 || origins[0] != ro.cfg.AppOrigin {
			http.Error(w, "cross-origin consent is not accepted", http.StatusForbidden)
			return
		}
	} else if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
		http.Error(w, "cross-origin consent is not accepted", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", bodyReadStatus(err))
		return
	}
	req, e := ro.parseAuthzRequest(r.PostForm)
	if e != nil {
		ro.answerAuthzError(w, r, e)
		return
	}
	if !ro.mayManageAgentGrants(r) {
		ro.showConsent(w, r, req)
		return
	}
	switch r.PostForm.Get("decision") {
	case "deny":
		ro.redirectWith(w, r, req, url.Values{"error": {"access_denied"}, "error_description": {"the owner declined"}})
		return
	case "allow":
	default:
		ro.renderConsent(w, r, http.StatusBadRequest, oauthConsentData{Error: "Choose Approve or Deny."})
		return
	}
	ownerID := ownerIDFromCtx(r.Context())
	code, err := ro.oauthCodes.issue(oauthCode{
		clientID: req.ClientID, label: req.Client.Name, redirectURI: req.RedirectURI,
		challenge: req.CodeChallenge, ownerID: ownerID, resource: req.Resource,
		sessionOwner: principalFromCtx(r.Context()).Kind == PrincipalSession,
	})
	if errors.Is(err, errOAuthOwnerCodeLimit) {
		ro.renderConsent(w, r, http.StatusTooManyRequests, oauthConsentData{Error: "Too many pending authorizations for your account. Complete a pending connection or wait five minutes and try again."})
		return
	}
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
// an agent grant over the approving owner's library.
func (ro *Router) exchangeOAuthCode(w http.ResponseWriter, r *http.Request) {
	if !limitOAuthBody(w, r) {
		return
	}
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
	if _, valid := ro.readClientID(clientID); !valid {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "client_id is invalid")
		return
	}
	switch {
	case clientID != code.clientID:
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "the code was issued to another client")
		return
	case f.Get("redirect_uri") != code.redirectURI:
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	case !pkceVerifies(f.Get("code_verifier"), code.challenge):
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match the code_challenge")
		return
	case len(f["resource"]) != 1 || f.Get("resource") != code.resource || code.resource != ro.cfg.MCPURL:
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "this server issues tokens only for "+ro.cfg.MCPURL)
		return
	}
	token, digest, err := agentscope.NewPersistedToken()
	if err != nil {
		serverError(w, r, "mint oauth agent grant", err)
		return
	}
	createGrant := ro.cfg.Store.CreateAgentGrant
	if code.sessionOwner {
		// Account deletion may have happened since approval. The store checks the
		// live owner in the insertion, so a pending code cannot recreate an orphan
		// grant after account cleanup. No-login instances need no users row.
		createGrant = ro.cfg.Store.CreateAgentGrantForUser
	}
	if _, err := createGrant(r.Context(), code.ownerID,
		normalizeAgentGrantLabel(code.label), code.label, digest); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "the owner is no longer available")
			return
		}
		serverError(w, r, "create oauth agent grant", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: token, TokenType: "Bearer"})
}

// An S256 challenge is exactly one SHA-256 digest in unpadded base64url.
func validPKCEChallenge(challenge string) bool {
	if len(challenge) != 43 {
		return false
	}
	digest, err := base64.RawURLEncoding.Strict().DecodeString(challenge)
	return err == nil && len(digest) == sha256.Size
}

// pkceVerifies checks an S256 code verifier (RFC 7636 §4.6).
func pkceVerifies(verifier, challenge string) bool {
	if n := len(verifier); n < 43 || n > 128 {
		return false
	}
	for _, c := range verifier {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", c)) {
			return false
		}
	}
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

// --- codes -------------------------------------------------------------------

type oauthCode struct {
	clientID, label, redirectURI, challenge string
	ownerID                                 int64
	resource                                string
	sessionOwner                            bool
	expires                                 time.Time
}

var errOAuthOwnerCodeLimit = errors.New("owner has too many pending authorization codes")

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
	pendingForOwner := 0
	for k, v := range s.codes {
		if !now.Before(v.expires) {
			delete(s.codes, k)
		} else if v.ownerID == c.ownerID {
			pendingForOwner++
		}
	}
	if pendingForOwner >= maxOAuthCodesPerOwner {
		return "", errOAuthOwnerCodeLimit
	}
	if len(s.codes) >= maxOAuthCodes {
		return "", errors.New("too many authorization codes awaiting exchange")
	}
	// Parsed form values can be slices of a much larger request allocation.
	// The store owns only the validated string bytes, never their backing form.
	c.clientID = strings.Clone(c.clientID)
	c.label = strings.Clone(c.label)
	c.redirectURI = strings.Clone(c.redirectURI)
	c.challenge = strings.Clone(c.challenge)
	c.resource = strings.Clone(c.resource)
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
	if !ok || !s.now().Before(c.expires) {
		return oauthCode{}, false
	}
	return c, true
}
