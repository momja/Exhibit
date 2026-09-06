package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-awr4: what a granted non-owner actually gets when they open the ordinary
// artifact URL.
//
// A grant is not a link, so there is no second address and no second template.
// That makes the two pages easy to confuse and the confusion expensive: the
// difference between them is *which controls exist*, and a control that exists
// and 404s is worse than one that never appeared — it teaches the visitor that
// Exhibit is flaky rather than that the artifact is not theirs.
//
// The client half of that claim (no prompt is raised, no decision is written)
// is web/gallery/detail.recipient.test.mjs, driven by running the page script.
// This file is the server half: who reaches the page at all, what the page
// carries, and whose state the frame it points at inlines.

const recipientState = "RecipientStateMarker9f3c"

// grantedInstance is two owners, one artifact, and a grant between them. It
// builds on the two-owner fixture the page-owner walk already needs — the
// point of a grant is that it crosses exactly the boundary that walk asserts
// nothing crosses by accident.
type grantedInstance struct {
	twoOwnerInstance
	// artifactOne belongs to ownerOne; ownerTwo holds a grant on it and
	// nothing else. cookieTwo is ownerTwo's session.
	granted string
	// The owner's own session. Every claim about a recipient's page is made
	// against the same page rendered for its owner, so the fixture carries
	// both — the instance has a login, where an uncredentialed request is not
	// the owner but a redirect to /auth/login.
	cookieOne *http.Cookie
}

func newGrantedInstance(t *testing.T) grantedInstance {
	t.Helper()
	in := newTwoOwnerInstance(t)
	ctx := context.Background()

	require.NoError(t, in.ro.cfg.Store.CreateShare(ctx, in.ownerOne, &store.Share{
		ID: "grant-to-two", ArtifactID: in.artifactOne, RecipientID: &in.ownerTwo,
	}))
	// The recipient's own rows on somebody else's artifact — av-q0ub's
	// (artifact, viewer) key, which is what "their own state inlined" means.
	require.NoError(t, in.ro.cfg.Store.SetState(ctx,
		store.OwnerID(in.ownerOne), in.artifactOne, store.ViewerID(in.ownerTwo),
		"note", recipientState))
	// A source URL, so "Update from source" is a control the owner's page
	// actually renders — the template only emits it for a URL-ingested
	// artifact, and a control that renders for nobody would make the
	// owner-side control below assert nothing.
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, in.artifactOne,
		map[string]any{"source_url": "https://source.example.test/tool"}))

	return grantedInstance{
		twoOwnerInstance: in,
		granted:          in.artifactOne,
		cookieOne:        sessionCookieFor(t, in.ro.cfg.Store, in.ownerOne, "session-owner-one"),
	}
}

// AC#1 and AC#2 together, because each is the other's control: the grant is
// what makes the page reachable, and its absence is indistinguishable from an
// artifact that never existed.
func TestAGrantIsWhatOpensTheOrdinaryArtifactURL(t *testing.T) {
	in := newTwoOwnerInstance(t)

	before := in.get(t, "/artifacts/"+in.artifactOne, in.cookieTwo)
	assert.Equal(t, http.StatusNotFound, before.Code,
		"without a grant a stranger gets the 404 page — never a 403, which would "+
			"confirm the id names something and make the route an oracle over other "+
			"owners' libraries")
	assert.NotContains(t, before.Body.String(), ownerOneTitle)

	require.NoError(t, in.ro.cfg.Store.CreateShare(context.Background(), in.ownerOne,
		&store.Share{ID: "grant-to-two", ArtifactID: in.artifactOne, RecipientID: &in.ownerTwo}))

	after := in.get(t, "/artifacts/"+in.artifactOne, in.cookieTwo)
	require.Equal(t, http.StatusOK, after.Code)
	assert.Contains(t, after.Body.String(), ownerOneTitle,
		"a grant is read access to the artifact, and the title is part of reading it")
}

