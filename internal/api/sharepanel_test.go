package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-6xjd: the owner's share panel, and the enumeration it exists to make
// possible.
//
// Three independent halves, tested as three: who has been given access, the
// public link, and whose data everybody writes. The fourth thing here is the
// gallery badge, which is what makes the answer visible without opening
// anything — the failure the whole ticket is aimed at is the share made months
// ago that nobody has thought about since.
//
// The client half (clicking a toggle writes the right request, and a bulk
// grant's report survives the re-render that follows it) is
// web/gallery/share.test.mjs, driven by running the shipped page script.

// --- naming a recipient -------------------------------------------------

// The resolution order, and why it is an order rather than a guess: a local
// account's login name IS its identity (external_id is UNIQUE, so the lookup is
// exact), while an OIDC identity has no login name at all and email is the only
// handle a person could type for it.
func TestARecipientIsNamedByLoginNameFirstAndEmailSecond(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()

	local, err := ro.cfg.Store.CreateLocalUser(ctx, store.NewLocalUser{
		ExternalID: auth.LocalExternalID("Alice"), Email: "alice", PasswordHash: "x"})
	require.NoError(t, err)
	federated, err := ro.cfg.Store.UpsertUser(ctx, "oidc-subject-opaque", "bob@example.test")
	require.NoError(t, err)

	// Typed however the person types it: the name is normalized on the way in,
	// which is what makes "one account per login name" hold for somebody who
	// capitalizes.
	for _, typed := range []string{"alice", "Alice", "  ALICE  "} {
		u, err := ro.resolveRecipient(ctx, typed)
		require.NoError(t, err, typed)
		assert.Equal(t, local.ID, u.ID, typed)
	}

	u, err := ro.resolveRecipient(ctx, "bob@example.test")
	require.NoError(t, err)
	assert.Equal(t, federated.ID, u.ID,
		"a provider subject is not something anybody could type; the address is the handle")

	_, err = ro.resolveRecipient(ctx, "nobody")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// --- granting -----------------------------------------------------------

// The bulk submit, and the report that makes it not all-or-nothing. Three
// names, three different outcomes, one request — because "alice, bob, tpyo"
// must neither refuse three people over one typo nor succeed without ever
// mentioning the typo.
//
// The `unknown` row is the deliberate leak: this field confirms existence,
// because answering "added" for a name nobody holds leaves the owner believing
// their friend has access when the friend has none.
func TestABulkGrantReportsEveryNameSeparately(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	seedLocalAccount(t, ro, "alice")
	seedLocalAccount(t, ro, "bob")
	artifactID := seedShareableArtifact(t, ro, "Tool")

	// Bob already holds a grant, so his row reports what is true rather than
	// failing the request: the owner wanted him to have access and he has it.
	require.NoError(t, ro.cfg.Store.CreateShare(ctx, defaultOwnerID, &store.Share{
		ID: "existing-bob", ArtifactID: artifactID,
		RecipientID: userIDOf(t, ro, "bob"),
	}))

	results := grantTo(t, ro, artifactID, []string{"alice", "bob", "tpyo"})
	require.Len(t, results, 3)
	assert.Equal(t, grantResult{Name: "alice", Status: grantStatusGranted,
		ShareID: results[0].ShareID}, results[0])
	assert.NotEmpty(t, results[0].ShareID)
	assert.Equal(t, grantStatusExisting, results[1].Status)
	assert.Equal(t, grantStatusUnknown, results[2].Status,
		"a name nobody holds must be reported, or the owner believes their friend has access")

	// And the one that succeeded really did: the guest list is the audit.
	shares := listShares(t, ro, artifactID)
	require.Len(t, shares.Grants, 2)
	assert.ElementsMatch(t, []string{"alice", "bob"},
		[]string{shares.Grants[0].Name, shares.Grants[1].Name})
}

// Granting to yourself is reported rather than silently skipped: a mistyped
// self-grant that looked like it worked is how somebody concludes they shared
// something they did not.
func TestGrantingToYourselfIsReportedNotStored(t *testing.T) {
	ro := newTestRouter(t)
	// The first account on an instance takes users.id 1, which is the owner
	// every API call here runs as — so naming it is naming yourself.
	seedLocalAccount(t, ro, ownerLoginName)
	artifactID := seedShareableArtifact(t, ro, "Tool")

	results := grantTo(t, ro, artifactID, []string{ownerLoginName})
	require.Len(t, results, 1)
	assert.Equal(t, grantStatusSelf, results[0].Status)
	assert.Empty(t, listShares(t, ro, artifactID).Grants)
}

// --- the public link ----------------------------------------------------

// The toggle's two directions, and the rotation that is neither. Replace is
// ONE request because the sequence a caller would otherwise write — delete,
// then create — leaves the artifact with no link at all when the second half
// fails, which is worse than either the old link or the new one.
func TestThePublicLinkIsOneRowThatCanBeRotatedInOneStep(t *testing.T) {
	ro := newTestRouter(t)
	artifactID := seedShareableArtifact(t, ro, "Tool")

	first := mintLink(t, ro, artifactID, false)
	assert.Contains(t, first.ShareURL, "http://render.test/s/",
		"a share is served from the render origin; that is the whole two-origin model")

	// A second mint is refused by the schema, which is what makes the control
	// a toggle rather than a button that accumulates links.
	req := httptest.NewRequest(http.MethodPost, "/api/shares",
		strings.NewReader(`{"artifact_id":"`+artifactID+`"}`))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)

	second := mintLink(t, ro, artifactID, true)
	assert.NotEqual(t, first.Share.ID, second.Share.ID)
	shares := listShares(t, ro, artifactID)
	require.NotNil(t, shares.Link)
	assert.Equal(t, second.Share.ID, shares.Link.ID,
		"exactly one link survives a rotation — the old URL stops working, which is the point")
}

