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
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	jose "github.com/go-jose/go-jose/v3"
	josejwt "github.com/go-jose/go-jose/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handshakeFixture is a development-instance Clerk whose Backend API is a test
// server: it serves the instance's signing key and answers the handshake
// payload with directives, counting both.
type handshakeFixture struct {
	c          *Clerk
	key        *rsa.PrivateKey
	directives []string
	status     int
	jwksDown   bool
	nonceCalls int
	jwksCalls  int
}

func newHandshakeFixture(t *testing.T) *handshakeFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &handshakeFixture{key: key, status: http.StatusOK}
	jwk := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "ins_key", Algorithm: "RS256", Use: "sig"}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk_test_example", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/jwks":
			f.jwksCalls++
			if f.jwksDown {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}))
		case "/clients/handshake_payload":
			require.Equal(t, "nonce_example", r.URL.Query().Get("nonce"))
			f.nonceCalls++
			w.WriteHeader(f.status)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"directives": f.directives}))
		default:
			t.Errorf("unexpected Backend API call %s", r.URL.Path)
		}
	}))
	t.Cleanup(backend.Close)
	config := &clerk.ClientConfig{}
	config.Key = clerk.String("sk_test_example")
	config.URL = clerk.String(backend.URL)
	f.c, err = newClerk("pk_test_"+base64.RawStdEncoding.EncodeToString([]byte("example.clerk.accounts.dev$")), "https://app.test", config)
	require.NoError(t, err)
	return f
}

// token signs claims with key under kid.
func (f *handshakeFixture) token(t *testing.T, key *rsa.PrivateKey, kid string, headers map[string]string, claims map[string]any) string {
	t.Helper()
	opts := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid)
	for k, v := range headers {
		opts = opts.WithHeader(jose.HeaderKey(k), v)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, opts)
	require.NoError(t, err)
	token, err := josejwt.Signed(signer).Claims(claims).CompactSerialize()
	require.NoError(t, err)
	return token
}

// session is a session token for user, expired when lapsed.
func (f *handshakeFixture) session(t *testing.T, user string, lapsed bool) string {
	exp := time.Now().Add(time.Minute)
	if lapsed {
		exp = time.Now().Add(-time.Minute)
	}
	return f.token(t, f.key, "ins_key", nil, map[string]any{"iss": f.c.FrontendAPI, "sub": user, "sid": "sess_" + user, "azp": "https://app.test", "exp": exp.Unix(), "iat": exp.Add(-time.Minute).Unix()})
}

func (f *handshakeFixture) renewal(t *testing.T, user string) []string {
	return []string{"__session=" + f.session(t, user, false) + "; Path=/; SameSite=Lax", "__client_uat=1793000001; Path=/; SameSite=Lax"}
}

func pageRequest(target string, cookies map[string]string) *http.Request {
	r := httptest.NewRequest("GET", "https://app.test"+target, nil)
	r.Header.Set("Sec-Fetch-Dest", "document")
	for name, value := range cookies {
		r.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	return r
}

// lapsed is the cookies of a browser whose user_one session token lapsed.
func (f *handshakeFixture) lapsed(t *testing.T) map[string]string {
	return map[string]string{"__session": f.session(t, "user_one", true), clientUATCookie: "1793000000", devBrowserCookie: "dvb_example"}
}

// start runs the outbound leg for target and returns the URL Clerk would send
// the browser back to, plus the cookies the browser then holds.
func (f *handshakeFixture) start(t *testing.T, target string) (*url.URL, map[string]string) {
	t.Helper()
	jar := f.lapsed(t)
	w := httptest.NewRecorder()
	var redirected bool
	f.c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = f.c.HandshakeRedirect(w, r) })).ServeHTTP(w, pageRequest(target, jar))
	require.True(t, redirected)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	back, err := url.Parse(loc.Query().Get("redirect_url"))
	require.NoError(t, err)
	for _, cookie := range w.Result().Cookies() {
		jar[cookie.Name] = cookie.Value
	}
	return back, jar
}

