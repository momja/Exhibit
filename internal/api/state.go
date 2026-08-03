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

// Key is a pointer so an omitted key (nil — a malformed request) stays
// distinct from an empty one (""), which is a legitimate Web Storage key:
// localStorage.setItem("", v) is valid and must round-trip like any other.
// Testing the string for "" would conflate the two, which is the same mistake
// the delete route avoids by keying on query-parameter presence (av-hh1o).
type setStateRequest struct {
	Key   *string `json:"key"`
	Value string  `json:"value"`
}

func (ro *Router) setState(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	var req setStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Key == nil {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}
	key := *req.Key

	if !ro.artifactExists(w, r, artifactID, "set state") {
		return
	}

	if err := ro.cfg.Store.SetState(r.Context(), artifactID, key, req.Value); err != nil {
		serverError(w, r, "set state", err)
		return
	}

	slog.DebugContext(r.Context(), "state written",
		slog.String("artifact_id", artifactID),
		slog.String("key", key),
		slog.Int("value_bytes", len(req.Value)),
	)

	w.WriteHeader(http.StatusNoContent)
}

// deleteState removes one state row, or every row for the artifact.
//
// The key travels as the `key` QUERY parameter, deliberately not as a path
// segment (av-hh1o). State keys are arbitrary artifact-chosen text, and a key
// of ".." in a path is resolved by the browser's URL parser *before the
// request is sent* — turning DELETE /api/artifacts/:id/state/.. into
// DELETE /api/artifacts/:id/, which deletes the artifact. That made untrusted
// artifact code able to destroy the artifact through the host frame's token
// by calling localStorage.removeItem(".."). A query value has no segment
// structure, so there is nothing to normalize.
//
// Two more things fall out of the same choice: the empty-string key becomes
// representable (there is no empty path segment, but there is an empty query
// value), and the key stops being bounded by the request line, which a long
// key could otherwise overflow on delete despite being settable via the PUT
// body.
//
// Query().Has("key") is what separates the two operations — present-but-empty
// deletes the empty-string key, absent erases everything. Testing the value
// for "" would conflate them.
func (ro *Router) deleteState(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	if !ro.artifactExists(w, r, artifactID, "delete state") {
		return
	}

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "invalid query encoding", http.StatusBadRequest)
		return
	}

	// Absent key: erase everything. Destructive and irreversible — there is no
	// version history for state — but bounded: it touches nothing else the
	// artifact owns (body, origin decisions, capability approvals all survive).
	if !query.Has("key") {
		if err := ro.cfg.Store.ClearState(r.Context(), artifactID); err != nil {
			serverError(w, r, "clear state", err)
			return
		}
		slog.InfoContext(r.Context(), "state cleared", slog.String("artifact_id", artifactID))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := query.Get("key")
	if err := ro.cfg.Store.DeleteState(r.Context(), artifactID, key); err != nil {
		serverError(w, r, "delete state", err)
		return
	}

	slog.InfoContext(r.Context(), "state key deleted",
		slog.String("artifact_id", artifactID), slog.String("key", key))
	w.WriteHeader(http.StatusNoContent)
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
