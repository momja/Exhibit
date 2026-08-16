package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-syug. av-swzv made owner_id a real predicate on every API query; the page
// routes never got it. They are registered outside the API's auth group,
// sessionGate resolved their user and propagated only a boolean, and
// ownerIDFromCtx quietly answered "owner 1" — so a user whose owner_id was 2
// logged in, loaded `/`, and was served owner 1's library. renderURLs minted
// that page's frame tokens from the same value, so the artifacts rendered too:
// bodies and inlined state, inside the wrong user's gallery.
//
// This file is what keeps the owner attached, and it is a route walk rather
// than a handful of page assertions for the reason csrf_test.go (av-ke2m),
// renderheaders_test.go (av-nr0p) and pagecredential_test.go (av-5imk) are: the
// failure mode is a *new* page nobody thought to check, and it is silent. The
// page renders perfectly; it just renders somebody else's shelf.

// Distinctive markers, so a body match means the leak and not a coincidence.
const (
	ownerOneTitle = "OwnerOneShelfTitle9f3c"
	ownerOneBody  = "OwnerOneBodyMarker9f3c"
	ownerOneState = "OwnerOneStateMarker9f3c"
	ownerTwoTitle = "OwnerTwoShelfTitle9f3c"
	ownerTwoBody  = "OwnerTwoBodyMarker9f3c"
	ownerTwoState = "OwnerTwoStateMarker9f3c"
)

// pageOwnerRoute is one GET route the app origin answers, and the claim being
// made about it: either it reads the library — in which case it must read it as
// the requester, and the walk proves that with two real owners — or it does
// not, in which case the row has to say why.
type pageOwnerRoute struct {
	route string
	// ownerScoped routes are exercised twice, with "{id}" replaced by the
	// session owner's artifact and then by the other owner's. The first
	// request is the non-vacuity control: a route that answers nothing at all
	// would pass the leak assertions trivially.
	ownerScoped bool
	ownPath     string
	foreignPath string
	// scopedByHandler marks a route that IS owner-scoped but not by this
	// walk's own mechanism (sessionGate + ownerMiddleware resolving the owner
	// for a handler that trusts ownerIDFromCtx). Its handler resolves the
	// owner itself instead — the SSE route is the one case, because
	// EventSource sets no headers and so cannot sit behind either middleware.
	// This is a distinct claim from "not owner-scoped at all" (why alone):
	// the row still promises per-owner isolation, just proven by a different
	// test, which coveredBy must name.
	scopedByHandler bool
	// coveredBy is the test file that pins ownership for a scopedByHandler
	// row, since this walk cannot exercise it directly. Required exactly when
	// scopedByHandler is true, so a handler-scoped exemption is a claim
	// backed by a named test rather than an assertion nobody checks.
	coveredBy string
	// why is required of every row that claims exemption, so "not owner-scoped"
	// is an argument someone made rather than a box left unticked.
	why string
}

