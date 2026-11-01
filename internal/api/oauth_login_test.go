package api

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oauthLoginDestination deliberately exceeds a browser's cookie limit, as a
// signed registration with several redirect URIs may. Its raw query also has a
// quote, which net/http would silently strip from a cookie value.
func oauthLoginDestination(t *testing.T, ro *Router) string {
	t.Helper()
	redirect := "https://client.test/callback"
	body, err := json.Marshal(map[string]any{
		"client_name":                "Round trip agent",
		"redirect_uris":              []string{redirect, "https://client.test/" + strings.Repeat("a", 1900), "https://client.test/" + strings.Repeat("b", 1900)},
		"token_endpoint_auth_method": "none",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var registration struct {
		ClientID string `json:"client_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &registration))
	q := url.Values{
		"client_id": {registration.ClientID}, "response_type": {"code"},
		"redirect_uri": {redirect}, "resource": {ro.cfg.MCPURL},
		"code_challenge": {auth.S256Challenge(strings.Repeat("v", 50))}, "code_challenge_method": {"S256"},
	}
	destination := "/oauth/authorize?" + q.Encode() + `&state=quoted"state%3B%26%2B%25`
	require.Greater(t, len(destination), 4096)
	return destination
}

func oauthLoginStart(t *testing.T, ro *Router, destination string) string {
	t.Helper()
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://app.test"+destination, nil))
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	login, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "/auth/login", login.Path)
	require.Equal(t, destination, login.Query().Get("next"))
	return login.String()
}

func oauthAssertConsentAfterLogin(t *testing.T, ro *Router, destination string, cookie *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "https://app.test"+destination, nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Round trip agent")
}

func TestOAuthLocalLoginPreservesAuthorizationURL(t *testing.T) {
	ro, _ := newLoginTestRouter(t, nil, newTestCredential(t, testUsername, testPassword), func(c *Config) { c.MCPURL = "https://mcp.test/mcp" })
	destination := oauthLoginDestination(t, ro)
	login := oauthLoginStart(t, ro, destination)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, login, nil))
	require.Equal(t, http.StatusOK, w.Code)
	field := regexp.MustCompile(`name="next" value="([^"]*)"`).FindStringSubmatch(w.Body.String())
	require.Len(t, field, 2)
	next := html.UnescapeString(field[1])
	require.Equal(t, destination, next)
	// A mistyped password must preserve the same destination for the retry.
	failed := submitLogin(t, ro, testUsername, "wrong password", next)
	require.Equal(t, http.StatusUnauthorized, failed.Code)
	field = regexp.MustCompile(`name="next" value="([^"]*)"`).FindStringSubmatch(failed.Body.String())
	require.Len(t, field, 2)
	require.Equal(t, destination, html.UnescapeString(field[1]))
	loggedIn := submitLogin(t, ro, testUsername, testPassword, next)
	require.Equal(t, http.StatusFound, loggedIn.Code)
	require.Equal(t, destination, loggedIn.Header().Get("Location"))
	oauthAssertConsentAfterLogin(t, ro, destination, cookiesFrom(loggedIn)[sessionCookieName])
}

func TestOAuthOIDCLoginPreservesAuthorizationURL(t *testing.T) {
	for _, localAlso := range []bool{false, true} {
		t.Run(map[bool]string{false: "OIDC only", true: "SSO from local login"}[localAlso], func(t *testing.T) {
			idp := &stubProvider{loginURL: "https://idp.test/authorize", identities: []auth.Identity{{ExternalID: "round-trip-user", Email: "owner@example.test"}}}
			ro, _ := newLoginTestRouter(t, idp, nil, func(c *Config) { c.MCPURL = "https://mcp.test/mcp"; c.LocalUsers = localAlso })
			destination := oauthLoginDestination(t, ro)
			login := oauthLoginStart(t, ro, destination)
			w := httptest.NewRecorder()
			ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, login, nil))
			if localAlso {
				require.Equal(t, http.StatusOK, w.Code)
				link := regexp.MustCompile(`href="([^"]*/auth/sso[^"]*)"`).FindStringSubmatch(w.Body.String())
				require.Len(t, link, 2)
				w = httptest.NewRecorder()
				ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, html.UnescapeString(link[1]), nil))
			}
			require.Equal(t, http.StatusFound, w.Code)
			flow := cookiesFrom(w)
			require.Len(t, flow, 2, "only fixed-size state and PKCE cookies cross the provider round trip")
			for _, cookie := range flow {
				require.Less(t, len(cookie.String()), 4096)
			}
			callback := httptest.NewRequest(http.MethodGet, "/auth/callback?code=provider-code&state="+url.QueryEscape(flow[stateCookieName].Value), nil)
			callback.AddCookie(flow[stateCookieName])
			callback.AddCookie(flow[verifierCookieName])
			w = httptest.NewRecorder()
			ro.ServeHTTP(w, callback)
			require.Equal(t, http.StatusFound, w.Code, w.Body.String())
			require.Equal(t, destination, w.Header().Get("Location"))
			require.Equal(t, 1, idp.exchanges)
			oauthAssertConsentAfterLogin(t, ro, destination, cookiesFrom(w)[sessionCookieName])
		})
	}
}

