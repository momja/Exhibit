package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

// Editing rewrites the body in place rather than minting a new id, so deleting
// an edited artifact still has exactly one file to remove — and it is the one
// holding the *edited* bytes. If an edit ever starts minting a new blob id,
// this test still passes but deleteArtifactBlobs stops being complete, which
// is why artifactBlobIDs states the assumption in its doc comment.
func TestDeleteArtifactAfterEditLeavesNoBlobBehind(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Edited",
		"body":              "<html><body>v1</body></html>",
		"network_allowlist": []string{},
	})
	before := artifactField(t, r, id, "source_blob_id")
	patchArtifact(t, r, id, map[string]any{"body": "<html><body>v2</body></html>"})
	require.Equal(t, before, artifactField(t, r, id, "source_blob_id"),
		"an edit is expected to rewrite the body in place")

	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)

	entries, err := os.ReadDir(blobDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no blob may outlive the only artifact that referenced it")
}

// Detaching a widget is the other place a blob loses its last reference: the
// column is cleared and the id is never reissued, so nothing can name those
// bytes again.
func TestDeleteWidgetRemovesItsBlobFromDisk(t *testing.T) {
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

	_, err := os.Stat(widgetPath)
	assert.True(t, os.IsNotExist(err), "the detached widget's bytes must be gone, got %v", err)
	// The artifact itself is untouched — only its tile was removed.
	require.FileExists(t, blobPath(t, r, blobDir, id, "source_blob_id"))
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
