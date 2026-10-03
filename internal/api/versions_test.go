package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Version history, through the routes. The store tests pin the transaction;
// these pin that every way an artifact changes is wired to it — a manual edit,
// a widget save, a refetch, an agent write — and that a restore is the one
// thing that can return to an earlier version.

type versionRow struct {
	Seq       int    `json:"seq"`
	Origin    string `json:"origin"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
	HasState  bool   `json:"has_state"`
	Current   bool   `json:"current"`
}

func listVersions(t *testing.T, r *Router, id string) []versionRow {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/versions", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Versions []versionRow `json:"versions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Versions
}

func versionCount(t *testing.T, r *Router, id string) int {
	t.Helper()
	return len(listVersions(t, r, id))
}

func restoreReq(t *testing.T, r *Router, id string, seq int) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, r, "POST", fmt.Sprintf("/api/artifacts/%s/versions/%d/restore", id, seq), nil)
}

// A new artifact has exactly one version, and it is the head.
func TestANewArtifactHasOneVersion(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})

	vs := listVersions(t, r, id)
	require.Len(t, vs, 1)
	assert.Equal(t, 1, vs[0].Seq)
	assert.Equal(t, "initial", vs[0].Origin)
	assert.True(t, vs[0].Current)
}

// A manual edit is versioned, and what it replaces is kept.
func TestAManualEditCreatesAVersion(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"})

	vs := listVersions(t, r, id)
	require.Len(t, vs, 2)
	assert.Equal(t, 2, vs[0].Seq)
	assert.Equal(t, "edit", vs[0].Origin)
	assert.True(t, vs[0].Current)
	assert.True(t, vs[1].HasState, "the version it replaced carries a state snapshot")
	assert.Equal(t, "<html><body>two</body></html>", getArtifactBody(t, r, id))
}

// Saving what is already there, or changing only the title, is not a version.
func TestSavesThatChangeNothingRecordNothing(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})

	patchArtifact(t, r, id, map[string]any{"body": "<html><body>one</body></html>"})
	patchArtifact(t, r, id, map[string]any{"title": "Renamed"})
	assert.Equal(t, 1, versionCount(t, r, id))
}

// The requirement the design is shaped around, end to end: the state is
// captured as it stood right before the edit, and a restore returns both the
// code and that state.
func TestRestoreReturnsTheCodeAndTheStateTheVersionLeftBehind(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	putState(t, r, id, "score", "10")

	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"})
	putState(t, r, id, "score", "99")
	putState(t, r, id, "added-later", "x")

	w := restoreReq(t, r, id, 1)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var restored versionRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &restored))
	assert.Equal(t, 3, restored.Seq, "a restore is a new version, not a rewind")
	assert.Equal(t, "restore", restored.Origin)
	assert.Equal(t, "Restored v1", restored.Message)

	assert.Equal(t, "<html><body>one</body></html>", getArtifactBody(t, r, id))
	assert.Equal(t, map[string]string{"score": "10"}, getState(t, r, id))
	assert.Len(t, listVersions(t, r, id), 3, "nothing in the history was discarded")

	// And it can be undone: the state the restore replaced was snapshotted onto
	// version 2, so restoring it brings that back.
	require.Equal(t, http.StatusOK, restoreReq(t, r, id, 2).Code)
	assert.Equal(t, "<html><body>two</body></html>", getArtifactBody(t, r, id))
	assert.Equal(t, map[string]string{"score": "99", "added-later": "x"}, getState(t, r, id))
}

func TestRestoreRefusals(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})

	assert.Equal(t, http.StatusConflict, restoreReq(t, r, id, 1).Code, "the head is already current")
	assert.Equal(t, http.StatusNotFound, restoreReq(t, r, id, 9).Code)
	w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/versions/zero/restore", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// The widget rides with the body: a save and a removal are versions, and a
// restore brings the tile back with the code it belonged to.
func TestWidgetChangesAreVersionsAndRestoreBringsTheTileBack(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>tile</b>").Code) // v2
	tile := artifactField(t, r, id, "widget_blob_id")
	require.Equal(t, http.StatusNoContent, deleteWidgetReq(t, r, id).Code) // v3
	assert.Equal(t, `""`, artifactField(t, r, id, "widget_blob_id"))
	assert.Equal(t, 3, versionCount(t, r, id))

	// Removing a widget that is not there records nothing.
	require.Equal(t, http.StatusNoContent, deleteWidgetReq(t, r, id).Code)
	assert.Equal(t, 3, versionCount(t, r, id))

	require.Equal(t, http.StatusOK, restoreReq(t, r, id, 2).Code)
	assert.Equal(t, tile, artifactField(t, r, id, "widget_blob_id"), "version 2's tile is back")
}

// Versions are the owner's: another owner's artifact answers 404 and
// byte-for-byte like one that does not exist (TestArtifactRoutes404ForAnotherOwner
// covers both routes); a recipient with a grant is in the same position.

