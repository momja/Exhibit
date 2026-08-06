package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider is a second identity provider, implementing the same two-method
// seam as the OIDC one. It exists to prove the claim the seam is for: a
// provider is a constructor, and plugging a different one in changes nothing
// anywhere else — no route, no cookie, no store call, no handler.
type stubProvider struct {
	loginURL   string
	identities []auth.Identity // handed out in order, one per exchange
	exchanges  int
	err        error

	lastState    string
	lastVerifier string
	lastCode     string
}

func (s *stubProvider) AuthURL(state, verifier string) string {
	s.lastState, s.lastVerifier = state, verifier
	return s.loginURL + "?state=" + url.QueryEscape(state) +
		"&code_challenge=" + url.QueryEscape(auth.S256Challenge(verifier))
}

func (s *stubProvider) Exchange(_ context.Context, code, verifier string) (*auth.Identity, error) {
	s.lastCode, s.lastVerifier = code, verifier
	if s.err != nil {
		return nil, s.err
	}
	id := s.identities[min(s.exchanges, len(s.identities)-1)]
	s.exchanges++
	return &id, nil
}

var _ auth.IdentityProvider = (*stubProvider)(nil)

func newIdentityTestRouter(t *testing.T, idp auth.IdentityProvider) (*Router, store.Store) {
	t.Helper()

	f, err := os.CreateTemp("", "test-auth-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	blobDir, err := os.MkdirTemp("", "test-auth-blobs-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(blobDir) })

	bl, err := blob.NewFSStore(blobDir)
	require.NoError(t, err)

	box, err := secrets.Load("test-secret", "")
	require.NoError(t, err)

	return NewRouter(Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    "https://app.test",
		RenderOrigin: "https://render.test",
		AuthToken:    "secret",
		Secrets:      box,
		Identity:     idp,
	}), st
}

// cookiesFrom collects a response's Set-Cookie headers by name, keeping the
// last write of each — which is how a browser resolves them too.
func cookiesFrom(w *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range (&http.Response{Header: w.Header()}).Cookies() {
		out[c.Name] = c
	}
	return out
}

// runLogin walks the whole flow — /auth/login, then the provider's redirect
// back to /auth/callback — and returns the session cookie it lands.
func runLogin(t *testing.T, ro *Router) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)

	flow := cookiesFrom(w)
	state := flow[stateCookieName]
	require.NotNil(t, state)
	require.NotNil(t, flow[verifierCookieName])

	cb := httptest.NewRequest("GET", "/auth/callback?code=the-code&state="+url.QueryEscape(state.Value), nil)
	cb.AddCookie(state)
	cb.AddCookie(flow[verifierCookieName])
	cw := httptest.NewRecorder()
	ro.ServeHTTP(cw, cb)
	require.Equal(t, http.StatusFound, cw.Code, "callback should land the session: %s", cw.Body.String())

	session := cookiesFrom(cw)[sessionCookieName]
	require.NotNil(t, session)
	require.NotEmpty(t, session.Value)
	return session
}

// --- The default: no identity provider configured ----------------------

// A self-hoster's instance must be untouched by this feature existing. With no
// provider, the /auth routes are not registered at all, the pages stay open,
// and the static token is the only credential.
func TestNoIdentityProviderLeavesSingleUserBehaviourAlone(t *testing.T) {
	ro := newTestRouter(t) // the shared helper: no Identity in its Config

	for _, path := range []string{"/auth/login", "/auth/callback", "/auth/logout"} {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		assert.Equal(t, http.StatusNotFound, w.Code, path)
		assert.Contains(t, w.Body.String(), "<!DOCTYPE html>",
			"an unconfigured instance answers /auth/* with the ordinary styled 404, like any other unrouted path")
	}

	// Pages: open, no redirect to a login that does not exist.
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	// API: the static token, exactly as before.
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/api/artifacts", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req := httptest.NewRequest("GET", "/api/artifacts", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// And a stray session cookie is not a credential when nothing issues them.
	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "made-up"})
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOwnerStaysOneWithoutIdentityProvider(t *testing.T) {
	ro := newTestRouter(t)
	req := httptest.NewRequest("GET", "/api/artifacts", nil)
	req.Header.Set("Authorization", authHeader())
	var seen int64
	// The chain the API group installs, read through a handler of our own.
	handler := ro.authMiddleware(ownerMiddleware(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { seen = ownerIDFromCtx(r.Context()) })))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, defaultOwnerID, seen)
}

// --- With a provider configured ----------------------------------------

