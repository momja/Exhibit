package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/clerk/clerk-sdk-go/v2"
	jose "github.com/go-jose/go-jose/v3"
	josejwt "github.com/go-jose/go-jose/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handshakeClerk is a development-instance Clerk whose Backend API is a test
// server answering the handshake payload with directives, counting calls.
func handshakeClerk(t *testing.T, directives []string, status int) (*Clerk, *int) {
	t.Helper()
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk_test_example", r.Header.Get("Authorization"))
		require.Equal(t, "/clients/handshake_payload", r.URL.Path)
		require.Equal(t, "nonce_example", r.URL.Query().Get("nonce"))
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"directives": directives}))
	}))
	t.Cleanup(backend.Close)
	config := &clerk.ClientConfig{}
	config.Key = clerk.String("sk_test_example")
	config.URL = clerk.String(backend.URL)
	c, err := newClerk("pk_test_"+base64.RawStdEncoding.EncodeToString([]byte("example.clerk.accounts.dev$")), "https://app.test", config)
	require.NoError(t, err)
	return c, &calls
}

func pageRequest(target string, cookies map[string]string) *http.Request {
	r := httptest.NewRequest("GET", "https://app.test"+target, nil)
	r.Header.Set("Sec-Fetch-Dest", "document")
	for name, value := range cookies {
		r.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	return r
}

// Exh-8409: a page request whose token lapsed while the browser stayed signed
// in to Clerk renews through the handshake rather than the login page.
func TestHandshakeRedirectSendsLapsedSessionToFrontendAPI(t *testing.T) {
	c, _ := handshakeClerk(t, nil, http.StatusOK)
	r := pageRequest("/?q=charts", map[string]string{"__session": "expired.jwt.value", clientUATCookie: "1793000000", devBrowserCookie: "dvb_example"})
	w := httptest.NewRecorder()
	var redirected bool
	c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = c.HandshakeRedirect(w, r) })).ServeHTTP(w, r)

	require.True(t, redirected)
	require.Equal(t, http.StatusTemporaryRedirect, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "https://example.clerk.accounts.dev/v1/client/handshake", loc.Scheme+"://"+loc.Host+loc.Path)
	q := loc.Query()
	assert.Equal(t, "https://app.test/?q=charts", q.Get("redirect_url"))
	assert.Equal(t, "nonce", q.Get("format"))
	assert.Equal(t, "false", q.Get("suffixed_cookies"))
	assert.Equal(t, "dvb_example", q.Get(devBrowserCookie))
	assert.Empty(t, q.Get("__session"), "the lapsed token is not sent along")
	cookie := w.Result().Cookies()
	require.Len(t, cookie, 1)
	assert.Equal(t, handshakePendingCookie, cookie[0].Name)
	assert.Equal(t, "1", cookie[0].Value)
	assert.True(t, cookie[0].HttpOnly)
}

func TestHandshakeRedirectDeclinesWhatItCannotRenew(t *testing.T) {
	signedIn := map[string]string{clientUATCookie: "1793000000", devBrowserCookie: "dvb_example"}
	tests := []struct {
		name    string
		request func() *http.Request
	}{
		{"signed out of Clerk", func() *http.Request {
			return pageRequest("/", map[string]string{clientUATCookie: "0", devBrowserCookie: "dvb_example"})
		}},
		{"never signed in", func() *http.Request { return pageRequest("/", map[string]string{devBrowserCookie: "dvb_example"}) }},
		{"not a page load", func() *http.Request {
			r := pageRequest("/partials/gallery-grid", signedIn)
			r.Header.Set("Sec-Fetch-Dest", "empty")
			return r
		}},
		{"not a GET", func() *http.Request {
			r := pageRequest("/", signedIn)
			r.Method = http.MethodPost
			return r
		}},
		{"development browser unknown", func() *http.Request {
			return pageRequest("/", map[string]string{clientUATCookie: "1793000000"})
		}},
		{"already tried three times", func() *http.Request {
			return pageRequest("/", map[string]string{clientUATCookie: "1793000000", devBrowserCookie: "dvb_example", handshakePendingCookie: "3"})
		}},
	}
	c, _ := handshakeClerk(t, nil, http.StatusOK)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			assert.False(t, c.HandshakeRedirect(w, tt.request()))
			assert.Empty(t, w.Header().Get("Location"))
			assert.Empty(t, w.Result().Cookies())
		})
	}
}

func TestHandshakeAcceptsHTMLWithoutFetchMetadata(t *testing.T) {
	c, _ := handshakeClerk(t, nil, http.StatusOK)
	r := pageRequest("/", map[string]string{clientUATCookie: "1793000000", devBrowserCookie: "dvb_example"})
	r.Header.Del("Sec-Fetch-Dest")
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	assert.True(t, c.HandshakeRedirect(httptest.NewRecorder(), r))
}

