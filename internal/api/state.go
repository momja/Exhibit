package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (ro *Router) getState(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifactID")

	// Verify artifact exists
	a, err := ro.cfg.Store.GetArtifact(r.Context(), artifactID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	state, err := ro.cfg.Store.GetState(r.Context(), artifactID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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

	// Verify artifact exists
	a, err := ro.cfg.Store.GetArtifact(r.Context(), artifactID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.SetState(r.Context(), artifactID, req.Key, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
