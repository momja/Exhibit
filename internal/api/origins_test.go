package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exhibit-x87: the edit page saves with one PATCH carrying the whole working
// network_allowlist. That translation to allow-row upserts must touch allow
// rows only — a block decision the page never saw ("don't ask again",
// exhibit-fr7) must survive the save, and must not leak into the CSP-driving
// allowlist either.
func TestPatchAllowlistPreservesBlockDecisions(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	create := map[string]any{
		"title":             "Blocked origins",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{"https://old.example.com"},
	}
	buf, _ := json.Marshal(create)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(buf))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["artifact"].(map[string]any)["id"].(string)

	// A runtime "don't ask again" answer, recorded outside the edit page.
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, id, "https://blocked.example.com", store.DecisionBlock, "runtime"))

	patch := map[string]any{"network_allowlist": []string{"https://new.example.com"}}
	buf, _ = json.Marshal(patch)
	req = httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(buf))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated updateArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.Equal(t, []string{"https://new.example.com"}, updated.Artifact.NetworkAllowlist,
		"the PATCHed allowlist replaces the allow rows and never includes a blocked origin")

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, id)
	require.NoError(t, err)
	require.Len(t, decisions, 2, "the block decision must survive an allowlist-only save")
	assert.Equal(t, "https://blocked.example.com", decisions[0].Origin)
	assert.Equal(t, store.DecisionBlock, decisions[0].Decision)
}

// exhibit-x87: an artifact's origin decisions are child rows, so deleting the
// artifact takes them with it (ON DELETE CASCADE) — no orphans behind the API.
func TestDeleteArtifactCascadesOriginDecisions(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	create := map[string]any{
		"title":             "Cascade",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{"https://ok.example.com"},
	}
	buf, _ := json.Marshal(create)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(buf))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["artifact"].(map[string]any)["id"].(string)

	req = httptest.NewRequest("DELETE", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, decisions)
}
