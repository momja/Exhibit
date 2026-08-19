package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/blob/blobtest"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

// av-52ll. The handlers were written against the Blob interface and this is the
// claim that costs nothing above it: the whole artifact lifecycle — ingest,
// read back, widget save, edit, refetch, delete — runs against a bucket with no
// handler change at all.
//
// It skips without a configured endpoint. The blob package's contract suite
// proves the backend keeps the interface's promises; this proves the promises
// were the right ones, by driving the real routes through it.
//
// Note what this deliberately does *not* do: point the rest of the api suite at
// a bucket. The deletion tests in blobdelete_test.go assert on the filesystem
// on purpose (av-7jcq) — they are about FSStore, and a version of them that
// asked the store instead would pass against the bug they exist to catch.

func newS3TestRouter(t *testing.T) *Router {
	t.Helper()
	bl := blobtest.S3OrSkip(t)

	f, err := os.CreateTemp("", "test-api-s3-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	box, err := secrets.Load("test-secret", "")
	require.NoError(t, err)

	return NewRouter(Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    "http://app.test",
		RenderOrigin: "http://render.test",
		AuthToken:    "secret",
		Secrets:      box,
	})
}

func TestArtifactLifecycleAgainstABucket(t *testing.T) {
	r := newS3TestRouter(t)

	body := `<html><body><h1>bucket-backed</h1></body></html>`
	id := createArtifact(t, r, map[string]any{
		"title":             "Bucket backed",
		"body":              body,
		"network_allowlist": []string{},
	})
	assert.Equal(t, body, getArtifactBody(t, r, id), "the ingested body must read back from the bucket")

	// An edit rewrites the object under the same id.
	edited := `<html><body><h1>bucket-backed v2</h1></body></html>`
	patchArtifact(t, r, id, map[string]any{"body": edited})
	assert.Equal(t, edited, getArtifactBody(t, r, id))

	// The widget is the artifact's second blob and takes the same path.
	const widget = "<html><body><b>42 km</b></body></html>"
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, widget).Code)
	req := httptest.NewRequest("GET", "/api/artifacts/"+id+"/widget", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var got widgetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, widget, got.Body)

	// Delete takes the row and both objects.
	require.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code)
	req = httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// URL ingest and refetch are the two paths that write a body the client never
// sent, so they are worth running against the bucket rather than assumed from
// the paste path.
func TestURLIngestAndRefetchAgainstABucket(t *testing.T) {
	r := newS3TestRouter(t)

	page := `<html><head><title>v1</title></head><body><h1>first</h1></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, page)
	}))
	defer srv.Close()

	id := createArtifact(t, r, map[string]any{"url": srv.URL, "network_allowlist": []string{}})
	assert.Contains(t, getArtifactBody(t, r, id), "first")

	page = `<html><head><title>v2</title></head><body><h1>second</h1></body></html>`
	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/refetch", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Contains(t, getArtifactBody(t, r, id), "second")
}

// The deletion contract from the caller's side: an artifact whose bytes are
// already gone still deletes cleanly, because Blob.Delete is idempotent all the
// way down. This is the retry-after-partial-failure shape, and it is the one an
// object store makes likely — the failure it recovers from is a network one.
func TestDeleteArtifactWhoseObjectsAreAlreadyGone(t *testing.T) {
	r := newS3TestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Half deleted",
		"body":              "<html><body>x</body></html>",
		"network_allowlist": []string{},
	})
	var blobID string
	require.NoError(t, json.Unmarshal([]byte(artifactField(t, r, id, "source_blob_id")), &blobID))
	require.NoError(t, r.cfg.Blob.Delete(context.Background(), blobID))

	assert.Equal(t, http.StatusNoContent, deleteArtifactReq(t, r, id).Code,
		"a delete must not fail on bytes a previous attempt already removed")
}