// The frame the recipient's page points at carries both principals separately
// (av-6axy), and the render surface acts on both: the owner authorizes the
// read, the recipient selects the state. Followed through to the render origin
// rather than asserted on the token alone — the token being right and the
// document being wrong is exactly the failure the page-owner walk was written
// for, one boundary over.
func TestTheRecipientsFrameRendersTheOwnersArtifactWithTheRecipientsState(t *testing.T) {
	in := newGrantedInstance(t)

	page := in.get(t, "/artifacts/"+in.granted, in.cookieTwo)
	require.Equal(t, http.StatusOK, page.Code)

	creds := renderCredentialsIn(responseText(page))
	require.Len(t, creds, 1, "the viewer page points at exactly one render document")

	claims, err := in.ro.tokens.Verify(creds[0].token, creds[0].artifactID)
	require.NoError(t, err)
	assert.Equal(t, in.ownerOne, claims.OwnerID,
		"the render surface checks the token's owner against the artifact's row, so a "+
			"token naming the recipient would 404 on their own shared artifact")
	assert.Equal(t, in.ownerTwo, claims.ViewerID,
		"and the state inlined is the viewer's, which is the whole of av-6axy's split")

	doc := renderGet(t, in.ro, creds[0].url)
	require.Equal(t, http.StatusOK, doc.Code)
	assert.Contains(t, doc.Body.String(), ownerOneBody, "the owner's artifact runs")
	assert.Contains(t, doc.Body.String(), recipientState, "under the recipient's own rows")
	assert.NotContains(t, doc.Body.String(), ownerOneState,
		"a grant is read access to the artifact, not to the owner's rows beneath it")
}

// AC#3. Every control below writes or reads something the server grants an
// owner alone, so each would answer 404 for a recipient. Listed as a table so
// a control added to detail.tmpl has an obvious place to be argued about.
func TestTheRecipientsPageOffersNothingItCannotDo(t *testing.T) {
	in := newGrantedInstance(t)

	ownerOnly := map[string]string{
		"the edit page":             "/artifacts/" + in.granted + "/edit",
		"the agent":                 "/agent?artifact=" + in.granted,
		"the self-contained export": "/api/artifacts/" + in.granted + "/export",
		"the allowlist settings":    "/artifacts/" + in.granted + "/edit#security-panel",
		"the popover's manage link": "capability-popover-manage",
		"the refetch button":        "refetchSource()",
	}

	recipient := in.get(t, "/artifacts/"+in.granted, in.cookieTwo).Body.String()
	for what, marker := range ownerOnly {
		assert.NotContains(t, recipient, marker,
			"the recipient's page offers %s, which the server would refuse — a control "+
				"that does nothing is how somebody concludes the tool is broken rather "+
				"than that it is not theirs", what)
	}
	assert.Contains(t, recipient, "READ_ONLY = true",
		"one flag decides what the page offers and what its script may send; two would "+
			"be two things to keep in agreement")

	// AC#7, and the control for every assertion above: on the owner's own page
	// each of these is present. Without it a template that dropped the toolbar
	// entirely would pass this test.
	owner := in.get(t, "/artifacts/"+in.artifactOne, in.cookieOne).Body.String()
	require.Contains(t, owner, ownerOneTitle, "the owner's page still renders")
	for what, marker := range ownerOnly {
		assert.Contains(t, owner, marker, "the owner's page lost %s", what)
	}
	assert.Contains(t, owner, "READ_ONLY = false")
}

// AC#4's server half: the explanation is markup the recipient's page carries
// and the owner's does not, so a blocked origin has somewhere to land. What it
// says once an origin arrives is asserted by running detail.js.
func TestOnlyTheRecipientsPageCarriesTheBlockedOriginExplanation(t *testing.T) {
	in := newGrantedInstance(t)

	recipient := in.get(t, "/artifacts/"+in.granted, in.cookieTwo).Body.String()
	assert.Contains(t, recipient, `id="origin-blocked-banner"`)
	assert.Contains(t, recipient, `id="origin-blocked-headline"`)
	assert.Contains(t, recipient, `id="origin-blocked-list"`)
	assert.Contains(t, recipient, "Only this artifact's owner can change what it is allowed to contact",
		"the failure this page exists to prevent is a tool that silently does nothing")

	owner := in.get(t, "/artifacts/"+in.artifactOne, in.cookieOne).Body.String()
	assert.NotContains(t, owner, "origin-blocked-banner",
		"the owner is prompted instead, and a page that both asks and says 'you cannot' "+
			"contradicts itself")
}