// Replace names the public link, so asking for it on a grant is a request that
// means nothing. Refused rather than ignored: silently dropping it would make
// "replace" look like it did something.
func TestReplaceIsRefusedOnAGrant(t *testing.T) {
	ro := newTestRouter(t)
	seedLocalAccount(t, ro, "alice")
	artifactID := seedShareableArtifact(t, ro, "Tool")

	req := httptest.NewRequest(http.MethodPost, "/api/shares",
		strings.NewReader(`{"artifact_id":"`+artifactID+`","recipients":["alice"],"replace":true}`))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, listShares(t, ro, artifactID).Grants)
}

// --- whose data ---------------------------------------------------------

// The write path this ticket owns. The value check is the part that matters:
// share_state_mode is an enum the render path branches on, so an unrecognized
// value is a branch nobody wrote deciding whose rows a recipient writes.
func TestShareStateModeIsPatchableAsOneOfTwoValues(t *testing.T) {
	ro := newTestRouter(t)
	artifactID := seedShareableArtifact(t, ro, "Tool")

	assert.Equal(t, http.StatusOK, patchArtifactStatus(t, ro, artifactID, `{"share_state_mode":"shared"}`))
	a, err := ro.cfg.Store.GetArtifact(context.Background(), defaultOwnerID, artifactID)
	require.NoError(t, err)
	assert.Equal(t, store.ShareStateShared, a.ShareStateMode)

	for _, bad := range []string{`"everyone"`, `""`, `1`, `true`, `null`} {
		assert.Equal(t, http.StatusBadRequest,
			patchArtifactStatus(t, ro, artifactID, `{"share_state_mode":`+bad+`}`), bad)
	}
	a, err = ro.cfg.Store.GetArtifact(context.Background(), defaultOwnerID, artifactID)
	require.NoError(t, err)
	assert.Equal(t, store.ShareStateShared, a.ShareStateMode,
		"a refused mode must leave the stored one alone")
}

// --- the panel ----------------------------------------------------------

// The panel is the owner's, and the recipient's page must not carry it — not
// because the controls would fail (they would, all of them are owner-scoped)
// but because the *list* is the guest list, and a grant does not carry the
// right to see who else was given the artifact.
func TestOnlyTheOwnerGetsTheSharePanel(t *testing.T) {
	in := newGrantedInstance(t)

	owner := responseText(in.get(t, "/artifacts/"+in.granted, in.cookieOne))
	assert.Contains(t, owner, `id="share-panel"`)
	// The modal is opened from the toolbar (av-esa1), so the trigger is part
	// of the same gate rather than a separate question: a panel nobody can
	// open is the failure mode that replaced "a panel always taking space".
	assert.Contains(t, owner, `id="share-open"`)
	assert.Contains(t, owner, `id="share-link-toggle"`)
	assert.Contains(t, owner, `id="share-add-input"`)
	assert.Contains(t, owner, "/assets/gallery/share.js")

	recipient := responseText(in.get(t, "/artifacts/"+in.granted, in.cookieTwo))
	assert.NotContains(t, recipient, `id="share-panel"`)
	assert.NotContains(t, recipient, `id="share-open"`)
	assert.NotContains(t, recipient, "/assets/gallery/share.js")
	assert.NotContains(t, recipient, "/assets/htmx/htmx.min.js",
		"a page with no panel to swap should not carry the library that swaps it")
}

