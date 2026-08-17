package api

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Credentials on the users table (av-rzvf). av-q30x put one username and one
// password hash in the environment, which is a login but not a user backend:
// an instance could be secured, and still only ever have one person in it.
// These tests hold what changed — accounts are rows, several can coexist, and
// an OIDC identity is one of the same rows with a different column filled in —
// and what must not have: an instance configured with neither is untouched.

// newAccountRouter builds an instance whose accounts live in the database.
// LocalUsers is what the server sets after counting them at startup.
func newAccountRouter(t *testing.T, cred *auth.Credential) (*Router, store.Store) {
	t.Helper()
	return newLoginTestRouter(t, nil, cred, func(c *Config) { c.LocalUsers = true })
}

// addAccount provisions an account exactly as `exhibit user add` does — same
// store call, same hashing — so these tests exercise the path an operator
// actually takes rather than a fixture that resembles it.
func addAccount(t *testing.T, st store.Store, name, password string) *store.User {
	t.Helper()
	user, err := st.CreateLocalUser(context.Background(), store.NewLocalUser{
		ExternalID: auth.LocalExternalID(name), Email: auth.NormalizeLoginName(name), PasswordHash: testHash(t, password),
	})
	require.NoError(t, err)
	return user
}

// --- more than one local user ------------------------------------------

// The point of the ticket. Two accounts provisioned by an operator, each
// logging in with its own password, each landing on its own owner id.
func TestSeveralLocalAccountsCanLogIn(t *testing.T) {
	ro, st := newAccountRouter(t, nil)
	alice := addAccount(t, st, "alice@example.test", "alice-long-passphrase")
	bob := addAccount(t, st, "BOB@example.test", "bob-long-passphrase")

	assert.NotEqual(t, alice.ID, bob.ID, "two accounts are two owners")
	assert.Equal(t, "bob@example.test", bob.Email,
		"the login name is normalized once, on the way in")

	for _, tc := range []struct {
		name, password string
		want           int64
	}{
		{"alice@example.test", "alice-long-passphrase", alice.ID},
		// Typed the way a person types it, not the way it is stored.
		{"Alice@Example.test", "alice-long-passphrase", alice.ID},
		{"bob@example.test", "bob-long-passphrase", bob.ID},
	} {
		w := submitLogin(t, ro, tc.name, tc.password, "")
		require.Equal(t, http.StatusFound, w.Code, tc.name)
		cookie := cookiesFrom(w)[sessionCookieName]
		require.NotNil(t, cookie, tc.name)
		sess, err := st.GetSession(context.Background(), cookie.Value)
		require.NoError(t, err)
		assert.Equal(t, tc.want, sess.UserID, tc.name)
	}

	// Each password unlocks one account and not the other.
	for _, tc := range []struct{ name, password string }{
		{"alice@example.test", "bob-long-passphrase"},
		{"bob@example.test", "alice-long-passphrase"},
		{"carol@example.test", "alice-long-passphrase"},
	} {
		w := submitLogin(t, ro, tc.name, tc.password, "")
		assert.Equal(t, http.StatusUnauthorized, w.Code, tc.name+"/"+tc.password)
		assert.Nil(t, cookiesFrom(w)[sessionCookieName])
	}
}

