package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/momja/Exhibit/internal/mcpserver"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The built-in authorization server for MCP clients (Exh-rhlq).

// oauthInstance is a two-owner instance with a login, served over HTTP at the
// origin it believes is its own, which OAuth discovery checks.
type oauthInstance struct {
	twoOwnerInstance
	srv       *httptest.Server
	service   *httptest.Server
	cookieOne *http.Cookie
}

func newOAuthInstance(t *testing.T) oauthInstance {
	t.Helper()
	in := newTwoOwnerInstance(t)
	srv := httptest.NewUnstartedServer(nil)
	t.Cleanup(srv.Close)
	service := httptest.NewUnstartedServer(nil)
	t.Cleanup(service.Close)
	cfg := in.ro.cfg
	cfg.AppOrigin = "http://" + srv.Listener.Addr().String()
	cfg.MCPURL = "http://" + service.Listener.Addr().String() + "/mcp"
	in.ro = NewRouter(cfg)
	srv.Config.Handler = in.ro
	serviceHandler, err := mcpserver.NewHTTPHandler(mcpserver.HTTPConfig{BaseURL: cfg.AppOrigin, ExhibitPublicURL: cfg.AppOrigin, PublicURL: cfg.MCPURL})
	require.NoError(t, err)
	service.Config.Handler = serviceHandler
	srv.Start()
	service.Start()
	return oauthInstance{twoOwnerInstance: in, srv: srv, service: service,
		cookieOne: sessionCookieFor(t, cfg.Store, in.ownerOne, "session-oauth-one")}
}

const testRedirect = "http://127.0.0.1:4567/callback"

