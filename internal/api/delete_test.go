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

// TestDeleteArtifact verifies DELETE returns 204 and the artifact is gone afterwards.
func TestDeleteArtifact(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Doomed",
		"body":              "<html><body>bye</body></html>",
		"network_allowlist": []string{},
	})

	// DELETE returns 204 No Content.
	req := httptest.NewRequest("DELETE", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// A subsequent GET returns 404 — the artifact is gone.
	req = httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestDeleteArtifactCascadesState proves that deleting an artifact also removes its
// associated shim state (via ON DELETE CASCADE in the schema).
func TestDeleteArtifactCascadesState(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Stateful",
		"body":              "<html></html>",
		"network_allowlist": []string{},
	})

	// Write some shim state.
	stateBody := map[string]any{"key": "counter", "value": "42"}
	sb, _ := json.Marshal(stateBody)
	req := httptest.NewRequest("PUT", "/api/artifacts/"+id+"/state", bytes.NewReader(sb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// Confirm the state is readable.
	req = httptest.NewRequest("GET", "/api/artifacts/"+id+"/state", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var state map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&state))
	require.Equal(t, "42", state["counter"])

	// Delete the artifact.
	req = httptest.NewRequest("DELETE", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// The cascade removed all rows referencing the artifact, including its state.
	// Query the store directly — the artifact row is gone, so no state can remain.
	got, err := r.cfg.Store.GetState(req.Context(), id)
	require.NoError(t, err)
	assert.Empty(t, got, "shim state should be removed by ON DELETE CASCADE")
}

// TestEditPageHasDeleteConfirmation verifies the rendered edit page embeds the
// verbatim irreversible-deletion warning.
func TestEditPageHasDeleteConfirmation(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Editable",
		"body":              "<html><body>content</body></html>",
		"network_allowlist": []string{},
	})

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	page := w.Body.String()
	// The delete flow (confirm() + DELETE) lives in the static edit page
	// script the page loads.
	assert.Contains(t, page, `<script src="/assets/gallery/edit.js"></script>`)
	editJS, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	assert.Contains(t, string(editJS), "Are you sure you want to delete this artifact? The action cannot be reversed and all data will be lost.")
}