// Owner scoping is av-swzv's and already enforced, but it was enforced for
// identities a provider issued. Proving it holds for database-backed
// credentials is the difference between assuming the two produce the same kind
// of user and knowing it.
func TestEachLocalAccountSeesOnlyItsOwnLibrary(t *testing.T) {
	ro, st := newAccountRouter(t, nil)
	alice := addAccount(t, st, "alice@example.test", "alice-long-passphrase")
	bob := addAccount(t, st, "bob@example.test", "bob-long-passphrase")

	seedOwnedArtifact(t, ro, alice.ID, "alice-artifact", "Alice's ledger", "alice-body", "alice-state")
	seedOwnedArtifact(t, ro, bob.ID, "bob-artifact", "Bob's ledger", "bob-body", "bob-state")

	aliceCookie := loginAs(t, ro, "alice@example.test", "alice-long-passphrase")
	bobCookie := loginAs(t, ro, "bob@example.test", "bob-long-passphrase")

	// The gallery each one is served.
	aliceGallery := getWithCookie(t, ro, "/", aliceCookie).Body.String()
	assert.Contains(t, aliceGallery, "Alice&#39;s ledger")
	assert.NotContains(t, aliceGallery, "Bob&#39;s ledger")

	bobGallery := getWithCookie(t, ro, "/", bobCookie).Body.String()
	assert.Contains(t, bobGallery, "Bob&#39;s ledger")
	assert.NotContains(t, bobGallery, "Alice&#39;s ledger")

	// And the API, reached with the session cookie rather than the shared
	// static token — the credential a browser actually carries.
	assert.Equal(t, http.StatusNotFound,
		getWithCookie(t, ro, "/api/artifacts/bob-artifact", aliceCookie).Code,
		"another owner's artifact is not found, not merely forbidden")
	assert.Equal(t, http.StatusOK,
		getWithCookie(t, ro, "/api/artifacts/alice-artifact", aliceCookie).Code)
}

func loginAs(t *testing.T, ro *Router, name, password string) *http.Cookie {
	t.Helper()
	w := submitLogin(t, ro, name, password, "")
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	cookie := cookiesFrom(w)[sessionCookieName]
	require.NotNil(t, cookie)
	return cookie
}