// appOriginGETOwnerScope is every GET route the app mux registers. A route
// missing from here fails the walk on purpose.
var appOriginGETOwnerScope = []pageOwnerRoute{
	// The server-rendered pages and their htmx fragments: everything that
	// reads the library on the visitor's behalf without going through the
	// API's auth group. These are the routes this ticket is about.
	{route: "/", ownerScoped: true, ownPath: "/", foreignPath: "/"},
	{route: "/artifacts/{artifactID}", ownerScoped: true,
		ownPath: "/artifacts/{id}", foreignPath: "/artifacts/{id}"},
	{route: "/artifacts/{artifactID}/edit", ownerScoped: true,
		ownPath: "/artifacts/{id}/edit", foreignPath: "/artifacts/{id}/edit"},
	{route: "/artifacts/{artifactID}/open", ownerScoped: true,
		ownPath: "/artifacts/{id}/open", foreignPath: "/artifacts/{id}/open"},
	{route: "/agent", ownerScoped: true,
		ownPath: "/agent?artifact={id}", foreignPath: "/agent?artifact={id}"},
	{route: "/partials/agent-preview", ownerScoped: true,
		ownPath: "/partials/agent-preview?artifact={id}", foreignPath: "/partials/agent-preview?artifact={id}"},
	{route: "/partials/card-widget", ownerScoped: true,
		ownPath: "/partials/card-widget?artifact={id}", foreignPath: "/partials/card-widget?artifact={id}"},

	// A page that reads nothing. Ingest is entirely a client-side
	// conversation with POST /api/artifacts, which authenticates itself, so
	// /new has no library data to scope. It still sits in the page group —
	// membership is cheap and keeps "page route" and "has an owner" the same
	// set — but there is nothing here to assert.
	{route: "/new", why: "renders no library data; ingest is a client-side conversation with the API"},

	// Authority-scoped rather than owner-scoped, and the distinction is what
	// av-utap is about. These read the instance's *account directory* — the
	// same list whoever is looking — so there is no "somebody else's library"
	// for them to leak, and an owner is the wrong question to ask of them.
	// The question they do have to answer is the opposite one, that not
	// everyone may see them at all; that is adminOnly's (internal/api/admin.go)
	// and is pinned by admin_test.go.
	{route: "/admin/users", why: "reads the instance's account directory, not a library — guarded by adminOnly (av-utap) rather than scoped by owner"},
	{route: "/api/admin/users/", why: "same directory as JSON; API group plus adminOnly, covered by admin_test.go"},

	// Static and instance-level surfaces: nothing per-visitor to scope.
	{route: "/assets/*", why: "embedded static assets, identical for every visitor"},
	{route: "/manifest.json", why: "static app manifest, identical for every visitor"},
	{route: "/api/settings/public", why: "the instance's own name and description, deliberately readable by anyone"},

	// Owner-independent by design rather than by omission.
	{route: "/s/{shareID}", why: "the share row is the authorization (architecture.md §7); it redirects to the render origin and reads no library"},
	{route: "/api/agent/sessions/{sessionID}/events", scopedByHandler: true, coveredBy: "agent_session_owner_test.go",
		why: "streams one agent session's events by id; it is owner-scoped, but by authorizeEventStream resolving the owner itself (EventSource sets no headers, so it cannot sit in a group that runs the middlewares)"},

	// The authenticated API group, which runs authMiddleware +
	// ownerMiddleware. Its owner scoping is av-ep8k's, pinned by
	// owner_scope_test.go; the rows are here so this walk covers the whole
	// mux rather than a subset someone has to keep redrawing.
	{route: "/api/artifacts/", why: "API group: authMiddleware + ownerMiddleware, covered by owner_scope_test.go"},
	{route: "/api/artifacts/{artifactID}/", why: "API group, covered by owner_scope_test.go"},
	{route: "/api/artifacts/{artifactID}/state", why: "API group, covered by owner_scope_test.go"},
	{route: "/api/artifacts/{artifactID}/widget", why: "API group, covered by owner_scope_test.go"},
	{route: "/api/artifacts/{artifactID}/transcripts", why: "API group, covered by owner_scope_test.go"},
	{route: "/api/agent/key", why: "API group, covered by owner_scope_test.go"},
	{route: "/api/collections/", why: "API group, covered by owner_scope_test.go"},
	{route: "/api/tags/", why: "API group, covered by owner_scope_test.go"},

	// The login flow, registered only when a login is configured. It resolves
	// who the visitor is; it cannot depend on the answer.
	{route: "/auth/login", why: "the login form itself — it runs before anyone has an owner"},
	{route: "/auth/sso", why: "redirect to the identity provider"},
	{route: "/auth/callback", why: "lands the session that supplies the owner"},
	{route: "/auth/logout", why: "revokes the session; reads no library"},
}

const undeclaredOwnerScopeMsg = `GET %s is registered on the app origin but has no row in appOriginGETOwnerScope.

That list is an owner-scoping invariant (av-syug), not bookkeeping. The page routes
sit outside the API's auth group, so nothing about being an http.HandlerFunc gets a
handler the requester's owner — it arrives only because sessionGate resolved it and
ownerMiddleware backstopped it, which is what registering the route inside the page
group in setupRoutes buys. A page that skips that reads ownerIDFromCtx and gets
"nobody", and a page that reads the wrong owner serves another tenant's library and
mints frame tokens that render their artifacts and inline their state.

Add a row. If the route reads library data, set ownerScoped and give it ownPath and
foreignPath ("{id}" is substituted with an artifact belonging to the session owner and
then to another owner); the walk will prove it renders the first and refuses the
second. If it does not, say why in the row.

See docs/security.md §1.6.`

// --- fixture -----------------------------------------------------------

