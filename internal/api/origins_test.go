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

// newArtifactWithOrigins creates an artifact through the API and returns its id.
func newArtifactWithOrigins(t *testing.T, r *Router, allowlist []string) string {
	t.Helper()
	buf, _ := json.Marshal(map[string]any{
		"title":             "Origins",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": allowlist,
	})
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(buf))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	return created["artifact"].(map[string]any)["id"].(string)
}

// originDecisionRequestFor issues one POST/DELETE against the per-origin route.
func originRequest(t *testing.T, r *Router, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rdr)
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// exhibit-fr7: the runtime prompt's "Allow" decides one origin. It must widen
// the allowlist (and so the next render's CSP) without disturbing the artifact's
// other decisions — the prompt only ever knows about the origin it just saw.
func TestSetOriginDecisionAllowWidensAllowlistOnly(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newArtifactWithOrigins(t, r, []string{"https://kept.example.com"})

	w := originRequest(t, r, "POST", "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://new.example.com", "decision": "allow", "source": "runtime_prompt",
	})
	require.Equal(t, http.StatusOK, w.Code)

	allowed, err := r.cfg.Store.AllowedOrigins(ctx, id)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"https://kept.example.com", "https://new.example.com"}, allowed)
}

// "Don't ask again" records a block. A block is a prompt-suppression marker
// only: it must never reach the allowlist the CSP is generated from.
func TestSetOriginDecisionBlockNeverReachesAllowlist(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newArtifactWithOrigins(t, r, nil)

	w := originRequest(t, r, "POST", "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://tracker.example.com", "decision": "block", "source": "runtime_prompt",
	})
	require.Equal(t, http.StatusOK, w.Code)

	allowed, err := r.cfg.Store.AllowedOrigins(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, allowed, "a block decision must not widen the allowlist")

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, id)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, store.DecisionBlock, decisions[0].Decision)
	assert.Equal(t, "runtime_prompt", decisions[0].Source)
}

// Forgetting a block returns the origin to undecided, so the runtime prompt can
// ask about it again — without this, "don't ask again" is a permanent trap.
func TestDeleteOriginDecisionForgetsABlock(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newArtifactWithOrigins(t, r, nil)
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, id, "https://tracker.example.com", store.DecisionBlock, "runtime_prompt"))

	w := originRequest(t, r, "DELETE",
		"/api/artifacts/"+id+"/origins?origin=https%3A%2F%2Ftracker.example.com", nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, decisions)
}

// The decision route writes straight into the CSP's input, so it validates its
// arguments rather than letting an arbitrary token land in the policy.
func TestSetOriginDecisionRejectsBadInput(t *testing.T) {
	r := newTestRouter(t)
	id := newArtifactWithOrigins(t, r, nil)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"bare hostname", map[string]any{"origin": "example.com", "decision": "allow"}},
		{"origin with a path", map[string]any{"origin": "https://example.com/x", "decision": "allow"}},
		{"wildcard", map[string]any{"origin": "*", "decision": "allow"}},
		{"unknown decision", map[string]any{"origin": "https://example.com", "decision": "maybe"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := originRequest(t, r, "POST", "/api/artifacts/"+id+"/origins", tc.body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}

	w := originRequest(t, r, "POST", "/api/artifacts/does-not-exist/origins", map[string]any{
		"origin": "https://example.com", "decision": "allow",
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}