func (in oauthInstance) register(t *testing.T, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(in.srv.URL+"/oauth/register", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func (in oauthInstance) clientID(t *testing.T) string {
	t.Helper()
	resp, out := in.register(t, `{"client_name":"Test Agent","redirect_uris":["`+testRedirect+`"],"token_endpoint_auth_method":"none"}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, out)
	return out["client_id"].(string)
}

func pkcePair() (verifier, challenge string) {
	verifier = strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func (in oauthInstance) authorizeQuery(clientID, challenge string) url.Values {
	return url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {testRedirect},
		"resource": {in.ro.cfg.MCPURL}, "state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
}

// noRedirect is an HTTP client that reports a redirect instead of following it.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (in oauthInstance) consent(t *testing.T, cookie *http.Cookie, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", in.srv.URL+"/oauth/authorize", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := noRedirect.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (in oauthInstance) exchange(t *testing.T, form url.Values) (int, map[string]any) {
	t.Helper()
	resp, err := http.PostForm(in.srv.URL+"/oauth/token", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return resp.StatusCode, out
}

func TestOAuthDiscoveryNamesThisInstance(t *testing.T) {
	in := newOAuthInstance(t)
	resp, err := http.Get(in.service.URL + "/.well-known/oauth-protected-resource/mcp")
	require.NoError(t, err)
	var prm map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&prm))
	resp.Body.Close()
	assert.Equal(t, []any{in.srv.URL}, prm["authorization_servers"])

	resp, err = http.Get(in.srv.URL + "/.well-known/oauth-authorization-server")
	require.NoError(t, err)
	var meta map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&meta))
	resp.Body.Close()
	assert.Equal(t, in.srv.URL, meta["issuer"])
	assert.Equal(t, in.srv.URL+"/oauth/token", meta["token_endpoint"])
	assert.Equal(t, in.srv.URL+"/oauth/authorize", meta["authorization_endpoint"])
	assert.Equal(t, in.srv.URL+"/oauth/register", meta["registration_endpoint"])
	assert.Equal(t, []any{"none"}, meta["token_endpoint_auth_methods_supported"])
	assert.Equal(t, []any{"authorization_code"}, meta["grant_types_supported"])
	assert.Equal(t, []any{"code"}, meta["response_types_supported"])
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, []any{"S256"}, meta["code_challenge_methods_supported"])
	assert.Equal(t, true, meta["authorization_response_iss_parameter_supported"])
}

func TestOAuthAuthorizationCodeFlowIssuesALibraryGrant(t *testing.T) {
	in := newOAuthInstance(t)
	clientID := in.clientID(t)
	verifier, challenge := pkcePair()
	q := in.authorizeQuery(clientID, challenge)

	// No session: the gate sends the visitor to sign in and back.
	resp, err := noRedirect.Get(in.srv.URL + "/oauth/authorize?" + q.Encode())
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/auth/login?next="), resp.Header.Get("Location"))

	// The owner sees the client and its redirect destination, with the full
	// authority and revocation policy in plain language and no artifact picker.
	req, _ := http.NewRequest("GET", in.srv.URL+"/oauth/authorize?"+q.Encode(), nil)
	req.AddCookie(in.cookieOne)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	page := readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, page)
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Contains(t, page, "Test Agent")
	assert.Contains(t, page, "http://127.0.0.1:4567")
	assert.Contains(t, strings.ToLower(page), "whole library")
	assert.Contains(t, page, "Connected agents")
	assert.NotContains(t, page, `name="artifact_id"`)
	assert.NotContains(t, page, ownerTwoTitle)
	assert.NotContains(t, page, in.ro.cfg.AuthToken)
	form := q
	form.Set("decision", "allow")
	resp = in.consent(t, in.cookieOne, form)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	back, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:4567", back.Host)
	assert.Equal(t, "xyz", back.Query().Get("state"))
	assert.Equal(t, in.srv.URL, back.Query().Get("iss"))
	code := back.Query().Get("code")
	require.NotEmpty(t, code)

	exchange := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID},
		"redirect_uri": {testRedirect}, "resource": {in.ro.cfg.MCPURL}, "code_verifier": {strings.Repeat("w", 50)}}
	status, out := in.exchange(t, exchange)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", out["error"], "a wrong verifier spends the code")
	exchange.Set("code_verifier", verifier)
	status, _ = in.exchange(t, exchange)
	assert.Equal(t, http.StatusBadRequest, status, "a code is single-use")

	// A fresh code, exchanged properly.
	resp = in.consent(t, in.cookieOne, form)
	back, _ = url.Parse(resp.Header.Get("Location"))
	exchange.Set("code", back.Query().Get("code"))
	status, out = in.exchange(t, exchange)
	require.Equal(t, http.StatusOK, status, out)
	assert.Equal(t, "Bearer", out["token_type"])
	assert.Nil(t, out["refresh_token"])
	assert.Nil(t, out["expires_in"], "a grant lives until it is revoked")
	token := out["access_token"].(string)

	// The token is listed under its client name, scoped to the approving owner.
	w := doWithCookie(t, in.ro, "GET", "/api/agent-grants", in.cookieOne, nil)
	assert.Contains(t, w.Body.String(), `"label":"Test Agent"`)
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactOne, token, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactTwo, token, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestOAuthRefusals(t *testing.T) {
	in := newOAuthInstance(t)

	for _, uri := range []string{"javascript:alert(1)", "http://evil.example/cb", "https://ok.example/cb#frag", "/relative", "https://user:secret@ok.example/cb", "http://user@127.0.0.1/callback", "file:///tmp/token", "data:text/html,unsafe"} {
		resp, out := in.register(t, `{"redirect_uris":["`+uri+`"]}`)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, uri)
		assert.Equal(t, "invalid_redirect_uri", out["error"], uri)
	}
	for _, body := range []string{`{}`, `{"redirect_uris":[]}`, `{"redirect_uris":["https://ok.example/cb","https://ok.example/cb","https://ok.example/cb","https://ok.example/cb","https://ok.example/cb","https://ok.example/cb"]}`} {
		response, out := in.register(t, body)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, "invalid_redirect_uri", out["error"])
	}
	resp, _ := in.register(t, `{"redirect_uris":["https://ok.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, out := in.register(t, `{"redirect_uris":["cursor://anysphere.cursor-retrieval/oauth/callback"]}`)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a native app's private-use scheme")
	assert.Equal(t, "MCP client", out["client_name"])
	assert.Equal(t, "none", out["token_endpoint_auth_method"])
	assert.Equal(t, []any{"authorization_code"}, out["grant_types"])
	assert.Equal(t, []any{"code"}, out["response_types"])
	assert.NotContains(t, out, "client_secret")

	clientID := in.clientID(t)
	_, challenge := pkcePair()
	get := func(q url.Values) *http.Response {
		req, _ := http.NewRequest("GET", in.srv.URL+"/oauth/authorize?"+q.Encode(), nil)
		req.AddCookie(in.cookieOne)
		resp, err := noRedirect.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		return resp
	}

	// An untrustworthy client or redirect is answered here, never redirected to.
	q := in.authorizeQuery(clientID+"x", challenge)
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode, "a tampered client id")
	q = in.authorizeQuery(clientID, challenge)
	q.Set("redirect_uri", "https://evil.example/cb")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)

	// A loopback redirect may change port; anything else about it may not.
	q = in.authorizeQuery(clientID, challenge)
	q.Set("redirect_uri", "http://127.0.0.1:9999/callback")
	assert.Equal(t, http.StatusOK, get(q).StatusCode)

	// Errors before a decision stay here, even for registered redirects.
	q = in.authorizeQuery(clientID, "")
	resp = get(q)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Location"))
	q = in.authorizeQuery(clientID, challenge)
	q.Set("resource", "https://elsewhere.example/mcp")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)
	q.Del("resource")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)
	q = in.authorizeQuery(clientID, challenge)
	q.Add("resource", "https://other.example/mcp")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)
	q = in.authorizeQuery(clientID, challenge)
	q.Set("code_challenge_method", "plain")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)
	q.Del("code_challenge_method")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)
	for _, malformed := range []string{"too-short", strings.Repeat("a", 10000), strings.Repeat("=", 43), strings.Repeat("a", 42) + "!"} {
		q = in.authorizeQuery(clientID, malformed)
		assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)
	}
	for _, redirect := range []string{"http://127.0.0.1:9999/else", "http://localhost:9999/callback", "http://127.0.0.1:9999/callback#leak", "http://user@127.0.0.1:9999/callback", "http://127.0.0.1:9999/callback?extra=1"} {
		q = in.authorizeQuery(clientID, challenge)
		q.Set("redirect_uri", redirect)
		response := get(q)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, redirect)
		assert.Empty(t, response.Header.Get("Location"), "untrusted redirects must not be followed")
	}

	// Deny.
	form := in.authorizeQuery(clientID, challenge)
	form.Set("decision", "deny")
	resp = in.consent(t, in.cookieOne, form)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "error=access_denied")

	// A consent submitted with no session is refused, not honored.
	form.Set("decision", "allow")
	resp = in.consent(t, nil, form)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