func TestHandshakeReturnSetsCookiesAndCleansURL(t *testing.T) {
	directives := []string{"__session=fresh.jwt; Path=/; SameSite=Lax", "__client_uat=1793000001; Path=/; SameSite=Lax"}
	c, calls := handshakeClerk(t, directives, http.StatusOK)
	r := pageRequest("/?q=charts&__clerk_handshake_nonce=nonce_example&__clerk_db_jwt=dvb_new", map[string]string{handshakePendingCookie: "1"})
	w := httptest.NewRecorder()
	c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("the page rendered before its cookies were set") })).ServeHTTP(w, r)

	assert.Equal(t, 1, *calls)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/?q=charts", w.Header().Get("Location"))
	assert.Equal(t, directives, w.Header().Values("Set-Cookie"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

// A nonce on a link this browser did not start would sign it in as whoever
// obtained the nonce, so it is not redeemed.
func TestHandshakeNonceIsOnlyRedeemedByTheBrowserThatAskedForIt(t *testing.T) {
	c, calls := handshakeClerk(t, []string{"__session=attacker.jwt; Path=/"}, http.StatusOK)
	r := pageRequest("/?__clerk_handshake_nonce=nonce_example", nil)
	w := httptest.NewRecorder()
	reached := false
	c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(w, r)

	assert.True(t, reached)
	assert.Equal(t, 0, *calls)
	assert.Empty(t, w.Header().Values("Set-Cookie"))
}

func TestHandshakeFailureFallsBackWithoutCookies(t *testing.T) {
	for name, tc := range map[string]struct {
		directives []string
		status     int
	}{
		"backend refuses":                 {[]string{"__session=x"}, http.StatusBadRequest},
		"directive is not a header value": {[]string{"__session=x\r\nLocation: https://evil.test"}, http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			c, calls := handshakeClerk(t, tc.directives, tc.status)
			r := pageRequest("/artifacts/a1?__clerk_handshake_nonce=nonce_example", map[string]string{handshakePendingCookie: "1"})
			w := httptest.NewRecorder()
			c.Middleware(http.NotFoundHandler()).ServeHTTP(w, r)

			assert.Equal(t, 1, *calls)
			require.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, "/artifacts/a1", w.Header().Get("Location"))
			// Only the marker is set, exhausted, so the page goes straight to
			// the login page rather than failing the same way twice more.
			cookies := w.Result().Cookies()
			require.Len(t, cookies, 1)
			assert.Equal(t, handshakePendingCookie, cookies[0].Name)
			assert.Equal(t, "3", cookies[0].Value)
		})
	}
}

// A production Frontend API may return the nonce as a cookie. Spending it on
// a request that already has a usable token would cost a Backend API call on
// every request the cookie outlives; spending it on one that does not is the
// handshake.
func TestHandshakeNonceCookieIsSpentOnlyWithoutAUsableToken(t *testing.T) {
	c, calls := handshakeClerk(t, []string{"__session=fresh.jwt; Path=/"}, http.StatusOK)
	cookies := map[string]string{handshakePendingCookie: "1", handshakeNonceParam: "nonce_example"}

	partial := pageRequest("/partials/gallery-grid", cookies)
	partial.Header.Set("Sec-Fetch-Dest", "empty")
	reached := false
	c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(httptest.NewRecorder(), partial)
	assert.True(t, reached, "a fragment request is never redirected through the handshake")
	assert.Equal(t, 0, *calls)

	w := httptest.NewRecorder()
	c.Middleware(http.NotFoundHandler()).ServeHTTP(w, pageRequest("/", cookies))
	assert.Equal(t, 1, *calls)
	assert.Equal(t, "/", w.Header().Get("Location"))
}

// A development Frontend API answers with the directives in a JWT signed by
// the instance's key, whatever format was asked for.
func TestHandshakeTokenIsVerifiedBeforeItsCookiesAreSet(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	forger, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	jwk := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "ins_key", Algorithm: "RS256", Use: "sig"}
	jwksCalls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/jwks", r.URL.Path)
		jwksCalls++
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}))
	}))
	defer backend.Close()
	config := &clerk.ClientConfig{}
	config.Key = clerk.String("sk_test_example")
	config.URL = clerk.String(backend.URL)
	c, err := newClerk("pk_test_"+base64.RawStdEncoding.EncodeToString([]byte("example.clerk.accounts.dev$")), "https://app.test", config)
	require.NoError(t, err)
	sign := func(k *rsa.PrivateKey, category string) string {
		opts := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "ins_key")
		if category != "" {
			opts = opts.WithHeader("cat", category)
		}
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: k}, opts)
		require.NoError(t, err)
		token, err := josejwt.Signed(signer).Claims(map[string]any{"handshake": []string{"__session=fresh.jwt; Path=/"}}).CompactSerialize()
		require.NoError(t, err)
		return token
	}
	for _, tt := range []struct {
		name    string
		token   string
		cookies []string
	}{
		{"signed by the instance", sign(key, "cl_B7d4PD111AAA"), []string{"__session=fresh.jwt; Path=/"}},
		{"signed by anyone else", sign(forger, "cl_B7d4PD111AAA"), nil},
		{"a machine token's category", sign(key, "cl_B7d4PD333AAA"), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := pageRequest("/?__clerk_handshake="+tt.token, map[string]string{handshakePendingCookie: "1"})
			w := httptest.NewRecorder()
			c.Middleware(http.NotFoundHandler()).ServeHTTP(w, r)
			require.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, "/", w.Header().Get("Location"))
			var set []string
			for _, line := range w.Header().Values("Set-Cookie") {
				if !strings.HasPrefix(line, handshakePendingCookie+"=") {
					set = append(set, line)
				}
			}
			assert.Equal(t, tt.cookies, set)
		})
	}
	assert.Equal(t, 1, jwksCalls, "the signing key is fetched once and kept")
}
