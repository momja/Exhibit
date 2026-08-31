package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/momja/Exhibit/internal/origin"
	"github.com/momja/Exhibit/internal/store"
)

// Per-origin network decision routes (av-kmwj, reviving exhibit-fr7).
//
// PATCH /api/artifacts/:id carries the artifact's *whole* allow set and is what
// the edit page's Save uses. These routes decide a single origin, which is what
// the runtime permission prompt needs: it learns about one blocked origin at a
// time, and it must not restate — and so risk clobbering — decisions made
// somewhere else since the page it lives on was rendered.
//
// The two write paths differ in what they can express, not only in width. PATCH
// replaces the allow rows and deliberately leaves block rows alone
// (TestPatchAllowlistPreservesBlockDecisions), so it can neither record a block
// nor return an origin to undecided. Both are this file's.

type originDecisionRequest struct {
	Origin   string `json:"origin"`
	Decision string `json:"decision"`
	// Source records where the decision came from. Informational only — the
	// handler defaults it rather than trusting an omitted value, so a row
	// always says something about its provenance.
	Source string `json:"source"`
}

type originDecisionResponse struct {
	Origin   string `json:"origin"`
	Decision string `json:"decision"`
	Source   string `json:"source"`
}

// setOriginDecision upserts one origin's decision. decision='allow' widens the
// artifact's CSP on its next render; decision='block' records only a "don't ask
// again" answer, which the render preamble reads to stay quiet and which never
// reaches the policy.
func (ro *Router) setOriginDecision(w http.ResponseWriter, r *http.Request) {
	artifactID := urlParamID(r, "artifactID")

	var req originDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	// The same origin rule the allowlist goes through at ingest and on PATCH
	// (av-i7hd): this is the single write path, and an allow row is pasted
	// verbatim into a CSP header, where a path-bearing entry is path-matched
	// and means something other than what the prompt showed. Normalizing here
	// also keeps "one decision per (artifact, origin)" true against a caller
	// that spells a host four different ways.
	normalized, err := origin.NormalizeOrigin(req.Origin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Decision != store.DecisionAllow && req.Decision != store.DecisionBlock {
		writeError(w, http.StatusBadRequest, `decision must be "allow" or "block"`)
		return
	}
	source := req.Source
	if source == "" {
		source = "user"
	}

	ownerID := ownerIDFromCtx(r.Context())
	if !ro.artifactExists(w, r, artifactID, "set origin decision") {
		return
	}
	if err := ro.cfg.Store.SetOriginDecision(r.Context(), ownerID, artifactID, normalized, req.Decision, source); err != nil {
		writeArtifactError(w, r, "set origin decision", err)
		return
	}
	slog.InfoContext(r.Context(), "origin decision set",
		slog.String("artifact_id", artifactID),
		slog.String("origin", normalized),
		slog.String("decision", req.Decision),
		slog.String("source", source))
	writeJSON(w, http.StatusOK, originDecisionResponse{
		Origin: normalized, Decision: req.Decision, Source: source,
	})
}

// deleteOriginDecision drops one origin's decision, returning it to undecided.
// This is how a "don't ask again" block is forgotten: a blocked origin never
// prompts again on its own, so without this the answer would be a one-way trap.
//
// The origin travels as a query parameter rather than a path segment, for the
// reason the state routes established (av-hh1o): the value is arbitrary
// caller-supplied text, and a path segment is resolved by the browser's URL
// parser before the request is sent — an origin containing ".." would land on
// a different route entirely.
func (ro *Router) deleteOriginDecision(w http.ResponseWriter, r *http.Request) {
	artifactID := urlParamID(r, "artifactID")
	// Presence, not emptiness: an absent parameter is a malformed request,
	// where an empty one is a value that simply matches no row.
	q := r.URL.Query()
	if !q.Has("origin") {
		writeError(w, http.StatusBadRequest, "origin query parameter is required")
		return
	}

	ownerID := ownerIDFromCtx(r.Context())
	if !ro.artifactExists(w, r, artifactID, "delete origin decision") {
		return
	}
	// Deliberately not normalized here: the store normalizes what it can and
	// passes the rest through verbatim, which is the only way a row written
	// before av-i7hd's validation existed can be deleted at all.
	if err := ro.cfg.Store.DeleteOriginDecision(r.Context(), ownerID, artifactID, q.Get("origin")); err != nil {
		writeArtifactError(w, r, "delete origin decision", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listOriginDecisions returns every decision for an artifact, allow and block
// alike — the read path behind any client that must tell the three origin
// states (allowed, blocked, undecided) apart. The edit page gets the same three
// server-rendered, so this exists for API clients rather than for that page.
func (ro *Router) listOriginDecisions(w http.ResponseWriter, r *http.Request) {
	artifactID := urlParamID(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	if !ro.artifactExists(w, r, artifactID, "list origin decisions") {
		return
	}
	decisions, err := ro.cfg.Store.ListOriginDecisions(r.Context(), ownerID, artifactID)
	if err != nil {
		serverError(w, r, "list origin decisions", err)
		return
	}
	writeJSON(w, http.StatusOK, decisions)
}