// A recipient's name is text somebody else chose — a login name, or an address
// an identity provider reported — and it is rendered into the owner's page. The
// escaping is html/template's, and this is the executable claim that nothing
// bypasses it by building the row in JavaScript instead.
func TestARecipientNameIsEscapedIntoTheOwnersPage(t *testing.T) {
	ro := newTestRouter(t)
	hostile := `"><img src=x onerror=alert(1)>`
	seedLocalAccount(t, ro, hostile)
	artifactID := seedShareableArtifact(t, ro, "Tool")
	results := grantTo(t, ro, artifactID, []string{hostile})
	require.Equal(t, grantStatusGranted, results[0].Status)

	page := getPage(t, ro, "/partials/share-panel?artifact="+artifactID)
	assert.NotContains(t, page, "<img src=x onerror=alert(1)>",
		"the name reached the page as markup; it is content, and only content")
	assert.Contains(t, page, "&lt;img src=x onerror=alert(1)&gt;")
}

// The panel and the fragment render the same partial, which is what keeps one
// definition of the guest list: after a grant the fragment shows what the full
// page render would.
func TestTheFragmentRendersTheSameListTheFullPageDoes(t *testing.T) {
	ro := newTestRouter(t)
	seedLocalAccount(t, ro, "alice")
	artifactID := seedShareableArtifact(t, ro, "Tool")
	grantTo(t, ro, artifactID, []string{"alice"})
	mintLink(t, ro, artifactID, false)

	fragment := getPage(t, ro, "/partials/share-panel?artifact="+artifactID)
	page := getPage(t, ro, "/artifacts/"+artifactID)
	for _, marker := range []string{"alice", `id="share-link-url"`, `id="share-state-mode"`} {
		assert.Contains(t, fragment, marker)
		assert.Contains(t, page, marker)
	}
}

// --- the shared tile ----------------------------------------------------

// av-ei5h: the public link section carries the widget's embed snippet
// whenever the link exists. The snippet names the link's widget URL, so it is
// live exactly while the link above it is — replacing the link replaces it,
// and the fragment re-render after any change keeps them in step. An artifact
// with no widget still gets the snippet (av-cp7j): that URL serves its
// default tile, and a widget added later takes over the same URL.
func TestThePanelEmbedsTheWidgetWheneverTheLinkExists(t *testing.T) {
	ro := newTestRouter(t)
	artifactID := seedShareableArtifact(t, ro, "Tool")

	panel := func() string {
		return getPage(t, ro, "/partials/share-panel?artifact="+artifactID)
	}
	assert.NotContains(t, panel(), `id="share-widget-embed"`,
		"no link: nothing to embed")

	mintLink(t, ro, artifactID, false)
	got := panel()
	assert.Contains(t, got, `id="share-widget-embed"`,
		"a link to an artifact with no widget embeds its default tile")
	assert.Contains(t, got, `/widget`, "the snippet must name the shared-widget route")
	assert.Contains(t, got, `&lt;iframe`, "the snippet is an iframe, escaped into the value attribute")

	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, artifactID, "<b>42 km</b>").Code)
	assert.Contains(t, panel(), `id="share-widget-embed"`)
}

// --- the badge ----------------------------------------------------------