// twoOwnerInstance is an instance with a login and two real users, each with a
// library of their own. Two owners is the whole point: with one, every
// owner-scoping bug is invisible, which is exactly how this one shipped.
type twoOwnerInstance struct {
	ro       *Router
	ownerOne int64
	ownerTwo int64
	// artifactOne belongs to ownerOne — and ownerOne is defaultOwnerID, so it
	// is precisely what a page that fell back to the default would serve.
	artifactOne string
	artifactTwo string
	cookieTwo   *http.Cookie
}

func newTwoOwnerInstance(t *testing.T) twoOwnerInstance {
	t.Helper()
	ro, st := newIdentityTestRouter(t, &stubProvider{})
	ctx := context.Background()

	one, err := st.UpsertUser(ctx, "sub-one", "one@example.test")
	require.NoError(t, err)
	two, err := st.UpsertUser(ctx, "sub-two", "two@example.test")
	require.NoError(t, err)
	require.Equal(t, defaultOwnerID, one.ID,
		"the first user must land on the owner id a single-user library is already filed under")
	require.NotEqual(t, one.ID, two.ID)

	in := twoOwnerInstance{ro: ro, ownerOne: one.ID, ownerTwo: two.ID}
	in.artifactOne = seedOwnedArtifact(t, ro, one.ID, "owner-one-artifact", ownerOneTitle, ownerOneBody, ownerOneState)
	in.artifactTwo = seedOwnedArtifact(t, ro, two.ID, "owner-two-artifact", ownerTwoTitle, ownerTwoBody, ownerTwoState)
	in.cookieTwo = sessionCookieFor(t, st, two.ID, "session-owner-two")
	return in
}

// seedOwnedArtifact writes an artifact directly into the store, because the API
// can only create artifacts for the request's own owner and this fixture needs
// two. It carries a widget so the owner's gallery card renders a real frame —
// the default tile carries no render token, and the token is half of what is
// being asserted.
func seedOwnedArtifact(t *testing.T, ro *Router, owner int64, id, title, bodyMarker, stateMarker string) string {
	t.Helper()
	ctx := context.Background()
	blobID, widgetBlobID := id+"-blob", id+"-widget-blob"

	require.NoError(t, ro.cfg.Blob.Put(ctx, blobID,
		strings.NewReader("<html><body><h1>"+bodyMarker+"</h1></body></html>")))
	require.NoError(t, ro.cfg.Blob.Put(ctx, widgetBlobID,
		strings.NewReader("<html><body>"+bodyMarker+" tile</body></html>")))
	require.NoError(t, ro.cfg.Store.PutArtifact(ctx, &store.Artifact{
		ID: id, OwnerID: owner, Title: title,
		SourceBlobID: blobID, WidgetBlobID: widgetBlobID,
		Tier: store.Tier1, SourceText: bodyMarker,
	}))
	require.NoError(t, ro.cfg.Store.SetState(ctx, store.OwnerID(owner), id, store.ViewerID(owner), "note", stateMarker))
	return id
}

func sessionCookieFor(t *testing.T, st store.Store, userID int64, id string) *http.Cookie {
	t.Helper()
	require.NoError(t, st.CreateSession(context.Background(), &store.Session{
		ID: id, UserID: userID, ExpiresAt: time.Now().Add(time.Hour),
	}))
	return &http.Cookie{Name: sessionCookieName, Value: id}
}

