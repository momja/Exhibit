package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-ep8k, AC#2/#3: every artifact route answers 404 for an artifact the
// requester does not own — the same 404 an unknown id gets.
//
// 403 is the specific wrong answer these tests exist to prevent. A permission
// error confirms the row exists, which turns the artifact routes into a
// membership oracle: an attacker holding no artifacts could enumerate ids and
// learn which ones are real. Because ownerMiddleware pins every request to
// owner 1 today, "the other owner" here is seeded directly into the store.

// otherOwner is the owner ownerMiddleware never resolves to.
const otherOwner int64 = 2

// seedForeignArtifact puts an artifact owned by someone other than the
// request owner, complete with a body blob, a widget, state, a share and an
// approved origin — so each route below has something real to refuse.
func seedForeignArtifact(t *testing.T, r *Router) string {
	t.Helper()
	ctx := context.Background()
	const id, blobID, widgetBlobID = "foreign-artifact", "foreign-blob", "foreign-widget-blob"

	require.NoError(t, r.cfg.Blob.Put(ctx, blobID, strings.NewReader("<html><body>theirs</body></html>")))
	require.NoError(t, r.cfg.Blob.Put(ctx, widgetBlobID, strings.NewReader("<html><body>their tile</body></html>")))
	require.NoError(t, r.cfg.Store.PutArtifact(ctx, &store.Artifact{
		ID: id, OwnerID: otherOwner, Title: "Someone Else's Tool",
		SourceBlobID: blobID, WidgetBlobID: widgetBlobID,
		SourceURL: "http://example.invalid/theirs", Tier: store.Tier1,
		NetworkAllowlist: []string{"https://theirs.example.com"},
		SourceText:       "distinctiveforeignterm",
	}))
	require.NoError(t, r.cfg.Store.SetState(ctx, store.OwnerID(otherOwner), id, store.ViewerID(otherOwner), "secret", "theirs"))
	return id
}

// seedForeignTagAndCollection returns ids of a tag and collection owned by
// the other owner, for the attach/detach cases.
func seedForeignTagAndCollection(t *testing.T, r *Router) (tagID, collectionID string) {
	t.Helper()
	ctx := context.Background()
	tagID, collectionID = "foreign-tag", "foreign-collection"
	require.NoError(t, r.cfg.Store.CreateTag(ctx, &store.Tag{ID: tagID, OwnerID: otherOwner, Name: "theirs"}))
	require.NoError(t, r.cfg.Store.CreateCollection(ctx, &store.Collection{ID: collectionID, OwnerID: otherOwner, Name: "theirs"}))
	return tagID, collectionID
}

// createOwnArtifact ingests an artifact through the API, so it belongs to the
// request owner the middleware supplies.
func createOwnArtifact(t *testing.T, r *Router) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"title": "Mine", "body": "<html><body>mine</body></html>", "network_allowlist": []string{},
	})
	w := do(t, r, "POST", "/api/artifacts", b)
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp["artifact"].(map[string]any)["id"].(string)
}

func do(t *testing.T, r *Router, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestArtifactRoutes404ForAnotherOwner walks every route that names an
// artifact id and asserts the cross-tenant response is byte-for-byte the
// unknown-id response.
func TestArtifactRoutes404ForAnotherOwner(t *testing.T) {
	patch, _ := json.Marshal(map[string]any{"title": "hijacked"})
	allowlist, _ := json.Marshal(map[string]any{"network_allowlist": []string{"https://evil.example.com"}})
	stateBody, _ := json.Marshal(map[string]any{"key": "planted", "value": "x"})
	widgetBody, _ := json.Marshal(map[string]any{"body": "<html><body>hijacked tile</body></html>"})
	originBody, _ := json.Marshal(map[string]any{"origin": "https://evil.example.com", "decision": "allow"})

	cases := []struct {
		name   string
		method string
		// path is formatted with the artifact id.
		path string
		body []byte
	}{
		{"GET artifact", "GET", "/api/artifacts/%s", nil},
		{"PATCH artifact", "PATCH", "/api/artifacts/%s", patch},
		{"PATCH allowlist (origin-decision write)", "PATCH", "/api/artifacts/%s", allowlist},
		{"DELETE artifact", "DELETE", "/api/artifacts/%s", nil},
		{"POST refetch", "POST", "/api/artifacts/%s/refetch", nil},
		{"GET state", "GET", "/api/artifacts/%s/state", nil},
		{"PUT state", "PUT", "/api/artifacts/%s/state", stateBody},
		{"DELETE state key", "DELETE", "/api/artifacts/%s/state?key=secret", nil},
		{"DELETE all state", "DELETE", "/api/artifacts/%s/state", nil},
		{"GET origins", "GET", "/api/artifacts/%s/origins", nil},
		{"POST origins (single-origin allow)", "POST", "/api/artifacts/%s/origins", originBody},
		{"DELETE origins", "DELETE", "/api/artifacts/%s/origins?origin=https%3A%2F%2Ftheirs.example.com", nil},
		{"GET widget", "GET", "/api/artifacts/%s/widget", nil},
		{"PUT widget", "PUT", "/api/artifacts/%s/widget", widgetBody},
		{"DELETE widget", "DELETE", "/api/artifacts/%s/widget", nil},
		{"POST widget/generate", "POST", "/api/artifacts/%s/widget/generate", nil},
		{"GET transcripts", "GET", "/api/artifacts/%s/transcripts", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(t)
			foreignID := seedForeignArtifact(t, r)

			got := do(t, r, tc.method, fmtPath(tc.path, foreignID), tc.body)
			ghost := do(t, r, tc.method, fmtPath(tc.path, "no-such-artifact"), tc.body)

			assert.NotEqual(t, http.StatusForbidden, got.Code,
				"403 confirms the artifact exists — that is the membership oracle av-ep8k forbids")
			assert.Equal(t, ghost.Code, got.Code,
				"another owner's id must answer exactly like an id that does not exist")
			assert.Equal(t, ghost.Body.String(), got.Body.String(),
				"another owner's id must answer byte-for-byte like an id that does not exist, "+
					"not just with the same status code")

			// Every route here is one whose owned equivalent would not 404,
			// so the shared answer must actually be 404 (transcripts is the
			// exception: it lists rows, and an empty list is a 200).
			if tc.name != "GET transcripts" {
				assert.Equal(t, http.StatusNotFound, got.Code)
			}
		})
	}
}

