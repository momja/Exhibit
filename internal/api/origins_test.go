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
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, 1, id, "https://blocked.example.com", store.DecisionBlock, "runtime"))

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

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
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

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
	require.NoError(t, err)
	assert.Empty(t, decisions)
}

// av-i7hd: the allowlist is the input to the render CSP, so the single write
// path is where "an entry is an origin" is enforced. A path-bearing entry is
// refused rather than truncated to its host: truncating would grant a whole
// origin from a value the user approved as one file.
func TestCreateArtifactRejectsNonOriginAllowlist(t *testing.T) {
	r := newTestRouter(t)
	for name, entry := range map[string]string{
		"path":     "https://unpkg.com/@ffmpeg/ffmpeg@0.12.10/dist/esm/worker.js",
		"wildcard": "https://*.example.com",
		"keyword":  "'self'",
		"relative": "example.com",
	} {
		t.Run(name, func(t *testing.T) {
			w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
				"title":             "Bad allowlist",
				"body":              "<html><body>hi</body></html>",
				"network_allowlist": []string{"https://ok.example.com", entry},
			})
			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), entry,
				"the 400 names the offending value so the client can point at it")
		})
	}
}

func TestPatchArtifactRejectsNonOriginAllowlist(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	const seed = "https://ok.example.com"
	for name, entry := range map[string]string{
		"path":     "https://unpkg.com/dist/esm/worker.js",
		"wildcard": "https://*.example.com",
	} {
		t.Run(name, func(t *testing.T) {
			w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
				"title":             "Patch validation",
				"body":              "<html></html>",
				"network_allowlist": []string{seed},
			})
			require.Equal(t, http.StatusCreated, w.Code)
			var created createArtifactResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
			id := created.Artifact.ID

			w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
				"network_allowlist": []string{entry},
			})
			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), entry)

			decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
			require.NoError(t, err)
			require.Len(t, decisions, 1, "a rejected PATCH stores nothing; the seeded origin is untouched")
			assert.Equal(t, seed, decisions[0].Origin)
			assert.Equal(t, store.DecisionAllow, decisions[0].Decision)
		})
	}
}

// Spellings that differ only in case, a trailing dot, a default port, or a
// trailing slash are one origin — and therefore one decision row (§3.3), not
// four near-duplicates in the allowlist editor.
func TestAllowlistCollapsesNearDuplicates(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()

	w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
		"title": "Duplicates",
		"body":  "<html><body>hi</body></html>",
		"network_allowlist": []string{
			"https://UNPKG.com",
			"https://unpkg.com.",
			"https://unpkg.com:443/",
			"https://unpkg.com",
		},
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created createArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.Equal(t, []string{"https://unpkg.com"}, created.Artifact.NetworkAllowlist)

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, created.Artifact.ID)
	require.NoError(t, err)
	require.Len(t, decisions, 1, "one decision per (artifact, origin) is a row-level invariant")
	assert.Equal(t, "https://unpkg.com", decisions[0].Origin)

	// The same collapse on the edit page's save path.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+created.Artifact.ID, map[string]any{
		"network_allowlist": []string{"https://CDN.example.com/", "https://cdn.example.com."},
	})
	require.Equal(t, http.StatusOK, w.Code)
	var updated updateArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.Equal(t, []string{"https://cdn.example.com"}, updated.Artifact.NetworkAllowlist)
}
