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

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The built-in authorization server for MCP clients (Exh-rhlq).

// oauthInstance is a two-owner instance with a login, served over HTTP at the
// origin it believes is its own, which OAuth discovery checks.
type oauthInstance struct {
	twoOwnerInstance
	srv       *httptest.Server
	cookieOne *http.Cookie
}

func newOAuthInstance(t *testing.T) oauthInstance {
	t.Helper()
	in := newTwoOwnerInstance(t)
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	cfg := in.ro.cfg
	cfg.AppOrigin = srv.URL
	in.ro = NewRouter(cfg)
	srv.Config.Handler = in.ro
	return oauthInstance{twoOwnerInstance: in, srv: srv,
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

func authorizeQuery(clientID, challenge string) url.Values {
	return url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {testRedirect},
		"state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
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
	resp, err := http.Get(in.srv.URL + "/.well-known/oauth-protected-resource/mcp")
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
	assert.Equal(t, []any{"S256"}, meta["code_challenge_methods_supported"])
	assert.Equal(t, true, meta["authorization_response_iss_parameter_supported"])
}

func TestOAuthAuthorizationCodeFlowIssuesAnArtifactGrant(t *testing.T) {
	in := newOAuthInstance(t)
	clientID := in.clientID(t)
	verifier, challenge := pkcePair()
	q := authorizeQuery(clientID, challenge)

	// No session: the gate sends the visitor to sign in and back.
	resp, err := noRedirect.Get(in.srv.URL + "/oauth/authorize?" + q.Encode())
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Location"), "/auth/login?next="), resp.Header.Get("Location"))

	// The owner sees their own artifacts, the client's name as a claim, and
	// where the browser will be sent.
	req, _ := http.NewRequest("GET", in.srv.URL+"/oauth/authorize?"+q.Encode(), nil)
	req.AddCookie(in.cookieOne)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	page := readBody(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, page)
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Contains(t, page, "“Test Agent”")
	assert.Contains(t, page, "http://127.0.0.1:4567")
	assert.Contains(t, page, ownerOneTitle)
	assert.NotContains(t, page, ownerTwoTitle, "only the signed-in owner's artifacts are offered")

	// Choosing somebody else's artifact is refused like a missing one.
	form := url.Values{}
	for k, v := range q {
		form[k] = v
	}
	form.Set("decision", "allow")
	form.Set("artifact_id", in.artifactTwo)
	resp = in.consent(t, in.cookieOne, form)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	form.Set("artifact_id", in.artifactOne)
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
		"redirect_uri": {testRedirect}, "code_verifier": {strings.Repeat("w", 50)}}
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

	// The token is an agent grant over that artifact: listed under the
	// client's name, reaching it and nothing else.
	w := doWithCookie(t, in.ro, "GET", "/api/artifacts/"+in.artifactOne+"/agent-grants", in.cookieOne, nil)
	assert.Contains(t, w.Body.String(), `"label":"Test Agent"`)
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactOne, token, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactTwo, token, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOAuthRefusals(t *testing.T) {
	in := newOAuthInstance(t)

	for _, uri := range []string{"javascript:alert(1)", "http://evil.example/cb", "https://ok.example/cb#frag", "/relative"} {
		resp, out := in.register(t, `{"redirect_uris":["`+uri+`"]}`)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, uri)
		assert.Equal(t, "invalid_redirect_uri", out["error"], uri)
	}
	resp, _ := in.register(t, `{"redirect_uris":["https://ok.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, out := in.register(t, `{"redirect_uris":["cursor://anysphere.cursor-retrieval/oauth/callback"]}`)
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "a native app's private-use scheme")
	assert.Equal(t, "MCP client", out["client_name"])

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
	q := authorizeQuery(clientID+"x", challenge)
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode, "a tampered client id")
	q = authorizeQuery(clientID, challenge)
	q.Set("redirect_uri", "https://evil.example/cb")
	assert.Equal(t, http.StatusBadRequest, get(q).StatusCode)

	// A loopback redirect may change port; anything else about it may not.
	q = authorizeQuery(clientID, challenge)
	q.Set("redirect_uri", "http://127.0.0.1:9999/callback")
	assert.Equal(t, http.StatusOK, get(q).StatusCode)

	// A well-formed request missing PKCE goes back to the client as an error.
	q = authorizeQuery(clientID, "")
	resp = get(q)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "error=invalid_request")
	q = authorizeQuery(clientID, challenge)
	q.Set("resource", "https://elsewhere.example/mcp")
	assert.Contains(t, get(q).Header.Get("Location"), "error=invalid_target")

	// Deny.
	form := authorizeQuery(clientID, challenge)
	form.Set("decision", "deny")
	resp = in.consent(t, in.cookieOne, form)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "error=access_denied")

	// A consent submitted with no session is refused, not honored.
	form.Set("decision", "allow")
	form.Set("artifact_id", in.artifactOne)
	resp = in.consent(t, nil, form)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// The SDK's own client, driven through discovery, registration, consent and
// the token exchange, then connected to /mcp with what it got: the whole flow
// as an MCP client sees it.
func TestOAuthWithTheSDKClient(t *testing.T) {
	in := newOAuthInstance(t)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	base, _ := url.Parse(in.srv.URL)
	jar.SetCookies(base, []*http.Cookie{in.cookieOne})
	browser := &http.Client{Jar: jar, CheckRedirect: noRedirect.CheckRedirect}

	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName: "SDK Agent", RedirectURIs: []string{testRedirect}, TokenEndpointAuthMethod: "none"}},
		RedirectURL: testRedirect,
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			// The person: open the page, pick the artifact, allow.
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
			form.Set("decision", "allow")
			form.Set("artifact_id", in.artifactOne)
			resp, err = browser.PostForm(in.srv.URL+"/oauth/authorize", form)
			if err != nil {
				return nil, err
			}
			resp.Body.Close()
			back, err := url.Parse(resp.Header.Get("Location"))
			if err != nil {
				return nil, err
			}
			return &auth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state"),
				Iss: back.Query().Get("iss")}, nil
		},
	})
	require.NoError(t, err)

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "sdk", Version: "test"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: in.srv.URL + "/mcp", OAuthHandler: handler}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	h := mcpHarness{r: in.ro, id: in.artifactOne, session: cs}
	out, isErr := h.call(t, "get_artifact", nil)
	require.False(t, isErr, out)
	assert.Contains(t, out, ownerOneBody)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}