func fmtPath(pattern, id string) string {
	return strings.Replace(pattern, "%s", id, 1)
}

// TestForeignArtifactSurvivesTheRefusedWrites is the other half: the routes
// above must not have taken effect before answering 404.
func TestForeignArtifactSurvivesTheRefusedWrites(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	foreignID := seedForeignArtifact(t, r)

	patch, _ := json.Marshal(map[string]any{"title": "hijacked", "network_allowlist": []string{"https://evil.example.com"}})
	do(t, r, "PATCH", "/api/artifacts/"+foreignID, patch)
	stateBody, _ := json.Marshal(map[string]any{"key": "planted", "value": "x"})
	do(t, r, "PUT", "/api/artifacts/"+foreignID+"/state", stateBody)
	do(t, r, "DELETE", "/api/artifacts/"+foreignID+"/state", nil)
	do(t, r, "DELETE", "/api/artifacts/"+foreignID+"/widget", nil)
	// The per-origin route is the narrower of the two allowlist write paths
	// (av-kmwj) and reaches the same rows, so it gets the same guarantee.
	originBody, _ := json.Marshal(map[string]any{"origin": "https://evil.example.com", "decision": "allow"})
	do(t, r, "POST", "/api/artifacts/"+foreignID+"/origins", originBody)
	do(t, r, "DELETE", "/api/artifacts/"+foreignID+"/origins?origin=https%3A%2F%2Ftheirs.example.com", nil)
	do(t, r, "DELETE", "/api/artifacts/"+foreignID, nil)

	a, err := r.cfg.Store.GetArtifact(ctx, otherOwner, foreignID)
	require.NoError(t, err)
	require.NotNil(t, a, "the artifact must not have been deleted")
	assert.Equal(t, "Someone Else's Tool", a.Title)
	assert.Equal(t, []string{"https://theirs.example.com"}, a.NetworkAllowlist,
		"the render CSP is built from this list; no other owner may widen it — "+
			"by PATCH or by the per-origin route, and the seeded origin must "+
			"still be there, so neither may narrow it either")
	assert.NotEmpty(t, a.WidgetBlobID, "the widget must still be attached")

	state, err := r.cfg.Store.GetState(ctx, store.OwnerID(otherOwner), foreignID, store.ViewerID(otherOwner))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"secret": "theirs"}, state)
}

