package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sourceSnapshot(t *testing.T, r *Router, id string) (body, revision string) {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"?body=true", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var snapshot struct {
		Body           string
		SourceRevision string `json:"source_revision"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snapshot))
	require.NotEmpty(t, snapshot.SourceRevision)
	return snapshot.Body, snapshot.SourceRevision
}

func widgetSnapshot(t *testing.T, r *Router, id string) widgetResponse {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/widget", nil)
	require.Contains(t, []int{http.StatusOK, http.StatusNotFound}, w.Code)
	var snapshot widgetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snapshot))
	require.NotEmpty(t, snapshot.WidgetRevision)
	return snapshot
}

// Both clients hold the same read snapshot. Exactly one wins; the loser
// cannot change metadata, record a version, or leak its staged bytes.
func TestConcurrentConditionalContentSaves(t *testing.T) {
	for _, resource := range []string{"source", "widget"} {
		t.Run(resource, func(t *testing.T) {
			r, blobDir := newTestRouterWithBlobDir(t)
			id := createArtifact(t, r, map[string]any{"title": "original", "body": "<p>original</p>"})
			_, revision := sourceSnapshot(t, r, id)
			method, path, field := "PATCH", "/api/artifacts/"+id, "expected_source_revision"
			if resource == "widget" {
				revision = widgetSnapshot(t, r, id).WidgetRevision
				method, path, field = "PUT", path+"/widget", "expected_widget_revision"
			}
			type result struct {
				body     string
				response *httptest.ResponseRecorder
			}
			results := make(chan result, 2)
			start := make(chan struct{})
			for _, body := range []string{"<p>first</p>", "<p>second</p>"} {
				go func(body string) {
					<-start
					payload := map[string]any{"body": body, field: revision}
					if resource == "source" {
						payload["title"] = body
					}
					results <- result{body, doJSON(t, r, method, path, payload)}
				}(body)
			}
			close(start)
			var winner string
			codes := []int{}
			for range 2 {
				got := <-results
				codes = append(codes, got.response.Code)
				if got.response.Code == http.StatusOK {
					winner = got.body
				} else {
					assert.Contains(t, got.response.Body.String(), resource+"_conflict")
				}
			}
			require.ElementsMatch(t, []int{http.StatusOK, http.StatusConflict}, codes)
			var current, currentRevision string
			if resource == "source" {
				current, currentRevision = sourceSnapshot(t, r, id)
			} else {
				snapshot := widgetSnapshot(t, r, id)
				current, currentRevision = snapshot.Body, snapshot.WidgetRevision
			}
			assert.Equal(t, winner, current)
			assert.NotEqual(t, revision, currentRevision)
			if resource == "source" {
				encoded, err := json.Marshal(winner)
				require.NoError(t, err)
				assert.Equal(t, string(encoded), artifactField(t, r, id, "title"))
			}
			require.Len(t, listVersions(t, r, id), 2)
			entries, err := os.ReadDir(blobDir)
			require.NoError(t, err)
			assert.Len(t, entries, 2, "losing staged bytes must be reclaimed")

			// Stale, identical-to-current input must still conflict, including bundled fields.
			payload := map[string]any{"body": current, field: revision}
			if resource == "source" {
				payload["title"] = "must not be written"
			}
			w := doJSON(t, r, method, path, payload)
			assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
			require.Len(t, listVersions(t, r, id), 2)
			if resource == "source" {
				encoded, err := json.Marshal(winner)
				require.NoError(t, err)
				assert.Equal(t, string(encoded), artifactField(t, r, id, "title"))
			}

			// A fresh identical save returns the existing token and creates no version.
			payload[field] = currentRevision
			if resource == "source" {
				delete(payload, "title")
			}
			w = doJSON(t, r, method, path, payload)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), currentRevision)
			require.Len(t, listVersions(t, r, id), 2)
		})
	}
}

func TestContentRevisionsAreIndependentAndRestoreAware(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "original", "body": "<p>one</p>"})
	_, original := sourceSnapshot(t, r, id)
	emptyWidget := widgetSnapshot(t, r, id).WidgetRevision
	require.Equal(t, http.StatusOK, doJSON(t, r, "PUT", "/api/artifacts/"+id+"/widget", map[string]any{
		"body": "<b>tile</b>", "expected_widget_revision": emptyWidget,
	}).Code)
	tile := widgetSnapshot(t, r, id)
	require.Equal(t, http.StatusNoContent, doJSON(t, r, "PUT", "/api/artifacts/"+id+"/state", map[string]any{"key": "count", "value": "3"}).Code)
	require.Equal(t, http.StatusOK, doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"title": "renamed"}).Code)
	_, unchanged := sourceSnapshot(t, r, id)
	assert.Equal(t, original, unchanged)
	// Widget and state writes and title-only edits did not invalidate source.
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>two</p>", "expected_source_revision": original})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, tile.WidgetRevision, widgetSnapshot(t, r, id).WidgetRevision)
	_, edited := sourceSnapshot(t, r, id)
	// A restore reuses the immutable pointers, including the absent widget.
	w = doJSON(t, r, "POST", "/api/artifacts/"+id+"/versions/1/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body, restored := sourceSnapshot(t, r, id)
	assert.Equal(t, "<p>one</p>", body)
	assert.Equal(t, original, restored)
	assert.Equal(t, emptyWidget, widgetSnapshot(t, r, id).WidgetRevision)
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>two</p>", "expected_source_revision": edited})
	assert.Equal(t, http.StatusConflict, w.Code)
	w = doJSON(t, r, "PUT", "/api/artifacts/"+id+"/widget", map[string]any{"body": "<b>tile</b>", "expected_widget_revision": tile.WidgetRevision})
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAgentContentWritesRequireReadRevision(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createTestArtifact(t, r, "protected")
	grant, err := reg.Issue(1, id, "")
	require.NoError(t, err)
	_, source := sourceSnapshot(t, r, id)
	tile := widgetSnapshot(t, r, id)
	for _, tc := range []struct{ method, suffix, field, revision string }{
		{"PATCH", "", "expected_source_revision", source},
		{"PUT", "/widget", "expected_widget_revision", tile.WidgetRevision},
	} {
		w := doWithToken(t, r, tc.method, "/api/artifacts/"+id+tc.suffix, grant.Token(), map[string]any{"body": "<p>agent</p>"})
		assert.Equal(t, http.StatusPreconditionRequired, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "precondition_required")
		w = doWithToken(t, r, tc.method, "/api/artifacts/"+id+tc.suffix, grant.Token(), map[string]any{"body": "<p>agent</p>", tc.field: tc.revision})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
}

func TestConditionalSourceSaveRollsBackInvalidBundledMetadata(t *testing.T) {
	r, dir := newTestRouterWithBlobDir(t)
	id := createTestArtifact(t, r, "original")
	original, revision := sourceSnapshot(t, r, id)
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>new</p>", "expected_source_revision": revision, "owner_id": 2})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	body, current := sourceSnapshot(t, r, id)
	assert.Equal(t, original, body)
	assert.Equal(t, revision, current)
	require.Len(t, listVersions(t, r, id), 1)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestRefetchConflictsWithSourceEditDuringFetch(t *testing.T) {
	for _, unchanged := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed", true: "unchanged"}[unchanged], func(t *testing.T) {
			var wait atomic.Bool
			entered, release := make(chan struct{}), make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				if wait.Load() {
					close(entered)
					<-release
				}
				if wait.Load() && !unchanged {
					_, _ = w.Write([]byte("<p>refetched</p>"))
				} else {
					_, _ = w.Write([]byte("<p>initial</p>"))
				}
			}))
			defer upstream.Close()
			r, dir := newTestRouterWithBlobDir(t)
			id := createArtifact(t, r, map[string]any{"url": upstream.URL})
			_, revision := sourceSnapshot(t, r, id)
			wait.Store(true)
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				result <- doJSON(t, r, "POST", "/api/artifacts/"+id+"/refetch", map[string]any{"expected_source_revision": revision})
			}()
			<-entered
			w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>human edit</p>", "expected_source_revision": revision})
			close(release)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			w = <-result
			assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "source_conflict")
			body, _ := sourceSnapshot(t, r, id)
			assert.Equal(t, "<p>human edit</p>", body)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Len(t, entries, 2)
		})
	}
}

func TestConditionalWidgetRemoval(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "widget")
	empty := widgetSnapshot(t, r, id).WidgetRevision
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>first</b>").Code)
	first := widgetSnapshot(t, r, id).WidgetRevision
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>second</b>").Code)
	remove := func(revision string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("DELETE", "/api/artifacts/"+id+"/widget", nil)
		req.Header.Set("Authorization", authHeader())
		req.Header.Set("If-Match", revision)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	assert.Equal(t, http.StatusConflict, remove(first).Code)
	current := widgetSnapshot(t, r, id)
	assert.Equal(t, "<b>second</b>", current.Body)
	w := remove(current.WidgetRevision)
	require.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, empty, w.Header().Get("X-Widget-Revision"))
	assert.Equal(t, http.StatusConflict, remove(current.WidgetRevision).Code, "no-op deletion still checks precondition")
}

// Compile-time contract: refetch preconditions belong to the store mutation,
// not a handler-only check separated from its transaction.
func TestSnapshotStorePrecondition(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "snapshot")
	_, revision := sourceSnapshot(t, r, id)
	require.Equal(t, http.StatusOK, doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>new</p>"}).Code)
	_, err := r.cfg.Store.ReplaceArtifactSnapshot(context.Background(), 1, id, store.ArtifactSnapshot{SourceBlobID: "unpublished", ExpectedSourceRevision: &revision})
	assert.ErrorIs(t, err, store.ErrSourceChanged)
	body, _ := sourceSnapshot(t, r, id)
	assert.Equal(t, "<p>new</p>", body)
}

func contentRevisionForTest(t *testing.T, r *Router, id string) string {
	t.Helper()
	_, revision := sourceSnapshot(t, r, id)
	return revision
}

// Hooks the response's reload, after the first request's store transaction and
// storage locks have finished, to deterministically simulate another writer.
type advanceBeforeReloadStore struct {
	store.Store
	reads      int
	beforeRead func()
	afterRead  func()
}

func (s *advanceBeforeReloadStore) GetArtifact(ctx context.Context, ownerID int64, id string) (*store.Artifact, error) {
	s.reads++
	if s.reads == 2 {
		hook := s.beforeRead
		s.beforeRead = nil
		if hook != nil {
			hook()
		}
	}
	a, err := s.Store.GetArtifact(ctx, ownerID, id)
	if s.afterRead != nil {
		hook := s.afterRead
		s.afterRead = nil
		hook()
	}
	return a, err
}

func TestSourceSuccessRevisionBelongsToSubmittedBytes(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "identity")
	_, initial := sourceSnapshot(t, r, id)
	var laterRevision string
	wrapped := &advanceBeforeReloadStore{Store: r.cfg.Store}
	wrapped.beforeRead = func() {
		// A second writer commits between the first commit and its response read.
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>later</p>"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		_, laterRevision = sourceSnapshot(t, r, id)
	}
	r.cfg.Store = wrapped
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>submitted</p>", "expected_source_revision": initial})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response updateArtifactResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotEmpty(t, laterRevision)
	assert.NotEqual(t, laterRevision, response.Artifact.SourceRevision)
	assert.Equal(t, store.ContentRevision(response.Artifact.SourceBlobID), response.Artifact.SourceRevision)
	saved, err := r.readBlobString(context.Background(), response.Artifact.SourceBlobID)
	require.NoError(t, err)
	assert.Equal(t, "<p>submitted</p>", saved)
	// Using the returned token with the cached submitted bytes must now conflict.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>submitted again</p>", "expected_source_revision": response.Artifact.SourceRevision})
	assert.Equal(t, http.StatusConflict, w.Code)
	body, _ := sourceSnapshot(t, r, id)
	assert.Equal(t, "<p>later</p>", body)
}

// A racing save cannot pair new bytes with the token captured by an older
// SQL read. Historical immutable blobs remain readable after the new commit.
func TestSourceReadKeepsBytesAndRevisionTogether(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "snapshot", "body": "<p>old</p>"})
	_, original := sourceSnapshot(t, r, id)
	wrapped := &advanceBeforeReloadStore{Store: r.cfg.Store}
	wrapped.afterRead = func() {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>new</p>", "expected_source_revision": original})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	r.cfg.Store = wrapped
	body, revision := sourceSnapshot(t, r, id)
	assert.Equal(t, "<p>old</p>", body)
	assert.Equal(t, original, revision)
	body, revision = sourceSnapshot(t, r, id)
	assert.Equal(t, "<p>new</p>", body)
	assert.NotEqual(t, original, revision)
}

func TestExplicitSourcePreconditionAlsoGuardsEmptyNoopAndMetadata(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "original")
	_, revision := sourceSnapshot(t, r, id)
	require.Equal(t, http.StatusOK, doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": "<p>new</p>"}).Code)
	for _, payload := range []map[string]any{
		{"body": "", "expected_source_revision": revision, "title": "stale"},
		{"expected_source_revision": revision, "title": "stale"},
	} {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, payload)
		assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		assert.Equal(t, `"original"`, artifactField(t, r, id, "title"))
	}
}
