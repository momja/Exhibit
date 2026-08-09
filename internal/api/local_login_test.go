package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Local credential login (av-q30x). The gap it closes: before this, an
// instance with no OIDC issuer had no login gate at all, so securing a
// self-hosted library meant running an identity server or putting auth in the
// proxy. These tests hold the two halves of the fix — that a credential arms
// the same gate, and that it lands on the same session layer the provider
// callback uses.

const (
	testUsername = "curator"
	testPassword = "a-long-enough-passphrase"
)

// testHash runs the same hashing the operator's `server hash-password` runs —
// production function, production cost. The tests below are slower for it and
// that is the point: the cost parameter is part of what is being asserted.
func testHash(t *testing.T, password string) string {
	t.Helper()
	h, err := auth.HashPassword(password)
	require.NoError(t, err)
	return h
}

func newTestCredential(t *testing.T, username, password string) *auth.Credential {
	t.Helper()
	cred, err := auth.NewCredential(username, testHash(t, password))
	require.NoError(t, err)
	return cred
}

func newLocalLoginRouter(t *testing.T) (*Router, store.Store) {
	t.Helper()
	return newLoginTestRouter(t, nil, newTestCredential(t, testUsername, testPassword))
}

// submitLogin posts the login form and returns the response.
func submitLogin(t *testing.T, ro *Router, username, password, next string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	if next != "" {
		form.Set("next", next)
	}
	req := httptest.NewRequest("POST", "/auth/local", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

// runLocalLogin walks the successful path and returns the session cookie.
func runLocalLogin(t *testing.T, ro *Router) *http.Cookie {
	t.Helper()
	w := submitLogin(t, ro, testUsername, testPassword, "")
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	session := cookiesFrom(w)[sessionCookieName]
	require.NotNil(t, session)
	require.NotEmpty(t, session.Value)
	return session
}

// --- the gate ----------------------------------------------------------

// The actual fix. identityEnabled() used to be the only thing that armed
// sessionGate, so a self-hoster who set a username and password would still
// have been serving every page to anyone who could reach the app origin.
func TestLocalCredentialArmsTheSessionGate(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)

	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/xyz", nil))
	require.Equal(t, http.StatusFound, w.Code, "an unauthenticated page request must not be served")
	assert.Equal(t, "/auth/login?next=%2Fartifacts%2Fxyz", w.Header().Get("Location"))

	// And the API refuses an unauthenticated request exactly as before.
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/api/artifacts", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	session := runLocalLogin(t, ro)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// The cookie authenticates the API too, with no Authorization header.
	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// The bearer token is the API/CLI credential and this feature replaces the
// *browser* credential only — seed scripts, e2e fixtures and the future
// extension have no browser to log in with.
func TestStaticTokenStillWorksAlongsideLocalLogin(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	req := httptest.NewRequest("GET", "/api/artifacts", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// --- the credential ----------------------------------------------------

func TestWrongCredentialIsRejected(t *testing.T) {
	ro, st := newLocalLoginRouter(t)

	for _, tc := range []struct{ name, user, pass string }{
		{"wrong password", testUsername, "not-the-passphrase"},
		{"wrong username", "someone-else", testPassword},
		{"both wrong", "someone-else", "not-the-passphrase"},
		{"empty", "", ""},
		// The stored value is a bcrypt hash; submitting it must not
		// authenticate, or the hash would be the password.
		{"the hash itself", testUsername, testHash(t, testPassword)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := submitLogin(t, ro, tc.user, tc.pass, "")
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Nil(t, cookiesFrom(w)[sessionCookieName], "no session is issued")
			// One message whichever field was wrong.
			assert.Contains(t, w.Body.String(), "don&#39;t match")
		})
	}

	// Nothing was recorded: a failed login creates no user and no session.
	_, err := st.GetUser(context.Background(), 1)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// The password must not be recoverable from the page a failed attempt renders.
// The username is echoed back so the form is not cleared; the password never is.
func TestFailedLoginDoesNotEchoThePassword(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	w := submitLogin(t, ro, "someone", "hunter2-the-secret", "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.NotContains(t, w.Body.String(), "hunter2-the-secret")
	assert.Contains(t, w.Body.String(), "someone", "the username is kept so the form is not cleared")
}

// bcrypt is doing the comparison, which is what makes it constant-time in the
// password and salted at rest. Asserted at the credential rather than through
// the handler because that is where the property lives.
func TestCredentialComparesWithBcrypt(t *testing.T) {
	hash := testHash(t, testPassword)
	assert.NotContains(t, hash, testPassword, "the password is not recoverable from the hash")
	assert.True(t, strings.HasPrefix(hash, "$2"), "bcrypt's own format, so the cost travels with the hash")

	// A second hash of the same password differs — per-call salt.
	assert.NotEqual(t, hash, testHash(t, testPassword))

	cred := newTestCredential(t, testUsername, testPassword)
	assert.True(t, cred.Verify(testUsername, testPassword))
	assert.False(t, cred.Verify(testUsername, testPassword+"!"))
	assert.False(t, cred.Verify(strings.ToUpper(testUsername), testPassword))
}

// A hash is required, not a password: an instance that accepted plaintext here
// would have nothing to hash. Rejecting it at construction means the operator
// finds out at startup rather than at their first login.
func TestNewCredentialRejectsAnythingButABcryptHash(t *testing.T) {
	for _, tc := range []struct{ name, user, hash string }{
		{"plaintext password", testUsername, testPassword},
		{"empty hash", testUsername, ""},
		{"empty username", "", testHash(t, testPassword)},
		{"truncated hash", testUsername, testHash(t, testPassword)[:20]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.NewCredential(tc.user, tc.hash)
			assert.Error(t, err)
		})
	}
}

// --- the shared session layer ------------------------------------------

// The structural claim of av-q30x: local credential is a second *login path*,
// not a second session mechanism. A local login must produce a session
// indistinguishable from a provider's — same cookie, same attributes, same
// row shape — because everything downstream reads only that.
func TestLocalAndProviderLoginsProduceTheSameSession(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	oidcRouter, oidcStore := newLoginTestRouter(t, idp, nil)
	localRouter, localStore := newLocalLoginRouter(t)

	oidcCookie := runLogin(t, oidcRouter)
	localCookie := runLocalLogin(t, localRouter)

	// The cookie: same name, same attributes, including the one that is the
	// whole CSRF defence (av-ke2m).
	assert.Equal(t, oidcCookie.Name, localCookie.Name)
	assert.Equal(t, oidcCookie.HttpOnly, localCookie.HttpOnly)
	assert.Equal(t, oidcCookie.Secure, localCookie.Secure)
	assert.Equal(t, oidcCookie.Path, localCookie.Path)
	assert.Empty(t, localCookie.Domain, "host-only — never reachable from the render origin")
	assertCSRFDefence(t, localCookie, sessionCookieName)

	// The row: same user id, and the same lifetime policy — av-30rj's, not a
	// second one.
	oidcSession, err := oidcStore.GetSession(context.Background(), oidcCookie.Value)
	require.NoError(t, err)
	localSession, err := localStore.GetSession(context.Background(), localCookie.Value)
	require.NoError(t, err)
	assert.Equal(t, oidcSession.UserID, localSession.UserID)
	assert.WithinDuration(t, oidcSession.ExpiresAt, localSession.ExpiresAt, time.Minute)
	assert.True(t, localSession.ExpiresAt.After(time.Now().Add(29*24*time.Hour)),
		"DefaultSessionTTL, inherited rather than redefined")
	assert.Equal(t, len(oidcCookie.Value), len(localCookie.Value),
		"the same opaque session id, not a credential format of its own")
}

// A local login creates the users row the same way a provider identity does,
// so owner_id works identically — and the row is keyed on a constant, so
// renaming the login relabels the owner rather than orphaning the library.
func TestLocalLoginCreatesTheUserRowAndSurvivesARename(t *testing.T) {
	ro, st := newLocalLoginRouter(t)
	runLocalLogin(t, ro)

	user, err := st.GetUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, auth.LocalExternalID, user.ExternalID)
	assert.Equal(t, testUsername, user.Email, "the username is the row's human-readable handle")

	// A second login reuses the row rather than minting owner 2.
	runLocalLogin(t, ro)
	_, err = st.GetUser(context.Background(), 2)
	assert.ErrorIs(t, err, store.ErrNotFound)

	// The operator renames the login and restarts — same database, new
	// LOGIN_USERNAME.
	ro.cfg.LocalCredential = newTestCredential(t, "archivist", testPassword)
	w := submitLogin(t, ro, "archivist", testPassword, "")
	require.Equal(t, http.StatusFound, w.Code)
	user, err = st.GetUser(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "archivist", user.Email)
	_, err = st.GetUser(context.Background(), 2)
	assert.ErrorIs(t, err, store.ErrNotFound, "the library is not orphaned by a rename")
}

// The property av-30rj established, now for this path too: logout is a
// deletion, so the credential dies on the next request rather than at a TTL.
func TestLocalLogoutRevokesImmediately(t *testing.T) {
	ro, st := newLocalLoginRouter(t)
	session := runLocalLogin(t, ro)

	row, err := st.GetSession(context.Background(), session.Value)
	require.NoError(t, err)
	require.True(t, row.ExpiresAt.After(time.Now().Add(29*24*time.Hour)),
		"expiry is weeks out, so nothing below is explained by the TTL")

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(session)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Less(t, cookiesFrom(w)[sessionCookieName].MaxAge, 0)

	_, err = st.GetSession(context.Background(), session.Value)
	assert.ErrorIs(t, err, store.ErrNotFound, "revoked server-side, not just dropped from the browser")

	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "a copy of the cookie is worthless immediately")

	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(session)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "/auth/login")
}

// --- the page ----------------------------------------------------------

// What is offered must match what is configured: no dead SSO button on an
// instance with no provider, and no bare redirect on one with a password.
func TestLoginPageOffersOnlyTheConfiguredPaths(t *testing.T) {
	t.Run("local only", func(t *testing.T) {
		ro, _ := newLocalLoginRouter(t)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/login", nil))
		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `action="/auth/local"`)
		assert.Contains(t, body, `type="password"`)
		assert.NotContains(t, body, "/auth/sso", "there is no provider to continue to")
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

		// The route does not exist either, so a hand-typed URL 404s rather
		// than starting a flow that cannot complete.
		w = httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/sso", nil))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("provider only keeps the straight redirect", func(t *testing.T) {
		ro, _ := newLoginTestRouter(t, &stubProvider{loginURL: "https://idp.test/authorize"}, nil)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/login", nil))
		require.Equal(t, http.StatusFound, w.Code, "a page whose only control is 'continue' is not a choice")
		assert.Contains(t, w.Header().Get("Location"), "https://idp.test/authorize")

		w = httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("POST", "/auth/local", nil))
		assert.Equal(t, http.StatusNotFound, w.Code, "no credential is configured, so there is nothing to post to")
	})

	t.Run("both", func(t *testing.T) {
		idp := &stubProvider{
			loginURL:   "https://idp.test/authorize",
			identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}},
		}
		ro, st := newLoginTestRouter(t, idp, newTestCredential(t, testUsername, testPassword))

		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/login?next=/artifacts/abc", nil))
		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `action="/auth/local"`)
		assert.Contains(t, body, "/auth/sso?next=%2Fartifacts%2Fabc")
		assert.Contains(t, body, `value="/artifacts/abc"`, "the destination survives the form post too")

		// Both work, and both land the same kind of session.
		local := runLocalLogin(t, ro)
		sso := runSSOLogin(t, ro)
		localRow, err := st.GetSession(context.Background(), local.Value)
		require.NoError(t, err)
		ssoRow, err := st.GetSession(context.Background(), sso.Value)
		require.NoError(t, err)
		assert.NotEqual(t, localRow.UserID, ssoRow.UserID,
			"two identities are two owners — the provider subject is not the local credential")
	})
}