func getWithCookie(t *testing.T, ro *Router, path string, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

// --- one table, one owner_id space -------------------------------------

// The nullable column is the whole design: an OIDC identity has no password
// and stays a first-class row beside accounts that do, so the two kinds of
// user are one directory rather than two that would later need linking.
func TestOIDCIdentityAndLocalAccountCoexist(t *testing.T) {
	idp := &stubProvider{
		loginURL:   "https://idp.test/authorize",
		identities: []auth.Identity{{ExternalID: "sub-1", Email: "sso@example.test"}},
	}
	ro, st := newLoginTestRouter(t, idp, nil, func(c *Config) { c.LocalUsers = true })
	local := addAccount(t, st, "local@example.test", "local-long-passphrase")

	ssoCookie := runSSOLogin(t, ro)
	ssoSession, err := st.GetSession(context.Background(), ssoCookie.Value)
	require.NoError(t, err)
	sso, err := st.GetUser(context.Background(), ssoSession.UserID)
	require.NoError(t, err)

	// One table, one id space, no overlap.
	assert.NotEqual(t, local.ID, sso.ID)
	users, err := st.ListUsers(context.Background())
	require.NoError(t, err)
	require.Len(t, users, 2)

	// They differ by which columns are populated, and by nothing else.
	assert.True(t, local.HasPassword)
	assert.False(t, sso.HasPassword, "an identity a provider issued has no password of ours")
	assert.Equal(t, "sub-1", sso.ExternalID, "the provider's subject, untouched")
	assert.Equal(t, auth.LocalExternalID("local@example.test"), local.ExternalID)

	// The provider's identity is not reachable through the password form,
	// even by someone who knows its email — it has no hash to match.
	w := submitLogin(t, ro, "sso@example.test", "local-long-passphrase", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Both kinds of session are the same session.
	localCookie := loginAs(t, ro, "local@example.test", "local-long-passphrase")
	assert.Equal(t, ssoCookie.Name, localCookie.Name)
	assert.Equal(t, len(ssoCookie.Value), len(localCookie.Value))
}

// --- the first-admin rule ----------------------------------------------

// "The first user on an instance becomes admin" is applied where users rows
// are made, so it holds however the first one arrives: a provider callback, a
// password form, or the CLI.
func TestFirstUserOnAnInstanceIsTheAdmin(t *testing.T) {
	t.Run("provisioned by the operator", func(t *testing.T) {
		_, st := newAccountRouter(t, nil)
		first := addAccount(t, st, "first@example.test", "first-long-passphrase")
		second := addAccount(t, st, "second@example.test", "second-long-passphrase")
		assert.True(t, first.IsAdmin)
		assert.False(t, second.IsAdmin)
	})

	t.Run("arriving from an identity provider", func(t *testing.T) {
		idp := &stubProvider{loginURL: "https://idp.test/authorize", identities: []auth.Identity{
			{ExternalID: "sub-1", Email: "one@example.test"},
			{ExternalID: "sub-2", Email: "two@example.test"},
		}}
		ro, st := newIdentityTestRouter(t, idp)
		runLogin(t, ro)
		runLogin(t, ro)

		users, err := st.ListUsers(context.Background())
		require.NoError(t, err)
		require.Len(t, users, 2)
		assert.True(t, users[0].IsAdmin)
		assert.False(t, users[1].IsAdmin)
	})

	// Mixed: whoever is first wins, whichever door they came through. This is
	// the same rule deployment.md §3.4 already stated — the first login adopts
	// owner 1's existing library, which is why the operator is told to do it
	// themselves.
	t.Run("whichever door is used first", func(t *testing.T) {
		idp := &stubProvider{loginURL: "https://idp.test/authorize",
			identities: []auth.Identity{{ExternalID: "sub-1", Email: "sso@example.test"}}}
		ro, st := newLoginTestRouter(t, idp, nil, func(c *Config) { c.LocalUsers = true })

		runSSOLogin(t, ro)
		later := addAccount(t, st, "later@example.test", "later-long-passphrase")

		first, err := st.GetUser(context.Background(), defaultOwnerID)
		require.NoError(t, err)
		assert.True(t, first.IsAdmin, "the SSO identity got there first")
		assert.False(t, later.IsAdmin)
	})
}

// --- the break-glass credential ----------------------------------------

// The decision recorded in cmd/server: the environment credential stays live
// once the users table has rows, and is checked before the table so nothing
// stored can shadow it. That is the whole of its value — it is the way back in
// after locking yourself out, which by definition happens on an instance that
// already has users.
func TestEnvironmentCredentialOutranksTheStoredPassword(t *testing.T) {
	const name = "curator@example.test"
	cred := newTestCredential(t, name, "break-glass-passphrase")
	ro, st := newAccountRouter(t, cred)
	user := addAccount(t, st, name, "the-forgotten-passphrase")
	require.True(t, user.IsAdmin)

	// The operator recovers into their own account, not a rescue account
	// holding none of their artifacts.
	cookie := loginAs(t, ro, name, "break-glass-passphrase")
	sess, err := st.GetSession(context.Background(), cookie.Value)
	require.NoError(t, err)
	assert.Equal(t, user.ID, sess.UserID)

	// The stored password still works; the environment one is additional,
	// not a replacement.
	assert.Equal(t, http.StatusFound,
		submitLogin(t, ro, name, "the-forgotten-passphrase", "").Code)

	// It grants nothing beyond the account it names.
	other := addAccount(t, st, "other@example.test", "other-long-passphrase")
	assert.Equal(t, http.StatusUnauthorized,
		submitLogin(t, ro, other.Email, "break-glass-passphrase", "").Code)

	// Removing the variables leaves the provisioned accounts working — which
	// is what makes turning the bypass off a safe thing to do.
	ro.cfg.LocalCredential = nil
	assert.Equal(t, http.StatusFound,
		submitLogin(t, ro, name, "the-forgotten-passphrase", "").Code)
	assert.Equal(t, http.StatusUnauthorized,
		submitLogin(t, ro, name, "break-glass-passphrase", "").Code)
}

// On an empty instance the same credential is the bootstrap: it creates the
// account it names, and the first-user rule makes that account the admin.
func TestEnvironmentCredentialBootstrapsTheFirstAdmin(t *testing.T) {
	ro, st := newLocalLoginRouter(t)
	require.Empty(t, mustListUsers(t, st), "nothing is provisioned ahead of the first login")

	runLocalLogin(t, ro)

	users := mustListUsers(t, st)
	require.Len(t, users, 1)
	assert.Equal(t, defaultOwnerID, users[0].ID,
		"and it adopts the owner id a single-user library is already filed under")
	assert.True(t, users[0].IsAdmin)
	assert.False(t, users[0].HasPassword,
		"the password is in the environment, so the row carries none — "+
			"unsetting the variables leaves an account with no way in, which is why "+
			"`user passwd` exists")
}

func mustListUsers(t *testing.T, st store.Store) []*store.User {
	t.Helper()
	users, err := st.ListUsers(context.Background())
	require.NoError(t, err)
	return users
}

// --- the instance that configured none of this -------------------------

// Most existing deployments. No provider, no environment credential, no
// accounts: no /auth routes, no gate, owner 1, exactly as before.
func TestInstanceWithNoCredentialIsUnchanged(t *testing.T) {
	ro, st := newLoginTestRouter(t, nil, nil)

	assert.False(t, ro.loginEnabled())
	assert.False(t, ro.localLoginEnabled())

	for _, path := range []string{"/auth/login", "/auth/local", "/auth/sso", "/auth/callback"} {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}

	// Pages are served without a login, and the static token still reaches
	// the API.
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	assert.Empty(t, mustListUsers(t, st), "no login means no identities to record")
}

// The gate arms on provisioned accounts alone, so an operator who uses the CLI
// and never sets the environment pair is not left serving their library to
// anyone who can reach the origin.
func TestProvisionedAccountsAloneArmTheGate(t *testing.T) {
	ro, st := newAccountRouter(t, nil)
	addAccount(t, st, "alice@example.test", "alice-long-passphrase")

	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/artifacts/xyz", nil))
	require.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "/auth/login")

	w = httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `action="/auth/local"`)
}