// Discovery, registration, browser consent and token exchange cross the two
// actual HTTP services. Every tool call then spends the resulting library grant
// at the core API, including after the owner revokes the connection.
func TestOAuthWithTheSDKClient(t *testing.T) {
	in := newOAuthInstance(t)
	in.artifactOne = seedOwnedArtifact(t, in.ro, in.ownerOne, "00000000-0000-4000-8000-000000000001", ownerOneTitle, ownerOneBody, ownerOneState)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	base, _ := url.Parse(in.srv.URL)
	jar.SetCookies(base, []*http.Cookie{in.cookieOne})
	browser := &http.Client{Jar: jar, CheckRedirect: noRedirect.CheckRedirect}
	approvals := 0
	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName: "SDK Agent", RedirectURIs: []string{testRedirect}, TokenEndpointAuthMethod: "none"}},
		RedirectURL: testRedirect,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			approvals++
			if approvals > 1 {
				return nil, errors.New("the owner revoked this connection")
			}
			u, err := url.Parse(args.URL)
			if err != nil {
				return nil, err
			}
			resp, err := browser.Get(args.URL)
			if err != nil {
				return nil, err
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, errors.New("consent page: " + resp.Status)
			}
			form := u.Query()
			require.Equal(t, in.ro.cfg.MCPURL, form.Get("resource"))
			form.Set("decision", "allow")
			req, err := http.NewRequestWithContext(ctx, "POST", in.srv.URL+"/oauth/authorize", strings.NewReader(form.Encode()))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", in.srv.URL)
			resp, err = browser.Do(req)
			if err != nil {
				return nil, err
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusFound {
				return nil, errors.New("approve: " + resp.Status)
			}
			back, err := url.Parse(resp.Header.Get("Location"))
			if err != nil {
				return nil, err
			}
			return &auth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state"), Iss: back.Query().Get("iss")}, nil
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "sdk", Version: "test"}, nil).Connect(ctx,
		&mcp.StreamableClientTransport{Endpoint: in.service.URL + "/mcp", OAuthHandler: handler}, nil)
	require.NoError(t, err)
	defer session.Close()
	call := func(name string, args any) string {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		require.False(t, result.IsError, "%+v", result.Content)
		var text strings.Builder
		for _, c := range result.Content {
			if v, ok := c.(*mcp.TextContent); ok {
				text.WriteString(v.Text)
			}
		}
		return text.String()
	}
	found := call("search_artifacts", map[string]any{"query": ""})
	require.Contains(t, found, in.artifactOne)
	require.NotContains(t, found, in.artifactTwo)
	require.Contains(t, call("get_artifact", map[string]any{"artifact_id": in.artifactOne}), ownerOneBody)
	call("edit_artifact", map[string]any{"artifact_id": in.artifactOne, "edits": []map[string]string{{"oldText": ownerOneBody, "newText": "Changed through OAuth MCP"}}})
	require.Contains(t, call("get_artifact", map[string]any{"artifact_id": in.artifactOne}), "Changed through OAuth MCP")
	grants, err := in.ro.cfg.Store.ListAgentGrants(ctx, in.ownerOne)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.Equal(t, "SDK Agent", grants[0].Label)
	assert.Equal(t, "SDK Agent", grants[0].Client)
	page := doWithCookie(t, in.ro, "GET", "/settings", in.cookieOne, nil)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Connected agents")
	assert.Contains(t, page.Body.String(), "SDK Agent")
	assert.Contains(t, page.Body.String(), grants[0].ID)
	versions, err := in.ro.cfg.Store.ListVersions(ctx, in.ownerOne, in.artifactOne)
	require.NoError(t, err)
	require.NotEmpty(t, versions)
	assert.Equal(t, store.VersionAgent, versions[0].Origin)
	assert.Equal(t, "SDK Agent", versions[0].Message)
	assert.Equal(t, "grant:"+grants[0].ID, versions[0].SessionID)
	require.Equal(t, http.StatusNoContent, doWithCookie(t, in.ro, "DELETE", "/api/agent-grants/"+grants[0].ID, in.cookieOne, nil).Code)
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "search_artifacts", Arguments: map[string]any{"query": ""}})
	require.Error(t, err, "the next MCP request must fail after revocation")
	assert.NotContains(t, doWithCookie(t, in.ro, "GET", "/settings", in.cookieOne, nil).Body.String(), grants[0].ID)
}