func TestLoginCreatesUserJustInTimeAndStartsSession(t *testing.T) {
	idp := &stubProvider{
		loginURL:   "https://idp.test/authorize",
		identities: []auth.Identity{{ExternalID: "sub-1", Email: "first@example.test"}},
	}
	ro, st := newIdentityTestRouter(t, idp)

	// /auth/login hands the browser to the provider with state and a PKCE
	// challenge derived from the verifier it parked in a cookie.
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/login?next=/artifacts/abc", nil))
	require.Equal(t, http.StatusFound, w.Code)
	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	flow := cookiesFrom(w)
	assert.Equal(t, flow[stateCookieName].Value, loc.Query().Get("state"))
	assert.Equal(t, auth.S256Challenge(flow[verifierCookieName].Value), loc.Query().Get("code_challenge"))
	assert.Equal(t, "/artifacts/abc", flow[nextCookieName].Value)
	for _, name := range []string{stateCookieName, verifierCookieName, nextCookieName} {
		assert.True(t, flow[name].HttpOnly, name)
		assert.True(t, flow[name].Secure, name+" on an https app origin")
		assertCSRFDefence(t, flow[name], name)
		assert.Empty(t, flow[name].Domain, name+" must be host-only — never reachable from the render origin")
	}

	// The callback exchanges once and issues our session.
	cb := httptest.NewRequest("GET", "/auth/callback?code=abc123&state="+
		url.QueryEscape(flow[stateCookieName].Value), nil)
	cb.AddCookie(flow[stateCookieName])
	cb.AddCookie(flow[verifierCookieName])
	cb.AddCookie(flow[nextCookieName])
	cw := httptest.NewRecorder()
	ro.ServeHTTP(cw, cb)
	require.Equal(t, http.StatusFound, cw.Code)
	assert.Equal(t, "/artifacts/abc", cw.Header().Get("Location"), "returns to the page that triggered the login")
	assert.Equal(t, 1, idp.exchanges, "the provider is touched exactly once, at the callback")
	assert.Equal(t, "abc123", idp.lastCode)
	assert.Equal(t, flow[verifierCookieName].Value, idp.lastVerifier)

	landed := cookiesFrom(cw)
	session := landed[sessionCookieName]
	require.NotNil(t, session)
	assert.True(t, session.HttpOnly)
	assert.True(t, session.Secure)
	assert.Empty(t, session.Domain)
	// The credential a forged cross-site request would ride (av-ke2m).
	assertCSRFDefence(t, session, sessionCookieName)
	// The single-use flow cookies are cleared once redeemed.
	for _, name := range []string{stateCookieName, verifierCookieName, nextCookieName} {
		require.NotNil(t, landed[name], name)
		assert.Less(t, landed[name].MaxAge, 0, name+" is cleared at the callback")
	}

	// The user row was created on first login, with the email stored beside
	// the provider subject.
	user, err := st.GetUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "sub-1", user.ExternalID)
	assert.Equal(t, "first@example.test", user.Email)

	// A second login for the same identity reuses the row rather than
	// minting a new owner.
	runLogin(t, ro)
	_, err = st.GetUser(context.Background(), 2)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestSessionAuthenticatesPagesAndAPI(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	ro, _ := newIdentityTestRouter(t, idp)

	// Unauthenticated: pages redirect into the login flow, carrying where
	// the visitor was going.
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/xyz", nil))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/auth/login?next=%2Fartifacts%2Fxyz", w.Header().Get("Location"))

	// Unauthenticated htmx fragment: a status, not a redirect to an HTML
	// page it would happily swap into the document.
	req := httptest.NewRequest("GET", "/partials/card-widget?id=xyz", nil)
	req.Header.Set("HX-Request", "true")
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	session := runLogin(t, ro)

	// Authenticated page.
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Authenticated API with no Authorization header at all — the cookie is
	// the credential.
	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// The static token still works, for API and CLI clients that have no
	// browser to log in with.
	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// The point of owning the session rather than trusting a provider-signed
// token: logout is a deletion, so the credential dies on the next request
// instead of whenever a TTL would have expired it.
func TestLogoutRevokesImmediatelyNotAtTTL(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	ro, st := newIdentityTestRouter(t, idp)
	session := runLogin(t, ro)

	// The session is nowhere near its expiry — a signed token with these
	// claims would stay valid for another month.
	row, err := st.GetSession(context.Background(), session.Value)
	require.NoError(t, err)
	assert.True(t, row.ExpiresAt.After(time.Now().Add(29*24*time.Hour)),
		"expiry is weeks out, so nothing below can be explained by the TTL")

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(session)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Less(t, cookiesFrom(w)[sessionCookieName].MaxAge, 0)

	// Server-side: the row is gone, so the value in any copy of that cookie
	// is now worthless.
	_, err = st.GetSession(context.Background(), session.Value)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// Replaying the cookie a browser might still hold gets nothing.
	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "/auth/login")
}