// AC#3: tag and collection membership, both route spellings.
func TestTagAndCollectionAttachRoutes404AcrossOwners(t *testing.T) {
	cases := []struct {
		name   string
		method string
		// path takes (artifactID, tagID, collectionID) via the replacer below.
		path string
		// foreignArtifact says which side of the pairing is the other
		// owner's; the other side belongs to the requester.
		foreignArtifact bool
	}{
		{"attach foreign artifact to own tag", "POST", "/api/artifacts/{artifact}/tags/{ownTag}", true},
		{"detach foreign artifact from own tag", "DELETE", "/api/artifacts/{artifact}/tags/{ownTag}", true},
		{"attach own artifact to foreign tag", "POST", "/api/tags/{tag}/artifacts/{ownArtifact}", false},
		{"detach own artifact from foreign tag", "DELETE", "/api/tags/{tag}/artifacts/{ownArtifact}", false},
		{"attach foreign artifact to own collection", "POST", "/api/artifacts/{artifact}/collections/{ownCollection}", true},
		{"attach own artifact to foreign collection", "POST", "/api/collections/{collection}/artifacts/{ownArtifact}", false},
		// No DELETE cases here: RemoveArtifactFromCollection is deliberately
		// idempotent (denySilentNoop in store/owner_scope_test.go) like
		// DeleteState/ClearState/DeleteOriginDecision, so a cross-owner
		// detach 204s as a no-op rather than 404ing — consistent with (not a
		// regression of) the 404-vs-403 contract this table checks.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(t)
			foreignArtifact := seedForeignArtifact(t, r)
			foreignTag, foreignCollection := seedForeignTagAndCollection(t, r)
			ownArtifact := createOwnArtifact(t, r)

			ownTag := postJSON(t, r, "/api/tags", map[string]any{"name": "mine"})
			ownCollection := postJSON(t, r, "/api/collections", map[string]any{"name": "mine"})

			path := tc.path
			for placeholder, value := range map[string]string{
				"{artifact}": foreignArtifact, "{tag}": foreignTag, "{collection}": foreignCollection,
				"{ownArtifact}": ownArtifact, "{ownTag}": ownTag, "{ownCollection}": ownCollection,
			} {
				path = strings.ReplaceAll(path, placeholder, value)
			}

			w := do(t, r, tc.method, path, nil)
			assert.Equal(t, http.StatusNotFound, w.Code,
				"a pairing that crosses owners must read as absent, not forbidden")
			assert.NotEqual(t, http.StatusForbidden, w.Code)
		})
	}
}

// postJSON creates a resource and returns its id.
func postJSON(t *testing.T, r *Router, path string, body map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(body)
	w := do(t, r, "POST", path, b)
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp["id"].(string)
}

// AC#1 at the API boundary: the listing and the search that feeds the gallery
// both stop at the owner.
func TestListAndSearchExcludeAnotherOwnersArtifacts(t *testing.T) {
	r := newTestRouter(t)
	seedForeignArtifact(t, r)
	ownID := createOwnArtifact(t, r)

	for _, path := range []string{"/api/artifacts", "/api/artifacts?q=distinctiveforeignterm"} {
		w := do(t, r, "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var arts []*store.Artifact
		require.NoError(t, json.NewDecoder(w.Body).Decode(&arts))
		for _, a := range arts {
			assert.NotEqual(t, otherOwner, a.OwnerID, "%s leaked another owner's artifact", path)
		}
		if path == "/api/artifacts" {
			require.Len(t, arts, 1)
			assert.Equal(t, ownID, arts[0].ID)
		} else {
			assert.Empty(t, arts, "a search matching only another owner's text must return nothing")
		}
	}
}

// The gallery pages read through the same owner-scoped store methods, so a
// foreign artifact renders the 404 page rather than someone else's source —
// including the edit page, which is where origin decisions are read.
func TestGalleryPages404ForAnotherOwner(t *testing.T) {
	r := newTestRouter(t)
	foreignID := seedForeignArtifact(t, r)

	for _, path := range []string{
		"/artifacts/" + foreignID,
		"/artifacts/" + foreignID + "/edit",
		"/partials/card-widget?artifact=" + foreignID,
	} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s served another owner's artifact", path)
		assert.NotContains(t, w.Body.String(), "Someone Else's Tool",
			"%s leaked the foreign artifact's title", path)
	}
}

// Shares are the sharpest case: a share row makes an artifact readable at the
// unauthenticated /s/:id path, so minting one for an artifact you don't own
// would be a publish primitive over someone else's library.
func TestShareRoutes404AcrossOwners(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	foreignID := seedForeignArtifact(t, r)

	body, _ := json.Marshal(map[string]any{"artifact_id": foreignID, "public": true})
	w := do(t, r, "POST", "/api/shares", body)
	assert.Equal(t, http.StatusNotFound, w.Code, "a share may only be minted by the artifact's owner")

	// A share the other owner already holds cannot be revoked either.
	require.NoError(t, r.cfg.Store.CreateShare(ctx, otherOwner,
		&store.Share{ID: "foreign-share", ArtifactID: foreignID, Public: true}))
	w = do(t, r, "DELETE", "/api/shares/foreign-share", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Read it back as its actual owner — deliberately not through the
	// unscoped accessor, whose call sites the store's grep tripwire keeps to
	// the render surface alone (av-ep8k AC#6).
	sh, err := r.cfg.Store.GetShare(ctx, otherOwner, "foreign-share")
	require.NoError(t, err)
	assert.NotNil(t, sh, "the share must survive another owner's delete")
}