// finish sends the browser's return: the URL Clerk redirects to with extra
// query parameters added, carrying the browser's cookies.
func (f *handshakeFixture) finish(back *url.URL, extra url.Values, cookies map[string]string) *httptest.ResponseRecorder {
	u := *back
	q := u.Query()
	for k, vs := range extra {
		q[k] = vs
	}
	u.RawQuery = q.Encode()
	w := httptest.NewRecorder()
	f.c.Middleware(http.NotFoundHandler()).ServeHTTP(w, pageRequest(u.RequestURI(), cookies))
	return w
}

func setCookies(w *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range w.Result().Cookies() {
		out[c.Name] = c
	}
	return out
}

func sessionSet(w *httptest.ResponseRecorder) bool {
	_, ok := setCookies(w)["__session"]
	return ok
}

var withNonce = url.Values{handshakeNonceParam: {"nonce_example"}}

// Exh-8409: a page request whose token lapsed while the browser stayed signed
// in to Clerk renews through the handshake rather than the login page.
func TestHandshakeRedirectSendsLapsedSessionToFrontendAPI(t *testing.T) {
	f := newHandshakeFixture(t)
	w := httptest.NewRecorder()
	var redirected bool
	f.c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = f.c.HandshakeRedirect(w, r) })).ServeHTTP(w, pageRequest("/?q=charts", f.lapsed(t)))

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

	_, again := f.start(t, "/")
	assert.NotEqual(t, state, again["__Host-exhibit_clerk_hs"], "every handshake gets its own state")
}

func TestHandshakeRedirectDeclinesWhatItCannotRenew(t *testing.T) {
	f := newHandshakeFixture(t)
	with := func(edit func(map[string]string)) map[string]string {
		jar := f.lapsed(t)
		edit(jar)
		return jar
	}
	forger, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tests := map[string]func() *http.Request{
		"signed out of Clerk": func() *http.Request {
			return pageRequest("/", with(func(j map[string]string) { j[clientUATCookie] = "0" }))
		},
		"never signed in": func() *http.Request {
			return pageRequest("/", with(func(j map[string]string) { delete(j, clientUATCookie) }))
		},
		"no lapsed token to renew": func() *http.Request {
			return pageRequest("/", with(func(j map[string]string) { delete(j, "__session") }))
		},
		"a lapsed token nobody signed": func() *http.Request {
			return pageRequest("/", with(func(j map[string]string) {
				j["__session"] = f.token(t, forger, "ins_key", nil, map[string]any{"iss": f.c.FrontendAPI, "sub": "user_one"})
			}))
		},
		"not a page load": func() *http.Request {
			r := pageRequest("/partials/gallery-grid", f.lapsed(t))
			r.Header.Set("Sec-Fetch-Dest", "empty")
			return r
		},
		"not a GET": func() *http.Request {
			r := pageRequest("/", f.lapsed(t))
			r.Method = http.MethodPost
			return r
		},
		"development browser unknown": func() *http.Request {
			return pageRequest("/", with(func(j map[string]string) { delete(j, devBrowserCookie) }))
		},
		"already tried three times": func() *http.Request {
			return pageRequest("/", with(func(j map[string]string) { j[handshakeCountCookie] = "3" }))
		},
	}
	for name, request := range tests {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			assert.False(t, f.c.HandshakeRedirect(w, request()))
			assert.Empty(t, w.Header().Get("Location"))
			assert.Empty(t, w.Result().Cookies())
		})
	}
}

func TestHandshakeAcceptsHTMLWithoutFetchMetadata(t *testing.T) {
	f := newHandshakeFixture(t)
	r := pageRequest("/", f.lapsed(t))
	r.Header.Del("Sec-Fetch-Dest")
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	assert.True(t, f.c.HandshakeRedirect(httptest.NewRecorder(), r))
}