func (in oauthInstance) code(t *testing.T, clientID, challenge string, cookie *http.Cookie) string {
	t.Helper()
	form := in.authorizeQuery(clientID, challenge)
	form.Set("decision", "allow")
	resp := in.consent(t, cookie, form)
	require.Equal(t, http.StatusFound, resp.StatusCode, readBody(t, resp))
	back, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	code := back.Query().Get("code")
	require.NotEmpty(t, code)
	return code
}

func (in oauthInstance) tokenForm(clientID, code, verifier string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {code},
		"redirect_uri": {testRedirect}, "resource": {in.ro.cfg.MCPURL}, "code_verifier": {verifier}}
}

func TestOAuthTokenBindsCodeToClientResourceAndRedirect(t *testing.T) {
	in := newOAuthInstance(t)
	clientID := in.clientID(t)
	verifier, challenge := pkcePair()
	_, other := in.register(t, `{"client_name":"Other client","redirect_uris":["`+testRedirect+`"]}`)
	for _, tc := range []struct{ name, key, value, errorCode string }{
		{"other client", "client_id", other["client_id"].(string), "invalid_grant"},
		{"missing client", "client_id", "", "invalid_client"},
		{"other resource", "resource", "https://other.example/mcp", "invalid_target"},
		{"missing resource", "resource", "", "invalid_target"},
		{"other redirect", "redirect_uri", "http://127.0.0.1:9999/callback", "invalid_grant"},
		{"missing redirect", "redirect_uri", "", "invalid_grant"},
		{"bad verifier", "code_verifier", strings.Repeat("z", 50), "invalid_grant"},
		{"missing verifier", "code_verifier", "", "invalid_grant"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := in.tokenForm(clientID, in.code(t, clientID, challenge, in.cookieOne), verifier)
			form.Set(tc.key, tc.value)
			status, out := in.exchange(t, form)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Equal(t, tc.errorCode, out["error"])
		})
	}
	// The successful path also spends its code, and public clients may name
	// themselves using Basic with an empty password (as OAuth libraries do).
	code := in.code(t, clientID, challenge, in.cookieTwo)
	form := in.tokenForm(clientID, code, verifier)
	form.Del("client_id")
	req, err := http.NewRequest("POST", in.srv.URL+"/oauth/token", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(clientID), "")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, out)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	token := out["access_token"].(string)
	assert.Equal(t, http.StatusOK, doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactTwo, token, nil).Code)
	assert.Equal(t, http.StatusNotFound, doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactOne, token, nil).Code)
	form.Set("client_id", clientID)
	status, out := in.exchange(t, form)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", out["error"])
	grants, err := in.ro.cfg.Store.ListAgentGrants(context.Background(), in.ownerOne)
	require.NoError(t, err)
	assert.Empty(t, grants, "refused exchanges must create no grants")
}

func TestOAuthExpiredCodeCannotBeExchanged(t *testing.T) {
	in := newOAuthInstance(t)
	clientID := in.clientID(t)
	verifier, challenge := pkcePair()
	code := in.code(t, clientID, challenge, in.cookieOne)
	in.ro.oauthCodes.mu.Lock()
	entry := in.ro.oauthCodes.codes[code]
	entry.expires = time.Now().Add(-time.Second)
	in.ro.oauthCodes.codes[code] = entry
	in.ro.oauthCodes.mu.Unlock()
	status, out := in.exchange(t, in.tokenForm(clientID, code, verifier))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "invalid_grant", out["error"])
}