func TestOAuthClerkLoginPreservesAuthorizationURL(t *testing.T) {
	ro, _ := newLoginTestRouter(t, nil, nil, func(c *Config) {
		c.MCPURL = "https://mcp.test/mcp"
		c.Clerk = hostedLoginStub{}
		c.ClerkPublishableKey = "pk_test_example"
		c.ClerkFrontendAPI = "https://example.clerk.accounts.dev"
	})
	destination := oauthLoginDestination(t, ro)
	login := oauthLoginStart(t, ro, destination)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://app.test"+login, nil))
	require.Equal(t, http.StatusOK, w.Code)
	field := regexp.MustCompile(`data-clerk-next-value="([^"]*)"`).FindStringSubmatch(w.Body.String())
	require.Len(t, field, 2)
	require.Equal(t, destination, html.UnescapeString(field[1]))
	// The verified hosted session reaches the original consent URL without
	// requiring or minting a self-hosted session cookie.
	req := httptest.NewRequest(http.MethodGet, "https://app.test"+destination, nil)
	req.Header.Set("Test-Clerk-Subject", "clerk:round-trip-user")
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Round trip agent")
	require.Contains(t, w.Body.String(), `name="clerk-publishable-key"`)
	require.Contains(t, w.Body.String(), "/gallery/clerk.js", "consent must keep the short-lived Clerk session fresh while the person reads it")
}

func TestLoginReturnStoreBoundedSingleUseAndExpiry(t *testing.T) {
	var returns loginReturnStore
	returns.put("flow-one", "/oauth/authorize?state=one")
	returns.put("flow-two", "/oauth/authorize?state=two")
	assert.Empty(t, returns.take("unknown"))
	assert.Equal(t, "/oauth/authorize?state=two", returns.take("flow-two"))
	assert.Empty(t, returns.take("flow-two"))
	returns.pending["flow-one"] = loginReturn{url: "/oauth/authorize", expires: time.Now().Add(-time.Second)}
	assert.Empty(t, returns.take("flow-one"))
	for i := 0; i < maxLoginReturns; i++ {
		returns.put(string(rune(i+1)), "/")
	}
	// Clock resolution can give rapid inserts the same expiry. Give the first
	// entry a distinct age so this asserts eviction policy, not timer precision.
	first := string(rune(1))
	returns.pending[first] = loginReturn{url: "/", expires: time.Now().Add(loginFlowTTL - time.Minute)}
	returns.put(string(rune(maxLoginReturns+1)), "/")
	assert.Len(t, returns.pending, maxLoginReturns)
	assert.Empty(t, returns.take(string(rune(1))), "oldest pending return is evicted")
	assert.Equal(t, "/", returns.take(string(rune(maxLoginReturns+1))))
}

func TestLoginReturnStoreBoundsURLBytes(t *testing.T) {
	var returns loginReturnStore
	destination := "/" + strings.Repeat("x", maxLoginReturnBytes/2-1)
	for _, state := range []string{"one", "two", "three"} {
		returns.put(state, destination)
		if state == "one" {
			returns.pending[state] = loginReturn{url: destination, expires: time.Now().Add(loginFlowTTL - time.Minute)}
		}
	}
	assert.Len(t, returns.pending, 2)
	assert.Empty(t, returns.take("one"))
	assert.Equal(t, destination, returns.take("three"))
	assert.Equal(t, destination, returns.take("two"))
	returns.put("too large", destination+destination+"x")
	assert.Empty(t, returns.pending)
}