func (in twoOwnerInstance) get(t *testing.T, path string, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if c != nil {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	return w
}

// responseText is everything a response says that could carry a render URL or a
// leaked marker. The Location header matters: /artifacts/{id}/open answers a
// redirect, and the token is in it.
func responseText(w *httptest.ResponseRecorder) string {
	return w.Body.String() + "\n" + w.Header().Get("Location")
}

// renderURLPattern finds the render-origin URLs a page points its frames and
// redirects at. It stops at "&" so a cache-busting parameter (rendered as
// "&amp;" in markup) is not swallowed into the token.
var renderURLPattern = regexp.MustCompile(
	`https://render\.test/([aw])/([^/?"'\s&]+)\?` + rendertoken.Param + `=([^"'\s&]+)`)

type renderCredential struct {
	url        string
	artifactID string
	token      string
}

func renderCredentialsIn(text string) []renderCredential {
	var out []renderCredential
	for _, m := range renderURLPattern.FindAllStringSubmatch(text, -1) {
		out = append(out, renderCredential{
			url:        "https://render.test/" + m[1] + "/" + m[2] + "?" + rendertoken.Param + "=" + m[3],
			artifactID: m[2],
			token:      m[3],
		})
	}
	return out
}

// assertFramesName is AC#2 at the seam it is decided: every render token a page
// emitted names the visitor it was rendered for. The render surface refuses a
// token whose owner does not own the artifact, so getting this right is what
// makes a wrong-library bug a blank page instead of a data leak.
func assertFramesName(t *testing.T, ro *Router, w *httptest.ResponseRecorder, want int64, where string) {
	t.Helper()
	for _, c := range renderCredentialsIn(responseText(w)) {
		claims, err := ro.tokens.Verify(c.token, c.artifactID)
		require.NoError(t, err, "%s minted an unverifiable token", where)
		assert.Equal(t, want, claims.OwnerID,
			"%s minted a frame token naming owner %d instead of the requester (%d). "+
				"renderURLs takes its owner from ownerIDFromCtx, so this is the page's owner being "+
				"wrong — and it is the difference between a listing bug and the render surface "+
				"serving another owner's body and inlined state (av-syug AC#2).",
			where, claims.OwnerID, want)
	}
}

// --- the walk ----------------------------------------------------------

func TestEveryAppOriginGETRouteDeclaresItsOwnerScope(t *testing.T) {
	in := newTwoOwnerInstance(t)

	declared := map[string]bool{}
	for _, row := range appOriginGETOwnerScope {
		require.False(t, declared[row.route], "duplicate row for %s", row.route)
		if row.ownerScoped {
			require.NotEmpty(t, row.ownPath, "%s: an owner-scoped row needs a path for the owner's own artifact", row.route)
			require.NotEmpty(t, row.foreignPath, "%s: an owner-scoped row needs a path for another owner's artifact", row.route)
		} else {
			require.NotEmpty(t, row.why, "%s: a row claiming exemption has to say why", row.route)
		}
		if row.scopedByHandler {
			require.False(t, row.ownerScoped, "%s: scopedByHandler is the alternative to this walk's own ownerScoped mechanism, not both", row.route)
			require.NotEmpty(t, row.coveredBy, "%s: a handler-scoped row must name the test that actually pins its ownership check", row.route)
		}
		declared[row.route] = true
	}

	seen := map[string]bool{}
	require.NoError(t, chi.Walk(in.ro.Mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != http.MethodGet || seen[route] {
			return nil
		}
		seen[route] = true
		assert.True(t, declared[route], undeclaredOwnerScopeMsg, route)
		return nil
	}))
	// And the reverse, so the walk can never pass by finding nothing.
	for route := range declared {
		assert.True(t, seen[route],
			"appOriginGETOwnerScope lists %s but the app mux no longer registers it — drop the stale row", route)
	}
}

// AC#1 and AC#2, over every owner-scoped route at once: a logged-in owner sees
// their own library and nothing of anyone else's, and every frame token the
// page emits names them.
func TestPageRoutesRenderTheSessionOwnersLibrary(t *testing.T) {
	in := newTwoOwnerInstance(t)

	exercised := 0
	for _, row := range appOriginGETOwnerScope {
		if !row.ownerScoped {
			continue
		}
		exercised++
		t.Run(row.route, func(t *testing.T) {
			// Their own artifact renders. Without this the assertions below
			// would pass on a route that had stopped answering anything —
			// including one registered outside the page group, where
			// ownerIDFromCtx now resolves to nobody.
			own := in.get(t, strings.ReplaceAll(row.ownPath, "{id}", in.artifactTwo), in.cookieTwo)
			require.Less(t, own.Code, 400,
				"%s must still serve the session owner their own artifact; got %d", row.ownPath, own.Code)
			assert.Contains(t, responseText(own), in.artifactTwo,
				"%s did not reference the session owner's own artifact — a route that answers nothing "+
					"passes the leak checks below for the wrong reason", row.ownPath)
			assertFramesName(t, in.ro, own, in.ownerTwo, "GET "+row.ownPath)

			// Another owner's artifact is not theirs to see. Owner one is
			// defaultOwnerID, so this is exactly the request the old silent
			// fallback answered with somebody else's library.
			foreign := in.get(t, strings.ReplaceAll(row.foreignPath, "{id}", in.artifactOne), in.cookieTwo)
			body := responseText(foreign)
			for marker, what := range map[string]string{
				ownerOneTitle: "title", ownerOneBody: "source", ownerOneState: "stored state",
			} {
				assert.NotContains(t, body, marker,
					"GET %s served owner %d another owner's %s. Page routes carry the session's owner "+
						"(sessionGate + ownerMiddleware, av-syug); a handler that reads a different one "+
						"is a cross-tenant read.", row.foreignPath, in.ownerTwo, what)
			}
			assertFramesName(t, in.ro, foreign, in.ownerTwo, "GET "+row.foreignPath)
		})
	}
	require.GreaterOrEqual(t, exercised, 7,
		"every server-rendered page and fragment that reads the library should be exercised here")
}

