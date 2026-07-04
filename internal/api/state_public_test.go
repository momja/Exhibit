package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The storage shim runs in a sandboxed, opaque-origin iframe with no auth
// token, so the state routes must work without an Authorization header while
// the rest of the API stays authenticated.
func TestStateRoutesArePublicWithCORS(t *testing.T) {
	r := newTestRouter(t)

	// Create an artifact (this route stays authenticated).
	body := map[string]any{"title": "S", "body": "<html></html>", "network_allowlist": []string{}}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	id := resp["artifact"].(map[string]any)["id"].(string)

	// PUT state WITHOUT an Authorization header must succeed.
	sb, _ := json.Marshal(map[string]any{"key": "counter", "value": "42"})
	req = httptest.NewRequest("PUT", "/api/artifacts/"+id+"/state", bytes.NewReader(sb))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// GET state WITHOUT an Authorization header must return the value.
	req = httptest.NewRequest("GET", "/api/artifacts/"+id+"/state", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var state map[string]string
	json.NewDecoder(w.Body).Decode(&state)
	assert.Equal(t, "42", state["counter"])

	// OPTIONS preflight returns the render origin in CORS headers.
	req = httptest.NewRequest("OPTIONS", "/api/artifacts/"+id+"/state", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://render.test", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PUT")

	// Sanity: opening /state must NOT have opened the rest of the API.
	// A normal artifact route without auth still rejects.
	req = httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
