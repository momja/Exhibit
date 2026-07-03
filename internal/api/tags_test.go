package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doJSON sends an authenticated request with an optional JSON body and
// returns the recorded response.
func doJSON(t *testing.T, r *Router, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createTestTag(t *testing.T, r *Router, name, color string) store.Tag {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/tags", map[string]string{"name": name, "color": color})
	require.Equal(t, http.StatusCreated, w.Code)
	var tag store.Tag
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tag))
	return tag
}

func createTestArtifact(t *testing.T, r *Router, title string) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
		"title": title, "body": "<html></html>", "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp["artifact"].(map[string]any)["id"].(string)
}

func TestTagUpdate(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "charts", "#FF0000")

	// Rename + recolor in one PATCH.
	w := doJSON(t, r, "PATCH", "/api/tags/"+tag.ID, map[string]string{"name": "graphs", "color": "#00FF00"})
	require.Equal(t, http.StatusOK, w.Code)
	var updated store.Tag
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.Equal(t, "graphs", updated.Name)
	assert.Equal(t, "#00FF00", updated.Color)

	// Partial PATCH: only color.
	w = doJSON(t, r, "PATCH", "/api/tags/"+tag.ID, map[string]string{"color": "#0000FF"})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.Equal(t, "graphs", updated.Name)
	assert.Equal(t, "#0000FF", updated.Color)

	// The change is global: visible on every artifact carrying the tag.
	artID := createTestArtifact(t, r, "A")
	w = doJSON(t, r, "POST", "/api/tags/"+tag.ID+"/artifacts/"+artID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = doJSON(t, r, "GET", "/api/artifacts/"+artID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var art store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	require.Len(t, art.Tags, 1)
	assert.Equal(t, "graphs", art.Tags[0].Name)
	assert.Equal(t, "#0000FF", art.Tags[0].Color)
}

func TestTagDelete(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "temp", "")
	artID := createTestArtifact(t, r, "A")

	w := doJSON(t, r, "POST", "/api/tags/"+tag.ID+"/artifacts/"+artID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	w = doJSON(t, r, "DELETE", "/api/tags/"+tag.ID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Tag is gone from the list and from the artifact.
	w = doJSON(t, r, "GET", "/api/tags", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var tags []store.Tag
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tags))
	assert.Empty(t, tags)

	w = doJSON(t, r, "GET", "/api/artifacts/"+artID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var art store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	assert.Empty(t, art.Tags)

	// Deleting again: 404.
	w = doJSON(t, r, "DELETE", "/api/tags/"+tag.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestTagAttachDetach(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "charts", "")
	artID := createTestArtifact(t, r, "A")

	w := doJSON(t, r, "POST", "/api/tags/"+tag.ID+"/artifacts/"+artID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Attach is idempotent.
	w = doJSON(t, r, "POST", "/api/tags/"+tag.ID+"/artifacts/"+artID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	w = doJSON(t, r, "DELETE", "/api/tags/"+tag.ID+"/artifacts/"+artID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Detaching a non-association: 404.
	w = doJSON(t, r, "DELETE", "/api/tags/"+tag.ID+"/artifacts/"+artID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestTagMutationFailures covers the failure paths of every tag mutation and
// asserts both the status code and the JSON error body shape.
func TestTagMutationFailures(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "existing", "")
	other := createTestTag(t, r, "other", "")
	artID := createTestArtifact(t, r, "A")

	tests := []struct {
		name   string
		method string
		path   string
		body   any
		want   int
	}{
		{"create duplicate name", "POST", "/api/tags", map[string]string{"name": "existing"}, http.StatusConflict},
		{"create empty name", "POST", "/api/tags", map[string]string{"name": ""}, http.StatusBadRequest},
		{"update missing tag", "PATCH", "/api/tags/nope", map[string]string{"name": "x"}, http.StatusNotFound},
		{"update to duplicate name", "PATCH", "/api/tags/" + other.ID, map[string]string{"name": "existing"}, http.StatusConflict},
		{"update to empty name", "PATCH", "/api/tags/" + tag.ID, map[string]string{"name": ""}, http.StatusBadRequest},
		{"delete missing tag", "DELETE", "/api/tags/nope", nil, http.StatusNotFound},
		{"attach missing tag", "POST", "/api/tags/nope/artifacts/" + artID, nil, http.StatusNotFound},
		{"attach missing artifact", "POST", "/api/tags/" + tag.ID + "/artifacts/nope", nil, http.StatusNotFound},
		{"detach missing tag", "DELETE", "/api/tags/nope/artifacts/" + artID, nil, http.StatusNotFound},
		{"detach missing artifact", "DELETE", "/api/tags/" + tag.ID + "/artifacts/nope", nil, http.StatusNotFound},
		{"detach non-association", "DELETE", "/api/tags/" + tag.ID + "/artifacts/" + artID, nil, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.want, w.Code)
			var body map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&body), "error responses must be JSON")
			assert.NotEmpty(t, body["error"])
		})
	}
}