// The part that leaks data rather than filenames, end to end.
//
// A page names a principal in every frame token it mints. Before this ticket a
// second user's gallery minted owner 1's, and the render surface then served
// owner 1's body with owner 1's state inlined into the storage shim — inside
// the wrong user's page. So it is not enough to check the markup: the URLs the
// markup carries are followed to the render origin and the documents they
// produce are checked too.
func TestNoRenderURLOnASecondOwnersPageServesAnotherOwnersArtifactOrState(t *testing.T) {
	in := newTwoOwnerInstance(t)

	// Non-vacuity: the render surface really does hand over owner one's body
	// and inlined state to a token that names owner one. That document is what
	// a page minting the wrong principal puts on somebody else's screen.
	leak := renderGet(t, in.ro, "https://render.test/a/"+in.artifactOne+"?"+
		rendertoken.Param+"="+in.ro.tokens.Mint(in.artifactOne, in.ownerOne))
	require.Equal(t, http.StatusOK, leak.Code)
	require.Contains(t, leak.Body.String(), ownerOneBody)
	require.Contains(t, leak.Body.String(), ownerOneState,
		"the render surface inlines the principal's state; if it did not, this test would be asserting nothing")

	followed := 0
	for _, path := range []string{
		"/",
		"/artifacts/" + in.artifactOne,
		"/artifacts/" + in.artifactOne + "/edit",
		"/artifacts/" + in.artifactOne + "/open",
		"/agent?artifact=" + in.artifactOne,
		"/partials/agent-preview?artifact=" + in.artifactOne,
		"/partials/card-widget?artifact=" + in.artifactOne,
	} {
		page := in.get(t, path, in.cookieTwo)
		for _, c := range renderCredentialsIn(responseText(page)) {
			followed++
			doc := renderGet(t, in.ro, c.url)
			assert.NotContains(t, doc.Body.String(), ownerOneBody,
				"a frame URL on GET %s rendered another owner's artifact body", path)
			assert.NotContains(t, doc.Body.String(), ownerOneState,
				"a frame URL on GET %s inlined another owner's stored state into the storage shim — "+
					"this is the cross-tenant read, not a listing bug (av-syug AC#2)", path)
		}
	}
	// Owner two's own gallery card frame is among these, so a run that
	// followed nothing means the extraction stopped working, not that the
	// pages are clean.
	require.Positive(t, followed, "no render URLs were followed; the walk proved nothing")
}

// AC#3, and the half of the walk a session instance cannot perform.
//
// sessionGate is a mux-level middleware, so on an instance with a login *every*
// route gets the session's owner whether or not anybody meant it to — which
// means the two-owner walk above would pass for a page route registered outside
// the page group. A single-user instance issues no sessions, so there the owner
// comes from ownerMiddleware alone, and a route that skipped the group reads
// "nobody" and renders an empty library. Walking the same rows here is what
// makes group membership an enforced property rather than a convention.
//
// It is also the acceptance criterion in its own right: the self-hoster's
// instance must be untouched by any of this.
func TestSingleUserPagesStillRenderOwnerOnesLibrary(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Single User Shelf")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>tile</b>").Code)

	for _, row := range appOriginGETOwnerScope {
		if !row.ownerScoped {
			continue
		}
		t.Run(row.route, func(t *testing.T) {
			path := strings.ReplaceAll(row.ownPath, "{id}", id)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			require.Less(t, w.Code, 400, "GET %s: got %d", path, w.Code)
			assert.Contains(t, responseText(w), id,
				"GET %s rendered nothing of the only library on this instance. On a single-user "+
					"instance the owner comes from ownerMiddleware alone, so this is what a page route "+
					"registered outside the page group looks like (av-syug): ownerIDFromCtx answers "+
					"'nobody' and every scoped read comes back empty. Move the route into the group "+
					"in setupRoutes.", path)
			assertFramesName(t, r, w, defaultOwnerID, "GET "+path)
		})
	}
}

