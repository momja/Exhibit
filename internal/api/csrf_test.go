package api

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-ke2m. Cookie auth (av-30rj) made the session an *ambient* credential: the
// browser attaches it to every request the app origin receives, including ones
// another site caused. Exhibit's whole answer to cross-site request forgery is
// one cookie attribute — `SameSite=Lax` — and no CSRF token underneath it.
//
// That answer is sufficient, but only while two conditions both hold, so this
// file pins one each:
//
//  1. The cookie really is `SameSite=Lax`, which is what withholds it from a
//     cross-site unsafe method.
//  2. No GET route mutates, because Lax *does* send the cookie on a cross-site
//     top-level GET.
//
// docs/security.md §1.4 states the posture; these two tests are what keep it
// true when someone changes a cookie attribute or adds a route.

// assertCSRFDefence asserts the attribute that stands between an ambient
// session credential and forged mutations. It is named for what it defends
// rather than for the field it reads, because a bare `SameSite` equality check
// reads as bookkeeping and invites being "fixed" by whoever needs the cookie to
// survive a cross-site request.
func assertCSRFDefence(t *testing.T, c *http.Cookie, name string) {
	t.Helper()
	require.NotNil(t, c, "%s was never set", name)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite,
		"%s must stay SameSite=Lax — it is Exhibit's entire CSRF defence (av-ke2m).\n"+
			"Lax is what withholds the cookie from a cross-site POST/PUT/PATCH/DELETE, so a "+
			"forged mutation arrives with no credential and 401s. Loosening it to None, to make "+
			"an embed or a cross-origin browser client work, hands every mutating route to any "+
			"page the user visits; there is no CSRF token underneath to catch it. Setting the "+
			"attribute explicitly is also load-bearing: Chrome's 'Lax+POST' two-minute grace "+
			"applies only to cookies that carry no SameSite attribute at all.\n"+
			"See docs/security.md §1.4.", name)
}

// The session cookie is the credential a forged request would ride, so the
// attribute that governs it gets a test of its own next to the invariant it
// depends on. (The login-flow cookies are checked at the same helper, in
// auth_test.go, where the flow they belong to is exercised.)
func TestSessionCookieIsSameSiteLaxTheCSRFControl(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	ro, _ := newIdentityTestRouter(t, idp)
	assertCSRFDefence(t, runLogin(t, ro), sessionCookieName)
}

// getRoute declares what one registered GET route does. `mutates` is false for
// every route that only reads; the handful that is true must say why a forged
// cross-site GET of it is harmless.
type getRoute struct {
	route   string
	mutates bool
	why     string
}

// appOriginGETRoutes is every GET route the **app origin's** mux answers, and
// the test below requires it to match exactly. (The render origin is a separate
// mux with three GET routes of its own; it holds no session and sets no cookie,
// and its coverage lives in renderheaders_test.go — av-nr0p.)
//
// This is a CSRF invariant written as a list, not an inventory. Adding a route
// to the mux without adding a row here fails the suite on purpose: the point is
// that someone states, once, whether the new handler reads or writes.
var appOriginGETRoutes = []getRoute{
	// Server-rendered pages and htmx fragments. All reads; the pages that
	// change things do it from page JS through the API's unsafe methods.
	{route: "/"},
	{route: "/new"},
	{route: "/agent"},
	{route: "/artifacts/{artifactID}"},
	{route: "/artifacts/{artifactID}/edit"},
	// Mints a render token and redirects (av-c5aq). The token is an in-memory
	// HMAC — nothing is stored, and the redirect target is a read too.
	{route: "/artifacts/{artifactID}/open"},
	// The account list (av-utap). A read: every change it offers is a POST or
	// PATCH to /api/admin/users, which is exactly the split this list exists
	// to keep — a "disable user" convenience GET would be forgeable by any
	// page an admin visits.
	{route: "/admin/users"},
	{route: "/partials/agent-preview"},
	{route: "/partials/card-widget"},
	// Static assets, the manifest, the share redirect, and the instance's
	// public identity (av-4ac9) — public reads, credential or not.
	{route: "/assets/*"},
	{route: "/manifest.json"},
	{route: "/s/{shareID}"},
	{route: "/api/settings/public"},

	// The authenticated API group — the routes the session cookie actually
	// unlocks, and therefore the ones a forged GET would be aimed at.
	{route: "/api/artifacts/"},
	{route: "/api/artifacts/{artifactID}/"},
	{route: "/api/artifacts/{artifactID}/state"},
	{route: "/api/artifacts/{artifactID}/widget"},
	{route: "/api/artifacts/{artifactID}/transcripts"},
	{route: "/api/agent/key"},
	// SSE. It subscribes to a live session's event stream; it starts no turn
	// and persists nothing.
	{route: "/api/agent/sessions/{sessionID}/events"},
	{route: "/api/collections/"},
	{route: "/api/tags/"},
	// The instance's account directory (av-utap), behind adminOnly. Listing
	// accounts changes nothing; creating, disabling and resetting are the
	// POST and PATCH on the same group.
	{route: "/api/admin/users/"},

	// The login flow: the only GET routes that change state, registered only
	// when a login is configured. Each is safe for its own reason, not by the
	// rule above.
	//
	// /auth/login mutates only on an instance with no local credential, where
	// it redirects into the provider flow; with one it renders the login page
	// and mints nothing (av-q30x). Declared by its worst case.
	{route: "/auth/login", mutates: true,
		why: "mints short-lived state/verifier cookies before any session exists; " +
			"forging it starts a login the attacker cannot finish"},
	{route: "/auth/sso", mutates: true,
		why: "is /auth/login's provider redirect split out for the login page's SSO button, " +
			"and mints the same pre-session state/verifier cookies — a forged one starts a " +
			"login the attacker cannot finish"},
	{route: "/auth/callback", mutates: true,
		why: "is a cross-site top-level GET by construction — the provider redirects the " +
			"browser to it — and carries its own forgery defence in the state cookie it must match"},
	{route: "/auth/logout", mutates: true,
		why: "revokes a session, so a forged request achieves nothing worse than logging the " +
			"user out; it stays a link because that is the affordance people expect"},
}