func TestExpiredSessionIsRejectedAndPruned(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1"}}}
	ro, st := newIdentityTestRouter(t, idp)

	user, err := st.UpsertUser(context.Background(), "sub-1", "a@example.test")
	require.NoError(t, err)
	require.NoError(t, st.CreateSession(context.Background(), &store.Session{
		ID: "stale-session", UserID: user.ID, ExpiresAt: time.Now().Add(-time.Hour),
	}))
	_, err = st.GetSession(context.Background(), "stale-session")
	assert.ErrorIs(t, err, store.ErrNotFound, "expired reads the same as revoked")

	req := httptest.NewRequest("GET", "/api/artifacts", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "stale-session"})
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	n, err := st.DeleteExpiredSessions(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

// Owner comes from the session, so two identities are two owners — the seam
// reaches all the way to owner_id without any provider-specific value doing so.
func TestSessionOwnerScopesWrites(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{
		{ExternalID: "sub-1", Email: "one@example.test"},
		{ExternalID: "sub-2", Email: "two@example.test"},
	}}
	ro, st := newIdentityTestRouter(t, idp)

	first := runLogin(t, ro)
	second := runLogin(t, ro)
	assert.NotEqual(t, first.Value, second.Value)

	create := func(session *http.Cookie, name string) {
		req := httptest.NewRequest("POST", "/api/collections",
			strings.NewReader(`{"name":"`+name+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(session)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}
	create(first, "one's shelf")
	create(second, "two's shelf")

	firstCollections, err := st.ListCollections(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, firstCollections, 1)
	assert.Equal(t, "one's shelf", firstCollections[0].Name)

	secondCollections, err := st.ListCollections(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, secondCollections, 1)
	assert.Equal(t, "two's shelf", secondCollections[0].Name)
}

func TestCallbackRejectsBadFlows(t *testing.T) {
	newFlow := func(t *testing.T, ro *Router) map[string]*http.Cookie {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/login", nil))
		return cookiesFrom(w)
	}

	t.Run("state mismatch", func(t *testing.T) {
		idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1"}}}
		ro, _ := newIdentityTestRouter(t, idp)
		flow := newFlow(t, ro)
		req := httptest.NewRequest("GET", "/auth/callback?code=c&state=not-the-state", nil)
		req.AddCookie(flow[stateCookieName])
		req.AddCookie(flow[verifierCookieName])
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, 0, idp.exchanges, "a mismatched state never reaches the provider")
	})

	t.Run("no flow cookies", func(t *testing.T) {
		ro, _ := newIdentityTestRouter(t, &stubProvider{})
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/callback?code=c&state=s", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("provider error", func(t *testing.T) {
		ro, _ := newIdentityTestRouter(t, &stubProvider{})
		flow := newFlow(t, ro)
		req := httptest.NewRequest("GET", "/auth/callback?error=access_denied&state="+
			url.QueryEscape(flow[stateCookieName].Value), nil)
		req.AddCookie(flow[stateCookieName])
		req.AddCookie(flow[verifierCookieName])
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("exchange fails", func(t *testing.T) {
		idp := &stubProvider{err: errors.New("token endpoint said no")}
		ro, st := newIdentityTestRouter(t, idp)
		flow := newFlow(t, ro)
		req := httptest.NewRequest("GET", "/auth/callback?code=c&state="+
			url.QueryEscape(flow[stateCookieName].Value), nil)
		req.AddCookie(flow[stateCookieName])
		req.AddCookie(flow[verifierCookieName])
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Nil(t, cookiesFrom(w)[sessionCookieName], "no session is issued")
		_, err := st.GetUser(context.Background(), 1)
		assert.ErrorIs(t, err, store.ErrNotFound, "a failed exchange creates no user")
	})
}

func TestCookiesFollowTheAppOriginScheme(t *testing.T) {
	// A plain-HTTP instance — the documented local default — must still be
	// able to log in; a Secure cookie there is silently dropped.
	ro := NewRouter(Config{AppOrigin: "http://localhost:8080"})
	assert.False(t, ro.cookieSecure())
	ro = NewRouter(Config{AppOrigin: "https://exhibit.example.com"})
	assert.True(t, ro.cookieSecure())
}

func TestSafeNextRejectsOffOriginDestinations(t *testing.T) {
	for _, bad := range []string{
		"", "https://evil.test/", "//evil.test/", "/\\evil.test", "evil.test", "javascript:alert(1)",
	} {
		assert.Empty(t, safeNext(bad), bad)
	}
	assert.Equal(t, "/artifacts/abc?q=1", safeNext("/artifacts/abc?q=1"))
}