// AC#4. sessionGate now resolves an owner, and it must not shadow the one
// public mode resolves for a visitor who presented nothing. The two answer
// different questions and are reached on different branches: the gate never
// runs on /api/* at all, and ownerMiddleware's "never overwrite an owner
// resolved upstream" rule keeps them composable in either order.
func TestAnonymousPublicVisitorStillResolvesThePublicOwner(t *testing.T) {
	ro := newPublicLoginRouter(t, PublicMode{Enabled: true, OwnerID: otherOwner})

	published := seedOwnedArtifact(t, ro, otherOwner, "published-artifact", ownerTwoTitle, ownerTwoBody, ownerTwoState)
	private := seedOwnedArtifact(t, ro, defaultOwnerID, "private-artifact", ownerOneTitle, ownerOneBody, ownerOneState)

	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/artifacts", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), published,
		"an anonymous read of a public instance resolves PUBLIC_OWNER_ID, login configured or not")
	assert.NotContains(t, w.Body.String(), private,
		"the session default must not shadow the published owner (av-wmp6)")

	// And the pages are still gated: nothing marks a *page* request as a
	// public visitor yet, so an anonymous one is sent to log in. Opening the
	// pages to anonymous readers is av-eu3v/av-epnt's decision to make
	// deliberately, not one this ticket should make by accident.
	page := httptest.NewRecorder()
	ro.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusFound, page.Code)
	assert.Contains(t, page.Header().Get("Location"), "/auth/login")
}

// The two middlewares compose. sessionGate resolves the owner and the marker
// together; ownerMiddleware, running underneath it, leaves the owner alone and
// only supplies one when nobody upstream did.
func TestSessionOwnerReachesTheHandlerAlongsideTheSessionMarker(t *testing.T) {
	in := newTwoOwnerInstance(t)

	var owner int64
	var marked bool
	handler := in.ro.sessionGate(in.ro.ownerMiddleware(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			owner, marked = ownerIDFromCtx(r.Context()), sessionAuthed(r.Context())
		})))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(in.cookieTwo)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, in.ownerTwo, owner,
		"ownerMiddleware must not overwrite the owner sessionGate resolved")
	assert.True(t, marked,
		"the owner and the session-authed marker are complementary, not alternatives: "+
			"one decides whose library the page shows (av-syug), the other what credential it may "+
			"embed (av-5imk)")
}

// AC#6, as behaviour. ownerIDFromCtx used to answer defaultOwnerID for a
// request nobody attributed, and that silence is why the page routes shipped
// unscoped for as long as they did. It now fails closed, matching the choice
// the store layer already made for an unset ListOptions.OwnerID.
func TestOwnerIDFromCtxFailsClosed(t *testing.T) {
	assert.Equal(t, noOwner, ownerIDFromCtx(context.Background()),
		"a request nobody attributed has no owner; answering with one is how a cross-tenant "+
			"read becomes invisible (av-syug AC#6)")
	assert.NotEqual(t, defaultOwnerID, noOwner)

	// And the answer is inert rather than merely different: it matches no row,
	// so a handler that somehow reached the store with it gets an empty
	// library instead of somebody's.
	in := newTwoOwnerInstance(t)
	arts, err := in.ro.cfg.Store.ListArtifacts(context.Background(), store.ListOptions{OwnerID: noOwner, Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, arts, "the unattributed owner must match no artifact")

	a, err := in.ro.cfg.Store.GetArtifact(context.Background(), noOwner, in.artifactOne)
	require.NoError(t, err)
	assert.Nil(t, a)
}

// newPublicLoginRouter builds an instance that is both public and has a login —
// the configuration where the two owner-resolving paths meet.
func newPublicLoginRouter(t *testing.T, public PublicMode) *Router {
	t.Helper()

	f, err := os.CreateTemp("", "test-pageowner-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	blobDir, err := os.MkdirTemp("", "test-pageowner-blobs-*")
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
		Identity:     &stubProvider{identities: []auth.Identity{{ExternalID: "sub-1"}}},
		Public:       public,
	})
}
