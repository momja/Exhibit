package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
)

type createShareRequest struct {
	ArtifactID string     `json:"artifact_id"`
	Public     bool       `json:"public"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type createShareResponse struct {
	Share    *store.Share `json:"share"`
	ShareURL string       `json:"share_url"`
}

func (ro *Router) createShare(w http.ResponseWriter, r *http.Request) {
	var req createShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ArtifactID == "" {
		http.Error(w, "artifact_id is required", http.StatusBadRequest)
		return
	}

	// Verify artifact exists
	a, err := ro.cfg.Store.GetArtifact(r.Context(), req.ArtifactID)
	if err != nil {
		serverError(w, r, "create share artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	sh := &store.Share{
		ID:         uuid.New().String(),
		ArtifactID: req.ArtifactID,
		Public:     req.Public,
		ExpiresAt:  req.ExpiresAt,
	}

	if err := ro.cfg.Store.CreateShare(r.Context(), sh); err != nil {
		serverError(w, r, "create share", err)
		return
	}

	slog.DebugContext(r.Context(), "share created",
		slog.String("share_id", sh.ID), slog.String("artifact_id", req.ArtifactID), slog.Bool("public", req.Public))

	resp := createShareResponse{
		Share:    sh,
		ShareURL: ro.cfg.RenderOrigin + "/s/" + sh.ID,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (ro *Router) deleteShare(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "shareID")

	sh, err := ro.cfg.Store.GetShare(r.Context(), id)
	if err != nil {
		serverError(w, r, "delete share lookup", err)
		return
	}
	if sh == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.DeleteShare(r.Context(), id); err != nil {
		serverError(w, r, "delete share", err)
		return
	}

	slog.DebugContext(r.Context(), "share deleted", slog.String("share_id", id))
	w.WriteHeader(http.StatusNoContent)
}
