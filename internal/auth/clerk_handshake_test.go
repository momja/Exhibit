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

// startHandshake runs the outbound leg for target and returns the URL Clerk
// would send the browser back to, plus the cookies the browser then holds.
func startHandshake(t *testing.T, c *Clerk, target string) (*url.URL, map[string]string) {
	t.Helper()
	r := pageRequest(target, map[string]string{"__session": "expired.jwt.value", clientUATCookie: "1793000000", devBrowserCookie: "dvb_example"})
	w := httptest.NewRecorder()
	var redirected bool
	c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = c.HandshakeRedirect(w, r) })).ServeHTTP(w, r)
	require.True(t, redirected)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	back, err := url.Parse(loc.Query().Get("redirect_url"))
	require.NoError(t, err)
	jar := map[string]string{}
	for _, cookie := range w.Result().Cookies() {
		jar[cookie.Name] = cookie.Value
	}
	return back, jar
}

// returnFrom builds the browser's return request: the URL Clerk redirects to
// with extra query parameters added, carrying the browser's cookies.
func returnFrom(back *url.URL, extra url.Values, cookies map[string]string) *http.Request {
	u := *back
	q := u.Query()
	for k, vs := range extra {
		q[k] = vs
	}
	u.RawQuery = q.Encode()
	return pageRequest(u.RequestURI(), cookies)
}

func setCookies(w *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range w.Result().Cookies() {
		out[c.Name] = c
	}
	return out
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
	assert.Equal(t, "nonce", q.Get("format"))
	assert.Equal(t, "false", q.Get("suffixed_cookies"))
	assert.Equal(t, "dvb_example", q.Get(devBrowserCookie))
	assert.Empty(t, q.Get("__session"), "the lapsed token is not sent along")

	back, err := url.Parse(q.Get("redirect_url"))
	require.NoError(t, err)
	assert.Equal(t, "https://app.test/", back.Scheme+"://"+back.Host+back.Path)
	assert.Equal(t, "charts", back.Query().Get("q"))
	state := back.Query().Get(handshakeStateParam)
	assert.GreaterOrEqual(t, len(state), 43, "the state is 256 random bits")

	cookies := setCookies(w)
	require.Contains(t, cookies, "__Host-exhibit_clerk_hs")
	stateCookie := cookies["__Host-exhibit_clerk_hs"]
	assert.Equal(t, state, stateCookie.Value)
	assert.True(t, stateCookie.HttpOnly)
	assert.True(t, stateCookie.Secure)
	assert.Equal(t, "/", stateCookie.Path)
	assert.Empty(t, stateCookie.Domain)
	assert.Equal(t, "1", cookies[handshakeCountCookie].Value)

	_, again := startHandshake(t, c, "/")
	assert.NotEqual(t, state, again["__Host-exhibit_clerk_hs"], "every handshake gets its own state")
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
			return pageRequest("/", map[string]string{clientUATCookie: "1793000000", devBrowserCookie: "dvb_example", handshakeCountCookie: "3"})
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
	back, jar := startHandshake(t, c, "/?q=charts")
	r := returnFrom(back, url.Values{handshakeNonceParam: {"nonce_example"}, devBrowserCookie: {"dvb_new"}}, jar)
	w := httptest.NewRecorder()
	c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("the page rendered before its cookies were set") })).ServeHTTP(w, r)

	assert.Equal(t, 1, *calls)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "https://app.test/?q=charts", w.Header().Get("Location"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	set := w.Header().Values("Set-Cookie")
	for _, d := range directives {
		assert.Contains(t, set, d)
	}
	spent := setCookies(w)["__Host-exhibit_clerk_hs"]
	require.NotNil(t, spent, "the state is spent")
	assert.Negative(t, spent.MaxAge)
}