// runSSOLogin is runLogin's twin for an instance where /auth/login renders the
// page instead of redirecting, so the flow starts at /auth/sso.
func runSSOLogin(t *testing.T, ro *Router) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/sso", nil))
	require.Equal(t, http.StatusFound, w.Code)
	flow := cookiesFrom(w)

	cb := httptest.NewRequest("GET", "/auth/callback?code=c&state="+
		url.QueryEscape(flow[stateCookieName].Value), nil)
	cb.AddCookie(flow[stateCookieName])
	cb.AddCookie(flow[verifierCookieName])
	cw := httptest.NewRecorder()
	ro.ServeHTTP(cw, cb)
	require.Equal(t, http.StatusFound, cw.Code, cw.Body.String())
	session := cookiesFrom(cw)[sessionCookieName]
	require.NotNil(t, session)
	return session
}

// After signing in, a visitor lands where they were going — and only ever on
// this origin. The `next` value arrives in a form field an attacker controls,
// so it goes through the same safeNext as the query-parameter path.
func TestLocalLoginHonoursNextButOnlyOnThisOrigin(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)

	w := submitLogin(t, ro, testUsername, testPassword, "/artifacts/abc?q=1")
	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/artifacts/abc?q=1", w.Header().Get("Location"))

	for _, bad := range []string{"https://evil.test/", "//evil.test/", "/\\evil.test"} {
		w := submitLogin(t, ro, testUsername, testPassword, bad)
		require.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"), bad)
	}
}

// The login page is where an unauthenticated visitor is sent, so it must be
// reachable — gate, assets and all — without a credential.
func TestLoginPageIsReachableUnauthenticated(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	for _, path := range []string{"/auth/login", "/assets/gallery/login.css"} {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		assert.Equal(t, http.StatusOK, w.Code, path)
	}
}