// An agent write is labelled with the chat that made it and the message it was
// answering — read off the grant the server holds, not off the request.
func TestAnAgentWriteIsLabelledWithItsSessionAndPrompt(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})

	grant, err := reg.Issue(1, id, "sess-42")
	require.NoError(t, err)
	grant.SetPrompt("  make it\ncount down  ")

	w := doWithToken(t, r, "PATCH", "/api/artifacts/"+id, grant.Token(),
		map[string]any{"body": "<html><body>two</body></html>"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	vs := listVersions(t, r, id)
	require.Len(t, vs, 2)
	assert.Equal(t, "agent", vs[0].Origin)
	assert.Equal(t, "sess-42", vs[0].SessionID)
	assert.Equal(t, "make it count down", vs[0].Message)
}

// An artifact an agent creates records the agent as the author of version 1.
func TestAnAgentCreatedArtifactIsAttributedToTheAgent(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	grant, err := reg.Issue(1, "", "sess-new")
	require.NoError(t, err)
	grant.SetPrompt("build me a timer")

	w := doWithToken(t, r, "POST", "/api/artifacts", grant.Token(),
		map[string]any{"title": "Timer", "body": "<html><body>timer</body></html>"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Artifact struct {
			ID string `json:"id"`
		} `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	vs := listVersions(t, r, resp.Artifact.ID)
	require.Len(t, vs, 1)
	assert.Equal(t, "agent", vs[0].Origin)
	assert.Equal(t, "sess-new", vs[0].SessionID)
	assert.Equal(t, "build me a timer", vs[0].Message)
}

// Returning an artifact to an earlier state is a person's decision. An agent
// session — steered by text Exhibit did not author — can neither read the
// history nor change it, which is deny-by-default and needs no code of its own:
// `versions` has no entry in agentSubResources.
func TestAnAgentCredentialCannotReachVersionHistory(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"})
	grant, err := reg.Issue(1, id, "sess")
	require.NoError(t, err)

	w := doWithToken(t, r, "GET", "/api/artifacts/"+id+"/versions", grant.Token(), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = doWithToken(t, r, "POST", "/api/artifacts/"+id+"/versions/1/restore", grant.Token(), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "<html><body>two</body></html>", getArtifactBody(t, r, id), "and nothing changed")
}

// A refetch is a version, and the body it replaces is kept; one that returns
// the document the artifact already holds records nothing.
//
// Ingest stores the page with an injected <base href>, which a refetch does not
// (TestRefetchArtifactOverwritesBody pins that), so the first refetch always
// differs from what ingest stored. The second, of the same page, is the one
// that is a no-op.
func TestRefetchIsAVersionAndAnUnchangedOneIsNot(t *testing.T) {
	page := "<html><head><title>Live</title></head><body>first</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, page)
	}))
	defer srv.Close()

	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"url": srv.URL})
	require.Equal(t, 1, versionCount(t, r, id))

	refetch := func() {
		w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/refetch", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	refetch()
	require.Equal(t, 2, versionCount(t, r, id))
	refetch()
	assert.Equal(t, 2, versionCount(t, r, id), "the same document records nothing")

	page = "<html><head><title>Live</title></head><body>second</body></html>"
	refetch()

	vs := listVersions(t, r, id)
	require.Len(t, vs, 3)
	assert.Equal(t, "refetch", vs[0].Origin)
	assert.Equal(t, srv.URL, vs[0].Message)

	// What ingest stored is one restore away.
	require.Equal(t, http.StatusOK, restoreReq(t, r, id, 1).Code)
	assert.Contains(t, getArtifactBody(t, r, id), `<base href="`+srv.URL+`">`)
}

// The edit page lists the history and offers a restore on every version but the
// current one. The ids are what versions.js looks up (the script test builds its
// own elements, so the template is where a misspelling would hide).
func TestEditPageListsVersionsAndOffersRestoreOnEarlierOnes(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>two</body></html>"})

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `id="versions-panel"`)
	assert.Contains(t, page, `id="versions-rows"`)
	assert.Contains(t, page, `id="versions-status"`)
	assert.Contains(t, page, "Versions (2)")
	assert.Contains(t, page, `/assets/gallery/versions.js`)

	// v1 can be restored; v2 is the one you are on.
	assert.Contains(t, page, `data-action="restore" data-seq="1"`)
	assert.NotContains(t, page, `data-action="restore" data-seq="2"`)
	assert.Contains(t, page, "Created")
	assert.Contains(t, page, "Edited")
}

// What a version's message says is untrusted text (a prompt, a URL), and it is
// rendered with the template's escaping like everything else on the page.
func TestEditPageEscapesVersionMessages(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "T", "body": "<html><body>one</body></html>"})
	grant, err := reg.Issue(1, id, "sess")
	require.NoError(t, err)
	grant.SetPrompt(`<script>alert(1)</script>`)
	w := doWithToken(t, r, "PATCH", "/api/artifacts/"+id, grant.Token(),
		map[string]any{"body": "<html><body>two</body></html>"})
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<script>alert(1)</script>")
	assert.Contains(t, rec.Body.String(), "&lt;script&gt;alert(1)&lt;/script&gt;")
}
