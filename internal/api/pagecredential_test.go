package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-5imk. Every server-rendered page used to embed `AUTH_TOKEN` — the
// operator's full-authority service credential — in its bootstrap <script>,
// whoever was looking at it. On an instance with an identity provider that
// undid the one property av-30rj chose opaque server-side sessions to get:
// logout deletes the session row, but it cannot delete a token out of page
// source the visitor has already loaded. Access survived logout, over every
// artifact, every collection, the share table and the BYO provider key, and
// could be withdrawn only by rotating the secret for everyone.
//
// The fix is that the credential is derived from the request
// (pagecredential.go). This file is what keeps it derived, and it is written
// as a route walk rather than a handful of page assertions for the reason
// csrf_test.go (av-ke2m) and renderheaders_test.go (av-nr0p) are: the failure
// mode is a *new* page that nobody thought to check, and it is silent — the
// page works perfectly while it leaks.

// pageRoute is one GET route the app origin answers, plus a concrete path to
// request it at. The path is what makes the walk an actual test rather than an
// inventory: every route is fetched and its response body inspected.
type pageRoute struct {
	route string
	// path is the concrete URL to request. "{id}" is replaced with a real
	// artifact's id so the artifact pages render fully — a 404 page would
	// contain no token no matter how badly the credential logic were broken.
	path string
}

// appOriginGETRoutePaths is every GET route the app mux registers. A route
// missing from here fails the walk below on purpose: a page route that ships
// without a row is a page nobody checked for the leak this file exists to
// prevent.
var appOriginGETRoutePaths = []pageRoute{
	// The server-rendered pages — the ones that carry a bootstrap <script>,
	// and therefore the whole point of this file.
	{route: "/", path: "/"},
	{route: "/new", path: "/new"},
	{route: "/agent", path: "/agent?artifact={id}"},
	{route: "/artifacts/{artifactID}", path: "/artifacts/{id}"},
	{route: "/artifacts/{artifactID}/edit", path: "/artifacts/{id}/edit"},
	{route: "/artifacts/{artifactID}/open", path: "/artifacts/{id}/open"},

	// htmx fragments: page markup by another name, so they are held to the
	// same rule as the pages they are swapped into.
	{route: "/partials/agent-preview", path: "/partials/agent-preview?artifact={id}"},
	{route: "/partials/card-widget", path: "/partials/card-widget?artifact={id}"},

	// Static and public surfaces. They embed nothing per-request, which is
	// exactly the claim being checked.
	{route: "/assets/*", path: "/assets/gallery/api.js"},
	{route: "/manifest.json", path: "/manifest.json"},
	{route: "/s/{shareID}", path: "/s/some-share"},
	{route: "/api/settings/public", path: "/api/settings/public"},

	// The authenticated API. A JSON response has no business echoing the
	// service token either, and including the group means the walk covers the
	// whole mux rather than a subset someone has to keep re-drawing.
	{route: "/api/artifacts/", path: "/api/artifacts"},
	{route: "/api/artifacts/{artifactID}/", path: "/api/artifacts/{id}"},
	{route: "/api/artifacts/{artifactID}/state", path: "/api/artifacts/{id}/state"},
	{route: "/api/artifacts/{artifactID}/widget", path: "/api/artifacts/{id}/widget"},
	{route: "/api/artifacts/{artifactID}/transcripts", path: "/api/artifacts/{id}/transcripts"},
	{route: "/api/agent/key", path: "/api/agent/key"},
	// SSE. With no agent manager configured it answers "not enabled" and
	// returns, so walking it does not park the test on an open stream.
	{route: "/api/agent/sessions/{sessionID}/events", path: "/api/agent/sessions/none/events"},
	{route: "/api/collections/", path: "/api/collections"},
	{route: "/api/tags/", path: "/api/tags"},

	// The login flow, registered only when a login is configured. Requested
	// last, because logging out is one of them.
	//
	// /auth/login is the one page here that renders markup rather than
	// redirecting (av-q30x gave it a form), so it is exactly the kind of route
	// this walk exists for. /auth/sso only redirects to the provider.
	{route: "/auth/login", path: "/auth/login"},
	{route: "/auth/sso", path: "/auth/sso"},
	{route: "/auth/callback", path: "/auth/callback"},
	{route: "/auth/logout", path: "/auth/logout"},
}

