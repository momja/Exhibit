package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
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

// --- av-kmwj: the per-origin decision route ---------------------------------

// newOriginsArtifact creates an artifact through the API and returns its id.
func newOriginsArtifact(t *testing.T, r *Router, allowlist []string) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/artifacts", map[string]any{
		"title":             "Origins",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": allowlist,
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created createArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	return created.Artifact.ID
}

// The runtime prompt's "Allow" decides one origin. It must widen the allowlist
// — and so the next render's CSP — without disturbing the artifact's other
// decisions: the prompt only ever knows about the origin it just saw, and the
// page it lives on holds no working copy of the rest.
func TestSetOriginDecisionAllowWidensAllowlistOnly(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, []string{"https://kept.example.com"})
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, 1, id, "https://refused.example.com", store.DecisionBlock, "runtime"))

	w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://new.example.com", "decision": "allow", "source": "runtime",
	})
	require.Equal(t, http.StatusOK, w.Code)

	allowed, err := r.cfg.Store.AllowedOrigins(ctx, 1, id)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"https://kept.example.com", "https://new.example.com"}, allowed,
		"one allow must not replace the allow set the way PATCH does")

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
	require.NoError(t, err)
	require.Len(t, decisions, 3, "the untouched block decision must survive a single-origin allow")
}

// "Don't ask again" records a block. A block is a prompt-suppression marker and
// nothing else: it must never reach the allowlist the CSP is generated from.
func TestSetOriginDecisionBlockNeverReachesAllowlist(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, nil)

	w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://tracker.example.com", "decision": "block", "source": "runtime",
	})
	require.Equal(t, http.StatusOK, w.Code)

	allowed, err := r.cfg.Store.AllowedOrigins(ctx, 1, id)
	require.NoError(t, err)
	assert.Empty(t, allowed, "a block decision must not widen the allowlist")

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
	require.NoError(t, err)
	require.Len(t, decisions, 1)
	assert.Equal(t, store.DecisionBlock, decisions[0].Decision)
	assert.Equal(t, "runtime", decisions[0].Source)
}

// An allow over an existing block flips the one row rather than adding a second,
// contradictory one — the (artifact, origin) primary key is what makes that an
// invariant, and this is the route most likely to test it.
func TestSetOriginDecisionAllowOverridesABlock(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, nil)
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, 1, id, "https://cdn.example.com", store.DecisionBlock, "runtime"))

	w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://cdn.example.com", "decision": "allow",
	})
	require.Equal(t, http.StatusOK, w.Code)

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
	require.NoError(t, err)
	require.Len(t, decisions, 1, "one decision per (artifact, origin), not two that disagree")
	assert.Equal(t, store.DecisionAllow, decisions[0].Decision)
}

// Forgetting a block returns the origin to undecided, so the runtime prompt can
// ask about it again. Without this the "don't ask again" answer is a one-way
// trap: a blocked origin never prompts on its own.
func TestDeleteOriginDecisionForgetsABlock(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, nil)
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, 1, id, "https://tracker.example.com", store.DecisionBlock, "runtime"))

	w := doJSON(t, r, "DELETE",
		"/api/artifacts/"+id+"/origins?origin=https%3A%2F%2Ftracker.example.com", nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
	require.NoError(t, err)
	assert.Empty(t, decisions, "forgetting must remove the row, not flip it to allow")
}

// The delete route names the origin in a query parameter, never a path segment:
// an origin is caller-supplied text, and a path segment is resolved by the URL
// parser before the request is sent (av-hh1o). Absence is a 400 rather than a
// silent no-op, because "forget nothing" is not a request anyone makes.
func TestDeleteOriginDecisionRequiresAnOrigin(t *testing.T) {
	r := newTestRouter(t)
	id := newOriginsArtifact(t, r, nil)
	w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/origins", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// The route writes straight into the CSP's input, so it enforces the same
// "an entry is an origin" rule as ingest and PATCH (av-i7hd) rather than
// letting an arbitrary token land in the policy.
func TestSetOriginDecisionRejectsBadInput(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, nil)

	for name, body := range map[string]map[string]any{
		"bare hostname":      {"origin": "example.com", "decision": "allow"},
		"origin with a path": {"origin": "https://example.com/worker.js", "decision": "allow"},
		"wildcard":           {"origin": "https://*.example.com", "decision": "allow"},
		"csp keyword":        {"origin": "'self'", "decision": "allow"},
		"unknown decision":   {"origin": "https://example.com", "decision": "maybe"},
		"missing decision":   {"origin": "https://example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/origins", body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}

	decisions, err := r.cfg.Store.ListOriginDecisions(ctx, 1, id)
	require.NoError(t, err)
	assert.Empty(t, decisions, "a rejected request stores nothing")

	w := doJSON(t, r, "POST", "/api/artifacts/does-not-exist/origins", map[string]any{
		"origin": "https://example.com", "decision": "allow",
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// The route normalizes exactly as the allowlist write paths do, so the prompt
// may post the origin however the violation report spelled it and still land on
// the one row the CSP is built from.
func TestSetOriginDecisionNormalizesTheOrigin(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, nil)

	w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://CDN.example.com:443/", "decision": "allow",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var got originDecisionResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "https://cdn.example.com", got.Origin)

	allowed, err := r.cfg.Store.AllowedOrigins(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://cdn.example.com"}, allowed)
}

// listOriginDecisions is the read path that tells the three origin states
// apart; allow and block both come back, since a client that saw only the
// allowlist could not distinguish "refused" from "not yet asked".
func TestListOriginDecisionsReturnsBothKinds(t *testing.T) {
	r := newTestRouter(t)
	ctx := context.Background()
	id := newOriginsArtifact(t, r, []string{"https://ok.example.com"})
	require.NoError(t, r.cfg.Store.SetOriginDecision(ctx, 1, id, "https://no.example.com", store.DecisionBlock, "runtime"))

	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/origins", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var decisions []store.OriginDecision
	require.NoError(t, json.NewDecoder(w.Body).Decode(&decisions))
	require.Len(t, decisions, 2)
	assert.Equal(t, store.DecisionBlock, decisions[0].Decision)
	assert.Equal(t, store.DecisionAllow, decisions[1].Decision)
}

// An agent session is steered by text Exhibit did not author, so it must not be
// able to approve its own network egress — the one decision the whole
// scan/approve/allowlist model reserves for a person. agentSubResources is a
// deny-by-default allowlist, and this pins that `origins` stays off it.
func TestAgentSessionCannotDecideOrigins(t *testing.T) {
	scope := agentscope.Scope{OwnerID: 1, ArtifactID: "abc"}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		assert.False(t, agentScopeAllows(scope, method, "/api/artifacts/abc/origins"),
			"%s on the per-origin route must stay outside an agent session's reach", method)
	}
	// The control: a route that IS in reach, so an always-false bug in the
	// check could not make this test pass.
	assert.True(t, agentScopeAllows(scope, http.MethodGet, "/api/artifacts/abc/state"))
}