// --- nothing leaks -----------------------------------------------------

// A password or a hash in a log line outlives the request by however long logs
// are kept, and reaches whoever can read them. Asserted over a real login
// rather than by inspecting call sites, so a future handler that adds one is
// caught.
func TestLoginLogsNeitherPasswordNorHash(t *testing.T) {
	const password = "an-unmistakable-passphrase"
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	ro, st := newAccountRouter(t, newTestCredential(t, "curator@example.test", password))
	user := addAccount(t, st, "alice@example.test", password)
	_, hash, err := st.LookupLocalCredential(context.Background(), user.ExternalID)
	require.NoError(t, err)

	// A success, a wrong password, and an unknown name — the three shapes a
	// login attempt takes.
	require.Equal(t, http.StatusFound, submitLogin(t, ro, "alice@example.test", password, "").Code)
	require.Equal(t, http.StatusFound, submitLogin(t, ro, "curator@example.test", password, "").Code)
	require.Equal(t, http.StatusUnauthorized, submitLogin(t, ro, "alice@example.test", "wrong-one", "").Code)
	require.Equal(t, http.StatusUnauthorized, submitLogin(t, ro, "nobody@example.test", password, "").Code)

	out := logged.String()
	require.NotEmpty(t, out, "the login path does log — otherwise this asserts nothing")
	assert.NotContains(t, out, password)
	assert.NotContains(t, out, "wrong-one")
	assert.NotContains(t, out, hash)
	assert.NotContains(t, out, "$2a$", "no bcrypt hash in any form")
	assert.Contains(t, out, "login", "a successful login is still recorded")
}

// The hash reaches exactly one caller, by name. Everything else that reads a
// user gets HasPassword, so a hash cannot arrive somewhere it was not asked
// for — a page view model, a log attribute, a future JSON response.
func TestUserRowsDoNotCarryTheHash(t *testing.T) {
	_, st := newAccountRouter(t, nil)
	user := addAccount(t, st, "alice@example.test", "alice-long-passphrase")

	fetched, err := st.GetUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.True(t, fetched.HasPassword)

	// The struct has no field for it, which is the actual guarantee; this
	// spells out the consequence for anything that serializes a user.
	rendered := strings.Join([]string{
		fetched.Email, fetched.ExternalID,
	}, " ")
	assert.NotContains(t, rendered, "$2a$")
}