const undeclaredPageRouteMsg = `GET %s is registered on the app origin but has no row in appOriginGETRoutePaths.

That list is a credential invariant (av-5imk), not bookkeeping. A server-rendered
page authenticates its own API calls with a credential its bootstrap <script>
carries, and the wrong answer there is the operator's AUTH_TOKEN — a full-authority
service credential that logout cannot revoke and that only a secret rotation can
withdraw. The failure is silent: the page works perfectly while it leaks.

Add {route: "%[1]s", path: "<a concrete URL to request it at>"} to the list. The
walk will fetch it and assert the response body does not contain the token. If it
does, derive the page's credential from the request via Router.pageCredentials
instead of reading ro.cfg.AuthToken.

See docs/security.md §1.5.`

// pageCredentialToken is deliberately not a word that appears in page markup,
// so a body match means the token and nothing else.
const pageCredentialToken = "operator-service-token-9f3c1a"

// newPageCredentialRouter builds an instance with the distinctive token above,
// optionally behind an identity provider, and returns it with one stored
// artifact for the artifact-scoped routes to render.
func newPageCredentialRouter(t *testing.T, idp auth.IdentityProvider) (*Router, string) {
	t.Helper()

	f, err := os.CreateTemp("", "test-pagecred-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	blobDir, err := os.MkdirTemp("", "test-pagecred-blobs-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(blobDir) })

	bl, err := blob.NewFSStore(blobDir)
	require.NoError(t, err)

	box, err := secrets.Load("test-secret", "")
	require.NoError(t, err)

	ro := NewRouter(Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    "https://app.test",
		RenderOrigin: "https://render.test",
		AuthToken:    pageCredentialToken,
		Secrets:      box,
		Identity:     idp,
	})

	// Ingest through the API with the static token, so the artifact exists
	// however the pages are later authenticated.
	body, _ := json.Marshal(map[string]any{
		"title":             "Walked Artifact",
		"body":              "<html><body><h1>hi</h1></body></html>",
		"network_allowlist": []string{},
	})
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+pageCredentialToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		Artifact struct{ ID string } `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Artifact.ID)
	return ro, created.Artifact.ID
}

// walkAppOriginGETRoutes checks the declaration against the mux in both
// directions — no route without a row, no row without a route — and returns the
// declared rows so the callers below can request them.
func walkAppOriginGETRoutes(t *testing.T, routers ...*Router) []pageRoute {
	t.Helper()

	declared := map[string]bool{}
	for _, row := range appOriginGETRoutePaths {
		require.False(t, declared[row.route], "duplicate row for %s", row.route)
		require.NotEmpty(t, row.path, "%s: a row needs a concrete path to request", row.route)
		declared[row.route] = true
	}

	seen := map[string]bool{}
	for _, ro := range routers {
		require.NoError(t, chi.Walk(ro.Mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if method != http.MethodGet || seen[route] {
				return nil
			}
			seen[route] = true
			assert.True(t, declared[route], undeclaredPageRouteMsg, route)
			return nil
		}))
	}
	for route := range declared {
		assert.True(t, seen[route],
			"appOriginGETRoutePaths lists %s but the app mux no longer registers it — drop the stale row", route)
	}
	return appOriginGETRoutePaths
}

// TestNoPageHandsASessionVisitorTheServiceToken is the ticket, as a test.
//
// It logs in, then requests every GET route the app origin answers with that
// session's cookie, and asserts none of the responses contains the static
// token. A page that embedded it would be handing a logged-in user a
// credential their own logout cannot take back.
func TestNoPageHandsASessionVisitorTheServiceToken(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	ro, artifactID := newPageCredentialRouter(t, idp)

	rendered := 0
	for _, row := range walkAppOriginGETRoutes(t, ro) {
		path := strings.ReplaceAll(row.path, "{id}", artifactID)
		// A fresh session per request: /auth/logout is one of the routes
		// being walked, and it revokes the cookie it is given.
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(runLogin(t, ro))
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)

		body := w.Body.String()
		assert.NotContains(t, body, pageCredentialToken,
			"GET %s handed a session-authenticated visitor the service token.\n"+
				"Its bootstrap must carry Router.pageCredentials(r), which emits nothing here: "+
				"the session cookie already authenticates the page's own fetches, and a bearer "+
				"token embedded beside it survives the logout that deletes the session (av-5imk).",
			path)
		if strings.Contains(body, "const TOKEN = ") {
			rendered++
			assert.Contains(t, body, `const TOKEN = "";`,
				"GET %s renders a bootstrap, so it must state the empty credential explicitly — "+
					"its page JS branches on TOKEN being falsy", path)
		}
	}
	// The walk must not pass by rendering no bootstraps at all.
	assert.GreaterOrEqual(t, rendered, 5,
		"expected every server-rendered page to emit a TOKEN bootstrap; only %d did", rendered)
}

// The single-user instance is the one this feature must not touch. It has no
// identity provider, therefore no session and no second credential — the static
// token is the only thing its page JS can authenticate with, and its page
// visitor is by construction the operator who already holds it.
func TestSingleUserPageStillCarriesTheStaticToken(t *testing.T) {
	ro, artifactID := newPageCredentialRouter(t, nil)

	for _, path := range []string{"/", "/new", "/agent", "/artifacts/" + artifactID, "/artifacts/" + artifactID + "/edit"} {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		require.Equal(t, http.StatusOK, w.Code, path)
		assert.Contains(t, w.Body.String(), `const TOKEN = "`+pageCredentialToken+`";`,
			"GET %s on an instance with no identity provider must behave exactly as it always "+
				"has: the static token is its only credential (av-5imk)", path)
		assert.Contains(t, w.Body.String(), "const READ_ONLY = false;", path)
	}
}

// The three cases, at the one function that decides them. The walk above
// covers the two an HTTP request can currently produce; nothing marks a *page*
// request as a public visitor yet (av-wmp6 marks API requests only), so the
// anonymous case is asserted here, where it can be constructed.
func TestPageCredentialsPerVisitor(t *testing.T) {
	single, _ := newPageCredentialRouter(t, nil)
	withIdentity, _ := newPageCredentialRouter(t, &stubProvider{})

	req := func(ctx context.Context) *http.Request {
		return httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	}
	anonymous := context.WithValue(context.Background(), publicVisitorKey, true)
	session := withSessionAuthed(context.Background())

	assert.Equal(t, pageCredentials{Token: pageCredentialToken},
		single.pageCredentials(req(context.Background())),
		"single-user: the static token is the only credential the instance has")

	assert.Equal(t, pageCredentials{},
		withIdentity.pageCredentials(req(session)),
		"session-authenticated: the cookie is the credential, so the page is handed none")

	assert.Equal(t, pageCredentials{ReadOnly: true},
		single.pageCredentials(req(anonymous)),
		"anonymous: no credential, and the page's JS refuses writes locally rather than erroring")
	assert.Equal(t, pageCredentials{ReadOnly: true},
		withIdentity.pageCredentials(req(anonymous)),
		"anonymous, identity configured: same answer — nothing about the instance grants a stranger authority")

	// The dangerous middle case: a provider is configured but this request
	// resolved no session. That is a public visitor or a hole in sessionGate,
	// and the static token is not the right answer to either.
	assert.Equal(t, pageCredentials{},
		withIdentity.pageCredentials(req(context.Background())),
		"identity configured, no session resolved: withhold rather than fall back to the service token")
}

// The SSE stream is the one API call a page cannot credential with a header —
// EventSource sets none — so it is also the one that a page carrying no token
// would silently lose. It authenticates on the session cookie instead.
func TestEventStreamAcceptsTheSessionCookie(t *testing.T) {
	idp := &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1", Email: "a@example.test"}}}
	ro, _ := newPageCredentialRouter(t, idp)

	// No credential at all is still refused.
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/api/agent/sessions/none/events", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// The session cookie authenticates it, so the handler gets as far as
	// looking for the session (absent here: no agent manager is configured).
	req := httptest.NewRequest("GET", "/api/agent/sessions/none/events", nil)
	req.AddCookie(runLogin(t, ro))
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code,
		"an EventSource opened by a session-authenticated page carries only the cookie (av-5imk); "+
			"refusing it would break the agent surface on every instance with an identity provider")

	// And the static token still works, which is how a single-user instance's
	// page opens the same stream (av-rgp1 owns narrowing that).
	req = httptest.NewRequest("GET", "/api/agent/sessions/none/events?token="+pageCredentialToken, nil)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
}

// The page scripts must spend the credential through one function, because a
// call site that builds its own Authorization header is exactly the defect
// pageCredentials cannot prevent from the server side.
func TestPageScriptsUseTheSharedAPIClient(t *testing.T) {
	ro := newTestRouter(t)

	api := galleryAsset(t, ro, "/assets/gallery/api.js")
	assert.Contains(t, api, "window.apiFetch")
	assert.Contains(t, api, "window.apiEventSource")

	for _, name := range []string{"index.js", "new.js", "detail.js", "edit.js", "agent.js", "state.js"} {
		js := galleryAsset(t, ro, "/assets/gallery/"+name)
		assert.NotContains(t, js, "'Authorization'",
			"%s builds its own Authorization header; call apiFetch instead, which knows which "+
				"of the three credential cases this page render landed in (av-5imk)", name)
		assert.NotContains(t, js, "new EventSource(",
			"%s opens an SSE stream directly; call apiEventSource instead, which appends the "+
				"token only when the page was given one (av-5imk)", name)
	}
}