// The recipient's page still offers "Open in new tab", so /open has to answer
// them — a page whose own affordance 404s is the bug this ticket is about,
// arriving by a different door. It grants nothing new: a top-level render of an
// artifact they may already read, under that artifact's unchanged CSP.
func TestOpenInNewTabWorksForTheRecipientAndNotForAStranger(t *testing.T) {
	in := newGrantedInstance(t)

	page := in.get(t, "/artifacts/"+in.granted, in.cookieTwo).Body.String()
	require.Contains(t, page, `href="/artifacts/`+in.granted+`/open"`)

	w := in.get(t, "/artifacts/"+in.granted+"/open", in.cookieTwo)
	require.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	require.True(t, strings.HasPrefix(loc, "https://render.test/a/"+in.granted+"?"+rendertoken.Param+"="), loc)

	doc := renderGet(t, in.ro, strings.TrimPrefix(loc, "https://render.test"))
	assert.Equal(t, http.StatusOK, doc.Code, "the minted token must actually render")
	assert.Contains(t, doc.Body.String(), recipientState,
		"and top-level is still the recipient's own state, not the owner's")

	// A third account holds no grant, so the same route answers what an
	// artifact that does not exist answers.
	stranger := newStrangerSession(t, in)
	assert.Equal(t, http.StatusNotFound,
		in.get(t, "/artifacts/"+in.granted+"/open", stranger).Code)
	assert.Equal(t, http.StatusNotFound,
		in.get(t, "/artifacts/"+in.granted, stranger).Code)
}

// newStrangerSession adds a third account with no grant on anything — the
// control that separates "a grant admits this person" from "any logged-in
// person may read anything".
func newStrangerSession(t *testing.T, in grantedInstance) *http.Cookie {
	t.Helper()
	u, err := in.ro.cfg.Store.UpsertUser(context.Background(), "sub-three", "three@example.test")
	require.NoError(t, err)
	return sessionCookieFor(t, in.ro.cfg.Store, u.ID, "session-stranger")
}

// A grant admits its recipient to the viewer page and to nothing else. The
// store's own coverage of this is exhaustive (av-lrae runs every owner-scoped
// method as a grantee); what it cannot cover is the app's routes, which is
// where a page that reached for a wider accessor would show up.
func TestAGrantReachesTheViewerPageAndNoOtherAppRoute(t *testing.T) {
	in := newGrantedInstance(t)

	// The other app-origin pages that name an artifact. Two of them 404 outright
	// and the agent page answers 200 with nothing of the artifact in it — it
	// resolves the id it was handed and renders an empty session when it cannot,
	// which is that page's own behaviour and not this ticket's to change. What
	// both shapes have to agree on is the only thing asserted: none of the
	// owner's artifact comes back.
	for _, path := range []string{
		"/artifacts/" + in.granted + "/edit",
		"/agent?artifact=" + in.granted,
		"/partials/card-widget?artifact=" + in.granted,
		"/partials/agent-preview?artifact=" + in.granted,
	} {
		w := in.get(t, path, in.cookieTwo)
		body := responseText(w)
		for marker, what := range map[string]string{
			ownerOneBody: "source", ownerOneState: "stored state", ownerOneTitle: "title",
		} {
			assert.NotContains(t, body, marker,
				"GET %s served a grantee the artifact's %s. A grant is read access "+
					"through the viewer page; widening it anywhere else is what av-lrae's "+
					"separate accessor exists to prevent", path, what)
		}
	}
	assert.Equal(t, http.StatusNotFound,
		in.get(t, "/artifacts/"+in.granted+"/edit", in.cookieTwo).Code,
		"the edit page is where the source is read and rewritten; it is not a grant's to open")

	// And the API, where the owner-scoped queries answer directly. Each body is
	// valid for its route on purpose: a body that failed validation would 400
	// before the owner predicate ran, and the test would be asserting that the
	// request was malformed rather than that it was refused.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/artifacts/" + in.granted, ""},
		{http.MethodPatch, "/api/artifacts/" + in.granted, `{"title":"mine now"}`},
		{http.MethodDelete, "/api/artifacts/" + in.granted, ""},
		{http.MethodPost, "/api/artifacts/" + in.granted + "/origins",
			`{"origin":"https://api.example.com","decision":"allow","source":"runtime"}`},
		{http.MethodPut, "/api/artifacts/" + in.granted + "/state", `{"key":"k","value":"v"}`},
		{http.MethodGet, "/api/artifacts/" + in.granted + "/export", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(in.cookieTwo)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code,
			"%s %s must answer a grantee exactly as it answers an id that was never "+
				"issued; a 403 would confirm the row exists. Body: %s",
			tc.method, tc.path, w.Body.String())
	}
}
