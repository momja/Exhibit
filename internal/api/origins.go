package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

// Per-origin decision routes (exhibit-fr7). PATCH /api/artifacts/:id carries the
// whole allowlist and is what the edit page's Save uses; these routes decide a
// *single* origin, which is what the runtime permission prompt needs — it learns
// about one blocked origin at a time and must never restate (and so risk
// clobbering) the rest of the artifact's decisions.

type originDecisionRequest struct {
	Origin   string `json:"origin"`
	Decision string `json:"decision"`
	// Source records where the decision came from. It is informational; the
	// handler defaults it rather than trusting an omitted value.
	Source string `json:"source"`
}

// setOriginDecision upserts one origin's decision. decision='allow' widens the
// artifact's CSP on its next render; decision='block' only records "don't ask
// again" so the runtime prompt stops surfacing that origin.
func (ro *Router) setOriginDecision(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	var req originDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !validOrigin(req.Origin) {
		http.Error(w, "origin must be an absolute http(s) origin", http.StatusBadRequest)
		return
	}
	if req.Decision != store.DecisionAllow && req.Decision != store.DecisionBlock {
		http.Error(w, `decision must be "allow" or "block"`, http.StatusBadRequest)
		return
	}
	source := req.Source
	if source == "" {
		source = "user"
	}

	if !ro.artifactExists(w, r, artifactID, "set origin decision") {
		return
	}
	if err := ro.cfg.Store.SetOriginDecision(r.Context(), artifactID, req.Origin, req.Decision, source); err != nil {
		serverError(w, r, "set origin decision", err)
		return
	}
	slog.InfoContext(r.Context(), "origin decision set",
		slog.String("artifact_id", artifactID),
		slog.String("origin", req.Origin),
		slog.String("decision", req.Decision),
		slog.String("source", source))
	writeJSON(w, http.StatusOK, map[string]any{
		"origin": req.Origin, "decision": req.Decision, "source": source,
	})
}

// deleteOriginDecision drops one origin's decision, returning it to undecided.
// This is how a "don't ask again" block is forgotten: without it a block is a
// permanent trap, since a blocked origin never prompts again on its own.
func (ro *Router) deleteOriginDecision(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")
	origin := r.URL.Query().Get("origin")
	if origin == "" {
		http.Error(w, "origin query parameter is required", http.StatusBadRequest)
		return
	}
	if !ro.artifactExists(w, r, artifactID, "delete origin decision") {
		return
	}
	if err := ro.cfg.Store.DeleteOriginDecision(r.Context(), artifactID, origin); err != nil {
		serverError(w, r, "delete origin decision", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listOriginDecisions returns every decision for an artifact, allow and block
// alike — the read path behind any UI that must tell the three origin states
// (allowed / blocked / undecided) apart.
func (ro *Router) listOriginDecisions(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")
	if !ro.artifactExists(w, r, artifactID, "list origin decisions") {
		return
	}
	decisions, err := ro.cfg.Store.ListOriginDecisions(r.Context(), artifactID)
	if err != nil {
		serverError(w, r, "list origin decisions", err)
		return
	}
	writeJSON(w, http.StatusOK, decisions)
}

// artifactExists reports whether the artifact is present, writing the 404/500
// response itself when it is not. label names the caller in the error log.
func (ro *Router) artifactExists(w http.ResponseWriter, r *http.Request, artifactID, label string) bool {
	a, err := ro.cfg.Store.GetArtifact(r.Context(), artifactID)
	if err != nil {
		serverError(w, r, label+" artifact lookup", err)
		return false
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return false
	}
	return true
}

// validOrigin accepts only an absolute http(s) origin with no path — the shape
// a CSP source expression takes. Anything else (a bare hostname, a full URL, a
// scheme we don't emit) would land in the CSP as a token that either does
// nothing or silently widens it in an unintended way.
func validOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}