const csrfUndeclaredGETMsg = `GET %s is registered on the app origin but has no row in appOriginGETRoutes.

That list is a CSRF invariant (av-ke2m), not bookkeeping. The session cookie is
SameSite=Lax, which withholds it from a cross-site POST/PUT/PATCH/DELETE but still
sends it on a cross-site top-level GET. So a GET that changes state is forgeable by
any page the user visits — an <img src> tag is enough — and there is no CSRF token
underneath to catch it.

  - If the handler only reads, add {route: "%[1]s"} to the list and you are done.
  - If it changes state, do not ship it as a GET. Move the mutation to POST/PUT/
    PATCH/DELETE, where Lax withholds the credential.

The mutating rows in that list are the login flow, and each one says why it is safe.
See docs/security.md §1.4.`

// TestNoAppOriginGETRouteMutates walks the app mux and holds every GET route it
// answers against the list above. Condition 2 of the CSRF posture — "Lax is
// enough because no GET mutates" — is otherwise a claim nothing checks, and the
// change that would break it (a convenience GET wrapping an existing POST) looks
// entirely reasonable at review time.
//
// Note the property is "every GET is a *read*", not "every GET is
// *authenticated*". Public mode (av-4ac9) serves some of these reads with no
// credential at all, and an unauthenticated GET has nothing to forge; opening a
// route up must not fail this test, while making one mutate must.
func TestNoAppOriginGETRouteMutates(t *testing.T) {
	declared := map[string]getRoute{}
	for _, row := range appOriginGETRoutes {
		_, dup := declared[row.route]
		require.False(t, dup, "duplicate row for %s", row.route)
		require.Equal(t, row.mutates, row.why != "",
			"%s: a mutating GET must carry the reason a forged one is harmless, and a read must not", row.route)
		declared[row.route] = row
	}

	// Every login configuration, because the /auth routes are registered
	// per-path — the provider ones only with an issuer, /auth/local only with
	// a credential (av-q30x) — and a route walked in none of them would slip
	// past this test entirely.
	plain := newTestRouter(t)
	withIdentity, _ := newIdentityTestRouter(t, &stubProvider{})
	withLocal, _ := newLocalLoginRouter(t)

	seen := map[string]bool{}
	for _, ro := range []*Router{plain, withIdentity, withLocal} {
		require.NoError(t, chi.Walk(ro.Mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			// A route registered in both configurations is walked twice; one
			// verdict per route is enough.
			if method != http.MethodGet || seen[route] {
				return nil
			}
			seen[route] = true
			_, ok := declared[route]
			assert.True(t, ok, csrfUndeclaredGETMsg, route)
			return nil
		}))
	}

	// And the reverse, so the walk can never pass by finding nothing: every row
	// must correspond to a route that is still registered.
	for route := range declared {
		assert.True(t, seen[route],
			"appOriginGETRoutes lists %s but the app mux no longer registers it — drop the stale row", route)
	}
}