func TestOAuthCodeCapacityRecoversWhenCodesExpire(t *testing.T) {
	codes := newOAuthCodeStore()
	now := time.Now()
	codes.now = func() time.Time { return now }
	for i := range maxOAuthCodes {
		_, err := codes.issue(oauthCode{ownerID: int64(i)})
		require.NoError(t, err)
	}
	_, err := codes.issue(oauthCode{})
	require.Error(t, err)
	now = now.Add(oauthCodeTTL + time.Second)
	code, err := codes.issue(oauthCode{})
	require.NoError(t, err)
	_, ok := codes.redeem(code)
	require.True(t, ok)
	_, ok = codes.redeem(code)
	require.False(t, ok)
}

func TestOAuthDisabledWithoutMCPURL(t *testing.T) {
	for _, login := range []bool{false, true} {
		t.Run(map[bool]string{false: "no login", true: "login"}[login], func(t *testing.T) {
			base := newTestRouter(t)
			cfg := base.cfg
			if login {
				cfg.LocalCredential = newTestCredential(t, "owner", "password")
			}
			ro := NewRouter(cfg)
			for _, tc := range []struct{ method, path string }{
				{"GET", "/.well-known/oauth-authorization-server"}, {"POST", "/oauth/register"},
				{"GET", "/oauth/authorize"}, {"POST", "/oauth/authorize"}, {"POST", "/oauth/token"},
			} {
				assert.Equal(t, http.StatusNotFound, doWithCookie(t, ro, tc.method, tc.path, nil, nil).Code, tc.method+" "+tc.path)
			}
		})
	}
}

func TestOAuthConsentRequiresPersonAndSameOrigin(t *testing.T) {
	in := newOAuthInstance(t)
	clientID := in.clientID(t)
	_, challenge := pkcePair()
	form := in.authorizeQuery(clientID, challenge)
	form.Set("decision", "allow")
	grant := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", in.cookieOne, map[string]string{"label": "existing agent"}))
	for _, token := range []string{in.ro.cfg.AuthToken, grant.Token} {
		req := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		assert.GreaterOrEqual(t, w.Code, 400, "bearer credentials cannot approve on a login instance")
	}
	for _, headers := range []http.Header{
		{"Origin": {"https://evil.example"}},
		{"Origin": {"null"}},
		{"Sec-Fetch-Site": {"cross-site"}},
		{"Sec-Fetch-Site": {"same-site"}},
	} {
		req := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(form.Encode()))
		req.Header = headers
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(in.cookieOne)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code, headers)
	}
	cfg := in.ro.cfg
	cfg.Identity = nil
	cfg.LocalCredential = nil
	cfg.Public = PublicMode{Enabled: true, OwnerID: in.ownerOne}
	public := NewRouter(cfg)
	clientID, err := public.mintClientID(oauthClient{Name: "public test", RedirectURIs: []string{testRedirect}})
	require.NoError(t, err)
	form.Set("client_id", clientID)
	for _, method := range []string{"GET", "POST"} {
		target := "/oauth/authorize"
		if method == "GET" {
			target += "?" + form.Encode()
		}
		req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		public.ServeHTTP(w, req)
		assert.GreaterOrEqual(t, w.Code, 400, "public visitors cannot approve")
	}
	// No login: reaching the page identifies the operator, just as Settings does.
	cfg.Public = PublicMode{}
	open := NewRouter(cfg)
	clientID, err = open.mintClientID(oauthClient{Name: "operator agent", RedirectURIs: []string{testRedirect}})
	require.NoError(t, err)
	form.Set("client_id", clientID)
	for _, token := range []string{grant.Token, "exgrant_revoked", "exagent_unknown"} {
		req := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		open.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code, "an agent cannot approve even on a no-login instance")
	}
	req := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", cfg.AppOrigin)
	w := httptest.NewRecorder()
	open.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	back, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.NotEmpty(t, back.Query().Get("code"))
	verifier, _ := pkcePair()
	exchange := in.tokenForm(clientID, back.Query().Get("code"), verifier)
	req = httptest.NewRequest("POST", "/oauth/token", strings.NewReader(exchange.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	open.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	grants, err := open.cfg.Store.ListAgentGrants(context.Background(), defaultOwnerID)
	require.NoError(t, err)
	require.Len(t, grants, 2)
	assert.Contains(t, []string{grants[0].Client, grants[1].Client}, "operator agent")
}