// Review of Exh-8409: the return is bound to the browser and the flow that
// started it. A return that cannot show that is refused, whatever it carries.
func TestHandshakeReturnIsBoundToTheFlowThatStartedIt(t *testing.T) {
	c, calls := handshakeClerk(t, []string{"__session=attacker.jwt; Path=/"}, http.StatusOK)
	back, jar := startHandshake(t, c, "/")
	forged := *back
	fq := forged.Query()
	fq.Set(handshakeStateParam, "guessed")
	forged.RawQuery = fq.Encode()

	for name, r := range map[string]*http.Request{
		// The attacker's link, in a browser that never started a handshake.
		"no handshake started": returnFrom(back, url.Values{handshakeNonceParam: {"nonce_example"}}, nil),
		// The victim's browser is mid-handshake, but the link is not the
		// return of that handshake.
		"another flow's state": returnFrom(&forged, url.Values{handshakeNonceParam: {"nonce_example"}}, jar),
		// A sibling domain can plant a cookie without the __Host- prefix.
		"a planted cookie": returnFrom(&forged, url.Values{handshakeNonceParam: {"nonce_example"}}, map[string]string{"exhibit_clerk_hs": "guessed"}),
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c.Middleware(http.NotFoundHandler()).ServeHTTP(w, r)
			assert.Equal(t, 0, *calls)
			assert.NotContains(t, w.Header().Values("Set-Cookie"), "__session=attacker.jwt; Path=/")
			assert.Equal(t, "3", setCookies(w)[handshakeCountCookie].Value, "the page goes to the login page")
		})
	}

	// And a genuine return is completed once: replaying it is refused.
	r := returnFrom(back, url.Values{handshakeNonceParam: {"nonce_example"}}, jar)
	w := httptest.NewRecorder()
	c.Middleware(http.NotFoundHandler()).ServeHTTP(w, r)
	assert.Equal(t, 1, *calls)
	jar["__Host-exhibit_clerk_hs"] = "" // what the browser holds after the spend
	w = httptest.NewRecorder()
	c.Middleware(http.NotFoundHandler()).ServeHTTP(w, returnFrom(back, url.Values{handshakeNonceParam: {"nonce_example"}}, jar))
	assert.Equal(t, 1, *calls)
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
			back, jar := startHandshake(t, c, "/artifacts/a1")
			w := httptest.NewRecorder()
			c.Middleware(http.NotFoundHandler()).ServeHTTP(w, returnFrom(back, url.Values{handshakeNonceParam: {"nonce_example"}}, jar))

			assert.Equal(t, 1, *calls)
			require.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, "https://app.test/artifacts/a1", w.Header().Get("Location"))
			cookies := setCookies(w)
			assert.NotContains(t, cookies, "__session")
			// The state is spent and the count exhausted, so the page goes
			// straight to the login page rather than failing twice more.
			assert.Negative(t, cookies["__Host-exhibit_clerk_hs"].MaxAge)
			assert.Equal(t, "3", cookies[handshakeCountCookie].Value)
		})
	}
}

// A production Frontend API may deliver the nonce or the token as a cookie
// instead of in the URL. Without the state the return is not completed at
// all, so a cookie left behind can never cause a loop.
func TestHandshakeReadsCookieDeliveredResults(t *testing.T) {
	c, calls := handshakeClerk(t, []string{"__session=fresh.jwt; Path=/"}, http.StatusOK)
	back, jar := startHandshake(t, c, "/")
	jar[handshakeNonceParam] = "nonce_example"
	w := httptest.NewRecorder()
	c.Middleware(http.NotFoundHandler()).ServeHTTP(w, returnFrom(back, nil, jar))
	assert.Equal(t, 1, *calls)
	assert.Contains(t, w.Header().Values("Set-Cookie"), "__session=fresh.jwt; Path=/")

	reached := false
	c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(httptest.NewRecorder(), pageRequest("/", jar))
	assert.True(t, reached, "a leftover nonce cookie is ignored outside a handshake's return")
	assert.Equal(t, 1, *calls)
}

// Review of Exh-8409: the cleaned URL is a path on the app's own origin even
// when the requested path looks like a link to another one.
func TestHandshakeReturnNeverRedirectsOffSite(t *testing.T) {
	c, _ := handshakeClerk(t, nil, http.StatusBadRequest)
	for _, path := range []string{"//evil.test/path", "/\\evil.test/path"} {
		back, jar := startHandshake(t, c, path)
		w := httptest.NewRecorder()
		c.Middleware(http.NotFoundHandler()).ServeHTTP(w, returnFrom(back, url.Values{handshakeNonceParam: {"nonce_example"}}, jar))
		loc := w.Header().Get("Location")
		assert.True(t, strings.HasPrefix(loc, "https://app.test/"), loc)
	}
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
			back, jar := startHandshake(t, c, "/")
			w := httptest.NewRecorder()
			c.Middleware(http.NotFoundHandler()).ServeHTTP(w, returnFrom(back, url.Values{handshakeTokenParam: {tt.token}}, jar))
			require.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, "https://app.test/", w.Header().Get("Location"))
			var set []string
			for _, line := range w.Header().Values("Set-Cookie") {
				if !strings.Contains(line, "exhibit_clerk_") {
					set = append(set, line)
				}
			}
			assert.Equal(t, tt.cookies, set)
		})
	}
	assert.Equal(t, 1, jwksCalls, "the signing key is fetched once and kept")

	// Review of Exh-8409: a token naming a key nobody has does not buy a
	// Backend API call per request.
	unknown := func() string {
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: forger}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "made_up"))
		require.NoError(t, err)
		token, err := josejwt.Signed(signer).Claims(map[string]any{"handshake": []string{"__session=x; Path=/"}}).CompactSerialize()
		require.NoError(t, err)
		return token
	}()
	for range 5 {
		back, jar := startHandshake(t, c, "/")
		c.Middleware(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), returnFrom(back, url.Values{handshakeTokenParam: {unknown}}, jar))
	}
	assert.Equal(t, 1, jwksCalls, "unknown key ids refetch at most every five minutes")
}
