package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/artifact-viewer/artifact-viewer/internal/blob"
	"github.com/artifact-viewer/artifact-viewer/internal/secrets"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRouter(t *testing.T) *Router {
	t.Helper()

	f, err := os.CreateTemp("", "test-api-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	st, err := store.OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })

	blobDir, err := os.MkdirTemp("", "test-blobs-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(blobDir) })

	bl, err := blob.NewFSStore(blobDir)
	require.NoError(t, err)

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

func authHeader() string { return "Bearer secret" }

func TestAuthMiddleware(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/api/artifacts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest("GET", "/api/artifacts", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIngestAndGet(t *testing.T) {
	r := newTestRouter(t)

	body := map[string]any{
		"title":             "Test Artifact",
		"body":              "<html><body><h1>Hello</h1></body></html>",
		"network_allowlist": []string{},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.NotNil(t, resp["artifact"])
	art := resp["artifact"].(map[string]any)
	assert.Equal(t, "Test Artifact", art["title"])
	id := art["id"].(string)
	assert.NotEmpty(t, id)

	// GET the artifact
	req = httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, id, got.ID)
	assert.Equal(t, "Test Artifact", got.Title)
}

func TestIngestScan(t *testing.T) {
	r := newTestRouter(t)

	// Ingest without network_allowlist → should return footprint without saving
	body := map[string]any{
		"title": "CDN Tool",
		"body":  `<html><head><script src="https://cdn.jsdelivr.net/npm/chart.js"></script></head><body></body></html>`,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	footprint := resp["network_footprint"].([]interface{})
	assert.Contains(t, footprint, "https://cdn.jsdelivr.net")
}

func TestPatchArtifact(t *testing.T) {
	r := newTestRouter(t)

	// Create artifact
	body := map[string]any{
		"title":             "Original",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	id := resp["artifact"].(map[string]any)["id"].(string)

	// Patch
	patch := map[string]any{
		"title":             "Updated",
		"network_allowlist": []string{"https://example.com"},
	}
	pb, _ := json.Marshal(patch)
	req = httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(pb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated updateArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	require.NotNil(t, updated.Artifact)
	assert.Equal(t, "Updated", updated.Artifact.Title)
	assert.Equal(t, []string{"https://example.com"}, updated.Artifact.NetworkAllowlist)
	// No body changed, so the re-scan reports nothing to re-approve.
	assert.False(t, updated.FootprintChanged)
	assert.Empty(t, updated.NetworkFootprint)
}

func TestStateAPI(t *testing.T) {
	r := newTestRouter(t)

	// Create artifact
	body := map[string]any{"title": "S", "body": "<html></html>", "network_allowlist": []string{}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	id := resp["artifact"].(map[string]any)["id"].(string)

	// Set state
	stateBody := map[string]any{"key": "counter", "value": "42"}
	sb, _ := json.Marshal(stateBody)
	req = httptest.NewRequest("PUT", "/api/artifacts/"+id+"/state", bytes.NewReader(sb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Get state
	req = httptest.NewRequest("GET", "/api/artifacts/"+id+"/state", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var state map[string]string
	json.NewDecoder(w.Body).Decode(&state)
	assert.Equal(t, "42", state["counter"])
}

func TestShareCreate(t *testing.T) {
	r := newTestRouter(t)

	// Create artifact
	body := map[string]any{"title": "Share Test", "body": "<html></html>", "network_allowlist": []string{}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	id := resp["artifact"].(map[string]any)["id"].(string)

	// Create share
	shareBody := map[string]any{"artifact_id": id, "public": true}
	sb, _ := json.Marshal(shareBody)
	req = httptest.NewRequest("POST", "/api/shares", bytes.NewReader(sb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var shareResp map[string]any
	json.NewDecoder(w.Body).Decode(&shareResp)
	share := shareResp["share"].(map[string]any)
	assert.NotEmpty(t, share["id"])
	assert.Equal(t, id, share["artifact_id"])
}