// One badge naming the strongest thing true, and none at all for a private
// artifact — the absence is the signal, because a library of forty cards must
// not render forty badges and marking the default trains people to ignore the
// marker.
func TestTheCardBadgeNamesTheStrongestThingTrue(t *testing.T) {
	link := true
	for _, tc := range []struct {
		name   string
		a      store.Artifact
		level  string
		label  string
		detail string
	}{
		{name: "private", a: store.Artifact{}, level: ""},
		{
			name:  "one grant",
			a:     store.Artifact{ShareGrantCount: 1, ShareStateMode: store.ShareStateOwn},
			level: "granted", label: "Shared with 1",
			detail: "1 person can open this artifact. Each keeps their own data.",
		},
		{
			name:  "the public link outranks grants, and still names them",
			a:     store.Artifact{ShareGrantCount: 3, SharePublicLink: link, ShareStateMode: store.ShareStateOwn},
			level: "public", label: "Public link",
			detail: "3 people can open this artifact. Each keeps their own data. " +
				"Anyone with its public link can open it.",
		},
		{
			name:  "writing the owner's data outranks anyone reading it",
			a:     store.Artifact{ShareGrantCount: 2, SharePublicLink: link, ShareStateMode: store.ShareStateShared},
			level: "shared-data", label: "Shared data",
			detail: "2 people can open this artifact, and everyone using it reads and writes " +
				"one shared copy of its data. Anyone with its public link can open it.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := newShareBadgeView(&tc.a)
			assert.Equal(t, tc.level, got.Level)
			assert.Equal(t, tc.label, got.Label)
			assert.Equal(t, tc.detail, got.Detail)
		})
	}
}

// The badge is AMBIENT — in the card's markup, not behind a hover or a click —
// because the share nobody remembers is exactly the one a marker you have to
// go looking for will not surface.
func TestASharedArtifactsCardCarriesItsBadgeInTheMarkup(t *testing.T) {
	ro := newTestRouter(t)
	artifactID := seedShareableArtifact(t, ro, "Tool")

	before := getPage(t, ro, "/")
	assert.NotContains(t, before, "share-badge",
		"a private artifact gets no marker; the absence is the signal")

	mintLink(t, ro, artifactID, false)
	after := getPage(t, ro, "/")
	assert.Contains(t, after, `class="share-badge share-badge-public"`)
	assert.Contains(t, after, "Public link")
}

// --- helpers ------------------------------------------------------------

// ownerLoginName is the account that lands on defaultOwnerID.
//
// users.id starts at 1 and the first row created takes it, which is the id
// every API call in this file already runs as (the static token resolves to
// the default owner). So the first account seeded has to be one no test hands
// an artifact to — otherwise a test's first "friend" would in fact be the
// owner, and granting to them would report `self`.
const ownerLoginName = "the-owner"

// seedLocalAccount provisions an account somebody could be granted, making
// sure the owner's own row exists first so this one is genuinely somebody
// else.
func seedLocalAccount(t *testing.T, ro *Router, name string) int64 {
	t.Helper()
	ctx := context.Background()
	if name != ownerLoginName {
		if _, err := ro.cfg.Store.GetUserByExternalID(ctx, auth.LocalExternalID(ownerLoginName)); err != nil {
			require.ErrorIs(t, err, store.ErrNotFound)
			require.Equal(t, defaultOwnerID, seedLocalAccount(t, ro, ownerLoginName),
				"the first account created must be the one the API calls here run as")
		}
	}
	u, err := ro.cfg.Store.CreateLocalUser(ctx, store.NewLocalUser{
		ExternalID: auth.LocalExternalID(name), Email: name + "@example.test", PasswordHash: "x"})
	require.NoError(t, err)
	return u.ID
}

func userIDOf(t *testing.T, ro *Router, name string) *int64 {
	t.Helper()
	u, err := ro.cfg.Store.GetUserByExternalID(context.Background(), auth.LocalExternalID(name))
	require.NoError(t, err)
	return &u.ID
}

func grantTo(t *testing.T, ro *Router, artifactID string, names []string) []grantResult {
	t.Helper()
	body, err := json.Marshal(map[string]any{"artifact_id": artifactID, "recipients": names})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/shares", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp createShareResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Results
}

func mintLink(t *testing.T, ro *Router, artifactID string, replace bool) createShareResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{"artifact_id": artifactID, "replace": replace})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/shares", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp createShareResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func listShares(t *testing.T, ro *Router, artifactID string) artifactSharesView {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+artifactID+"/shares", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var view artifactSharesView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &view))
	return view
}

// patchArtifactStatus is the package's patchArtifact with the assertion taken
// off: half the cases here are 400s, and a helper that requires 200 cannot
// express "this value must be refused".  The body is raw JSON text rather than
// a map, for the same reason — `1` and `true` and `null` are exactly the
// share_state_mode values a map[string]any would let through untested.
func patchArtifactStatus(t *testing.T, ro *Router, artifactID, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/artifacts/"+artifactID, strings.NewReader(body))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w.Code
}