func TestHandshakeReturnSetsCookiesAndCleansURL(t *testing.T) {
	f := newHandshakeFixture(t)
	f.directives = f.renewal(t, "user_one")
	back, jar := f.start(t, "/?q=charts")
	w := f.finish(back, url.Values{handshakeNonceParam: {"nonce_example"}, devBrowserCookie: {"dvb_new"}}, jar)

	assert.Equal(t, 1, f.nonceCalls)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "https://app.test/?q=charts", w.Header().Get("Location"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	set := w.Header().Values("Set-Cookie")
	for _, d := range f.directives {
		assert.Contains(t, set, d)
	}
	spent := setCookies(w)["__Host-exhibit_clerk_hs"]
	require.NotNil(t, spent, "the state is spent")
	assert.Negative(t, spent.MaxAge)
}

// Review of Exh-8409: the return is bound to the browser and the flow that
// started it. A return that cannot show that is refused, whatever it carries.
func TestHandshakeReturnIsBoundToTheFlowThatStartedIt(t *testing.T) {
	f := newHandshakeFixture(t)
	f.directives = f.renewal(t, "user_one")
	back, jar := f.start(t, "/")
	forged := *back
	fq := forged.Query()
	fq.Set(handshakeStateParam, "guessed")
	forged.RawQuery = fq.Encode()

	for name, tc := range map[string]struct {
		back *url.URL
		jar  map[string]string
	}{
		// The attacker's link, in a browser that never started a handshake.
		"no handshake started": {back, f.lapsed(t)},
		// The browser is mid-handshake, but the link is not that handshake's
		// return.
		"another flow's state": {&forged, jar},
		// A sibling domain can plant a cookie without the __Host- prefix.
		"a planted cookie": {&forged, map[string]string{"exhibit_clerk_hs": "guessed"}},
	} {
		t.Run(name, func(t *testing.T) {
			w := f.finish(tc.back, withNonce, tc.jar)
			assert.Equal(t, 0, f.nonceCalls)
			assert.False(t, sessionSet(w))
			assert.Equal(t, "3", setCookies(w)[handshakeCountCookie].Value, "the page goes to the login page")
		})
	}

	// A genuine return is completed once: replaying it is refused.
	assert.True(t, sessionSet(f.finish(back, withNonce, jar)))
	assert.Equal(t, 1, f.nonceCalls)
	jar["__Host-exhibit_clerk_hs"] = "" // what the browser holds after the spend
	assert.False(t, sessionSet(f.finish(back, withNonce, jar)))
	assert.Equal(t, 1, f.nonceCalls)
}

// Review of Exh-8409, round two: the state proves the return is this
// browser's handshake, not that what it carries is this browser's session.
// Whatever an attacker smuggles in, the renewal must be the lapsed session's
// own user.
func TestHandshakeOnlyRenewsTheLapsedSessionsUser(t *testing.T) {
	t.Run("a result on the link that starts the handshake is dropped", func(t *testing.T) {
		f := newHandshakeFixture(t)
		back, _ := f.start(t, "/?__clerk_handshake_nonce=attacker_nonce&__clerk_handshake=attacker.jwt&q=1")
		assert.Empty(t, back.Query().Get(handshakeNonceParam))
		assert.Empty(t, back.Query().Get(handshakeTokenParam))
		assert.Equal(t, "1", back.Query().Get("q"))
	})
	for name, deliver := range map[string]func(jar map[string]string) url.Values{
		"in the URL": func(map[string]string) url.Values { return withNonce },
		// A cookie planted on the parent domain, with the genuine state.
		"in a planted cookie": func(jar map[string]string) url.Values {
			jar[handshakeNonceParam] = "nonce_example"
			return nil
		},
	} {
		t.Run("someone else's session delivered "+name, func(t *testing.T) {
			f := newHandshakeFixture(t)
			f.directives = f.renewal(t, "user_attacker")
			back, jar := f.start(t, "/")
			w := f.finish(back, deliver(jar), jar)
			assert.Equal(t, 1, f.nonceCalls)
			assert.False(t, sessionSet(w), "the attacker's session is not set")
			assert.Equal(t, "3", setCookies(w)[handshakeCountCookie].Value)
		})
	}
	// Review of Exh-8409, round three: every session cookie in the bundle is
	// checked, and each must replace the one the server reads.
	for name, directives := range map[string]func(f *handshakeFixture) []string{
		"a second session for someone else": func(f *handshakeFixture) []string {
			return append(f.renewal(t, "user_one"), "__session="+f.session(t, "user_attacker", false)+"; Path=/")
		},
		"a suffixed session for someone else": func(f *handshakeFixture) []string {
			return append(f.renewal(t, "user_one"), "__session_V5P0="+f.session(t, "user_attacker", false)+"; Path=/")
		},
		"a session scoped to another domain": func(f *handshakeFixture) []string {
			return []string{"__session=" + f.session(t, "user_one", false) + "; Path=/; Domain=app.test"}
		},
		"a session scoped to another path": func(f *handshakeFixture) []string {
			return []string{"__session=" + f.session(t, "user_one", false) + "; Path=/admin"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHandshakeFixture(t)
			f.directives = directives(f)
			back, jar := f.start(t, "/")
			w := f.finish(back, withNonce, jar)
			assert.False(t, sessionSet(w))
			assert.Equal(t, "3", setCookies(w)[handshakeCountCookie].Value)
		})
	}
	t.Run("a renewal with no session sets nothing", func(t *testing.T) {
		f := newHandshakeFixture(t)
		f.directives = []string{"__client_uat=0; Path=/"}
		back, jar := f.start(t, "/")
		w := f.finish(back, withNonce, jar)
		assert.NotContains(t, w.Header().Values("Set-Cookie"), "__client_uat=0; Path=/")
	})
}

func TestHandshakeFailureFallsBackWithoutCookies(t *testing.T) {
	for name, tc := range map[string]struct {
		directives func(*handshakeFixture) []string
		status     int
	}{
		"backend refuses": {func(f *handshakeFixture) []string { return f.renewal(t, "user_one") }, http.StatusBadRequest},
		"directive is not a header value": {func(f *handshakeFixture) []string {
			return []string{"__session=" + f.session(t, "user_one", false) + "\r\nLocation: https://evil.test"}
		}, http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHandshakeFixture(t)
			f.directives, f.status = tc.directives(f), tc.status
			back, jar := f.start(t, "/artifacts/a1")
			w := f.finish(back, withNonce, jar)

			assert.Equal(t, 1, f.nonceCalls)
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
	f := newHandshakeFixture(t)
	f.directives = f.renewal(t, "user_one")
	back, jar := f.start(t, "/")
	jar[handshakeNonceParam] = "nonce_example"
	assert.True(t, sessionSet(f.finish(back, nil, jar)))
	assert.Equal(t, 1, f.nonceCalls)

	reached := false
	f.c.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(httptest.NewRecorder(), pageRequest("/", jar))
	assert.True(t, reached, "a leftover nonce cookie is ignored outside a handshake's return")
	assert.Equal(t, 1, f.nonceCalls)
}

// Review of Exh-8409: the cleaned URL is a path on the app's own origin even
// when the requested path looks like a link to another one.
func TestHandshakeReturnNeverRedirectsOffSite(t *testing.T) {
	f := newHandshakeFixture(t)
	f.status = http.StatusBadRequest
	for _, path := range []string{"//evil.test/path", "/\\evil.test/path"} {
		back, jar := f.start(t, path)
		loc := f.finish(back, withNonce, jar).Header().Get("Location")
		assert.True(t, strings.HasPrefix(loc, "https://app.test/"), loc)
	}
}

// A development Frontend API answers with the directives in a JWT signed by
// the instance's key, whatever format was asked for.
func TestHandshakeTokenIsVerifiedBeforeItsCookiesAreSet(t *testing.T) {
	f := newHandshakeFixture(t)
	forger, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	renewal := f.renewal(t, "user_one")
	handshake := func(key *rsa.PrivateKey, category string) string {
		return f.token(t, key, "ins_key", map[string]string{"cat": category}, map[string]any{"handshake": renewal})
	}
	for _, tt := range []struct {
		name  string
		token string
		set   bool
	}{
		{"signed by the instance", handshake(f.key, "cl_B7d4PD111AAA"), true},
		{"signed by anyone else", handshake(forger, "cl_B7d4PD111AAA"), false},
		{"a machine token's category", handshake(f.key, "cl_B7d4PD333AAA"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			back, jar := f.start(t, "/")
			w := f.finish(back, url.Values{handshakeTokenParam: {tt.token}}, jar)
			require.Equal(t, http.StatusFound, w.Code)
			assert.Equal(t, "https://app.test/", w.Header().Get("Location"))
			assert.Equal(t, tt.set, sessionSet(w))
		})
	}
	assert.Equal(t, 1, f.jwksCalls, "the signing key is fetched once and kept")

	// A token naming a key nobody has does not buy a Backend API call per
	// request.
	unknown := f.token(t, forger, "made_up", nil, map[string]any{"handshake": renewal})
	for range 5 {
		back, jar := f.start(t, "/")
		f.finish(back, url.Values{handshakeTokenParam: {unknown}}, jar)
	}
	assert.Equal(t, 1, f.jwksCalls, "unknown key ids refetch at most every five minutes")
}

// Review of Exh-8409, round two: keys are trusted for an hour after the last
// successful fetch, and a failed refresh does not extend that.
func TestHandshakeSigningKeysExpireAndFailedRefreshesDoNotRenewThem(t *testing.T) {
	f := newHandshakeFixture(t)
	ctx := t.Context()
	_, err := f.c.signingKey(ctx, "ins_key")
	require.NoError(t, err)
	require.Equal(t, 1, f.jwksCalls)

	// An hour on, with the Backend API down: the old key is not trusted.
	f.c.keysFetched = f.c.keysFetched.Add(-61 * time.Minute)
	f.c.keysAttempted = f.c.keysFetched
	f.jwksDown = true
	_, err = f.c.signingKey(ctx, "ins_key")
	require.Error(t, err)
	require.Equal(t, 2, f.jwksCalls)
	_, err = f.c.signingKey(ctx, "ins_key")
	require.Error(t, err, "still not trusted after the failed refresh")
	require.Equal(t, 2, f.jwksCalls, "and the failure is not retried at once")

	// Recovery is retried soon, not five minutes later.
	f.jwksDown = false
	f.c.keysAttempted = f.c.keysAttempted.Add(-11 * time.Second)
	_, err = f.c.signingKey(ctx, "ins_key")
	require.NoError(t, err)
	require.Equal(t, 3, f.jwksCalls)

	// Review of Exh-8409, round three: a refresh that fails while the set is
	// fresh is retried as soon as one that fails after it expired. (The set
	// is six minutes old, so an unknown id may refetch.)
	f.c.keysFetched = f.c.keysFetched.Add(-6 * time.Minute)
	f.c.keysAttempted = f.c.keysFetched
	f.jwksDown = true
	_, err = f.c.signingKey(ctx, "rotated_in")
	require.Error(t, err)
	require.Equal(t, 4, f.jwksCalls)
	f.c.keysAttempted = f.c.keysAttempted.Add(-11 * time.Second)
	_, err = f.c.signingKey(ctx, "rotated_in")
	require.Error(t, err)
	require.Equal(t, 5, f.jwksCalls)
}
