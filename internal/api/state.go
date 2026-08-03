package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

func (ro *Router) getState(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	if !ro.artifactExists(w, r, artifactID, "get state") {
		return
	}

	state, err := ro.cfg.Store.GetState(r.Context(), artifactID)
	if err != nil {
		serverError(w, r, "get state", err)
		return
	}
	slog.DebugContext(r.Context(), "state read",
		slog.String("artifact_id", artifactID), slog.Int("keys", len(state)))
	writeJSON(w, http.StatusOK, state)
}

type setStateRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (ro *Router) setState(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	var req setStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	if !ro.artifactExists(w, r, artifactID, "set state") {
		return
	}

	if err := ro.cfg.Store.SetState(r.Context(), artifactID, req.Key, req.Value); err != nil {
		serverError(w, r, "set state", err)
		return
	}

	slog.DebugContext(r.Context(), "state written",
		slog.String("artifact_id", artifactID),
		slog.String("key", req.Key),
		slog.Int("value_bytes", len(req.Value)),
	)

	w.WriteHeader(http.StatusNoContent)
}

// deleteStateKey drops one state row. Unlike the shim's removeItem — which
// today write-throughs an empty string and so leaves a tombstone the artifact
// reads back as "" (av-ms3r) — this genuinely removes the row, so the key is
// absent on the next render.
func (ro *Router) deleteStateKey(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	key, err := pathParam(r, "key")
	if err != nil {
		http.Error(w, "invalid key encoding", http.StatusBadRequest)
		return
	}

	if !ro.artifactExists(w, r, artifactID, "delete state") {
		return
	}

	if err := ro.cfg.Store.DeleteState(r.Context(), artifactID, key); err != nil {
		serverError(w, r, "delete state", err)
		return
	}

	slog.InfoContext(r.Context(), "state key deleted",
		slog.String("artifact_id", artifactID), slog.String("key", key))
	w.WriteHeader(http.StatusNoContent)
}

// clearState erases every state row for one artifact. Destructive and
// irreversible — there is no version history for state — but bounded: it
// touches nothing else the artifact owns (body, origin decisions, capability
// approvals all survive).
func (ro *Router) clearState(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	if !ro.artifactExists(w, r, artifactID, "clear state") {
		return
	}

	if err := ro.cfg.Store.ClearState(r.Context(), artifactID); err != nil {
		serverError(w, r, "clear state", err)
		return
	}

	slog.InfoContext(r.Context(), "state cleared", slog.String("artifact_id", artifactID))
	w.WriteHeader(http.StatusNoContent)
}

// pathParam returns a URL parameter as the text the client meant, which for a
// state key can be anything an artifact chose to call it — slashes, spaces,
// percent signs included.
//
// It exists because chi hands the param over in whichever form it routed on:
// the raw (still-encoded) path when the URL carries one, the decoded path
// otherwise. Unescaping unconditionally would corrupt the second case — a key
// like "100% done" arrives already decoded, and its literal '%' is not the
// start of an escape — so this mirrors chi's own choice instead of guessing.
func pathParam(r *http.Request, name string) (string, error) {
	v := chi.URLParam(r, name)
	if r.URL.RawPath == "" {
		return v, nil
	}
	return url.PathUnescape(v)
}

// artifactExists reports whether the artifact is there, having already written
// the 404/500 response when it is not. State rows outlive nothing: without this
// check a delete against an unknown id would silently succeed, since removing
// rows that don't exist is a no-op.
func (ro *Router) artifactExists(w http.ResponseWriter, r *http.Request, artifactID, op string) bool {
	a, err := ro.cfg.Store.GetArtifact(r.Context(), artifactID)
	if err != nil {
		serverError(w, r, op+" artifact lookup", err)
		return false
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return false
	}
	return true
}
