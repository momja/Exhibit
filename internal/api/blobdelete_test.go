package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-7jcq. Deleting an artifact used to drop the row, let the FK cascade take
// its tags, collections, shares and state — and leave the body on disk
// forever. Every assertion here reads the *filesystem*, deliberately: a test
// written against the database rows passes against exactly the bug this ticket
// exists to fix.

// blobPath is where the test router's FSStore keeps the blob an artifact's
// `field` names. artifactField hands back raw JSON, so the id arrives quoted.
func blobPath(t *testing.T, r *Router, dir, id, field string) string {
	t.Helper()
	var blobID string
	require.NoError(t, json.Unmarshal([]byte(artifactField(t, r, id, field)), &blobID))
	require.NotEmpty(t, blobID, "%s is empty — the artifact has no such blob", field)
	return filepath.Join(dir, blobID)
}

func TestDeleteArtifactRemovesItsBodyFromDisk(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Doomed",
		"body":              "<html><body>bytes that must not survive</body></html>",
		"network_allowlist": []string{},
	})
	bodyPath := blobPath(t, r, blobDir, id, "source_blob_id")
	require.FileExists(t, bodyPath, "the ingest should have written the body")

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)

	_, err := os.Stat(bodyPath)
	assert.True(t, os.IsNotExist(err),
		"the artifact body must be gone from the blob store, got %v", err)
}

// An artifact has up to two blobs, and both are its own. A deletion that took
// only the body would leave the tile's document behind — the same orphan in a
// smaller file.
func TestDeleteArtifactRemovesItsWidgetFromDisk(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Has a tile",
		"body":              "<html><body>tool</body></html>",
		"network_allowlist": []string{},
	})
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)

	bodyPath := blobPath(t, r, blobDir, id, "source_blob_id")
	widgetPath := blobPath(t, r, blobDir, id, "widget_blob_id")
	require.FileExists(t, bodyPath)
	require.FileExists(t, widgetPath)

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)

	for _, p := range []string{bodyPath, widgetPath} {
		_, err := os.Stat(p)
		assert.True(t, os.IsNotExist(err), "%s must be gone, got %v", p, err)
	}
}

// An edit mints a new blob and leaves the old one to the version that named
// it, so an edited artifact has one file per version — and deleting the
// artifact must take every one of them, not only the head's. Anything less
// leaks the history of everything an owner ever deleted.
func TestDeleteArtifactAfterEditLeavesNoBlobBehind(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Edited",
		"body":              "<html><body>v1</body></html>",
		"network_allowlist": []string{},
	})
	before := artifactField(t, r, id, "source_blob_id")
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>v2</body></html>"})
	require.NotEqual(t, before, artifactField(t, r, id, "source_blob_id"),
		"an edit writes a new blob; the old one belongs to version 1")
	entries, err := os.ReadDir(blobDir)
	require.NoError(t, err)
	require.Len(t, entries, 2, "the replaced body is kept")

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)

	entries, err = os.ReadDir(blobDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no blob may outlive the only artifact that referenced it")
}

// Removing a widget is a new version, not a deletion of bytes: the version that
// had the tile still names it, so restoring that version brings the tile back.
// The bytes go when the artifact does.
func TestRemovingAWidgetKeepsItsBytesForTheVersionThatHadIt(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Tile then no tile",
		"body":              "<html><body>tool</body></html>",
		"network_allowlist": []string{},
	})
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>tile</b>").Code)
	widgetPath := blobPath(t, r, blobDir, id, "widget_blob_id")
	require.FileExists(t, widgetPath)

	req := httptest.NewRequest("DELETE", "/api/artifacts/"+id+"/widget", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	assert.Equal(t, `""`, artifactField(t, r, id, "widget_blob_id"), "the artifact has no widget now")
	require.FileExists(t, widgetPath, "but the version that had it still does")

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)
	_, err := os.Stat(widgetPath)
	assert.True(t, os.IsNotExist(err), "deleting the artifact takes the widget's bytes, got %v", err)
}

// av-8gyd. The bytes go without an operator doing anything, so the queue that
// makes that safe is empty again by the time the request returns — nothing is
// left for anyone to run.
func TestDeleteArtifactLeavesNothingQueued(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Automatic",
		"body":              "<html><body>bytes</body></html>",
		"network_allowlist": []string{},
	})
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)

	pending, err := r.cfg.Store.PendingBlobDeletions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pending, "the synchronous drain must retire what the delete queued")

	entries, err := os.ReadDir(blobDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// Two artifacts can name the same blob — that is what per-owner content
// addressing (av-20fk) is for — so deleting one of them must condemn nothing.
// The survivor is asked to *render* between the two deletes, because a blob
// removed too early is invisible in the row and obvious in the document.
func TestDeletingOneOfTwoArtifactsSharingABlobKeepsTheBytes(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)
	ctx := context.Background()

	const sharedBody = "<html><body>the same tool twice</body></html>"
	require.NoError(t, r.cfg.Blob.Put(ctx, "shared-blob", strings.NewReader(sharedBody)))
	for _, id := range []string{"twin-a", "twin-b"} {
		require.NoError(t, r.cfg.Store.PutArtifact(ctx, &store.Artifact{
			ID: id, OwnerID: defaultOwnerID, Title: id, SourceBlobID: "shared-blob", Tier: store.Tier1,
		}))
	}
	sharedPath := filepath.Join(blobDir, "shared-blob")

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, "twin-a").Code)

	pending, err := r.cfg.Store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending, "the queue must never hold an id a live row still names")
	require.FileExists(t, sharedPath)
	assert.Contains(t, renderArtifactDoc(t, r, "twin-b"), "the same tool twice",
		"the surviving artifact must still render its body")

	// The second delete takes the last reference, so now the bytes go.
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, "twin-b").Code)
	assert.NoFileExists(t, sharedPath)
	pending, err = r.cfg.Store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// renderArtifactDoc fetches an artifact through the render surface, the way a
// visitor's iframe does.
func renderArtifactDoc(t *testing.T, r *Router, id string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/a/"+id+"?"+rendertoken.Param+"="+r.tokens.Mint(id, defaultOwnerID), nil)
	w := httptest.NewRecorder()
	r.RenderHandler().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w.Body.String()
}

func deleteArtifactReq(t *testing.T, r *Router, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("DELETE", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func patchArtifact(t *testing.T, r *Router, id string, updates map[string]any) {
	t.Helper()
	b, err := json.Marshal(updates)
	require.NoError(t, err)
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
