package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/store"
)

type createShareRequest struct {
	ArtifactID string `json:"artifact_id"`

	// Public is a tombstone on exactly ExpiresAt's grounds, and it is what
	// closes av-20xv. The column was accepted, stored, and never read by
	// ServeShare, so `public: false` was precisely the value a caller would
	// set believing they had restricted something — an access control the API
	// advertised and did not have. av-lrae dropped it rather than wiring it
	// up, because once a recipient is on the row it means exactly "no
	// recipient": a share with nobody named on it *is* the public link.
	// Continuing to accept the key would leave the request shape offering a
	// choice the resource no longer has.
	Public json.RawMessage `json:"public"`

	// ExpiresAt is a tombstone, not a setting. Share expiry was removed in
	// av-8ipt; this field exists only so a request that still asks for one is
	// answered with a 400 instead of quietly getting a share that never
	// expires. Accepting a field the server discards is precisely the defect
	// av-20xv exists to fix, and re-creating it here to save four lines would
	// be a poor trade. Typed as RawMessage because the value is never read —
	// only its presence is.
	ExpiresAt json.RawMessage `json:"expires_at"`
}

type createShareResponse struct {
	Share    *store.Share `json:"share"`
	ShareURL string       `json:"share_url"`
}

func (ro *Router) createShare(w http.ResponseWriter, r *http.Request) {
	if principalFromCtx(r.Context()).ReadOnly {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}
	var req createShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ArtifactID == "" {
		http.Error(w, "artifact_id is required", http.StatusBadRequest)
		return
	}
	// Any mention of either key is an error, whatever its value — one rule with
	// no sub-cases. The first says out loud that a share now lives until it is
	// deleted (DELETE /api/shares/:id is how it ends); the second, that a share
	// with no recipient is already the public one.
	if req.ExpiresAt != nil {
		http.Error(w, "expires_at is no longer supported: shares live until deleted (DELETE /api/shares/:id)", http.StatusBadRequest)
		return
	}
	if req.Public != nil {
		http.Error(w, "public is no longer supported: a share with no recipient is the artifact's public link (av-20xv)", http.StatusBadRequest)
		return
	}

	ownerID := ownerIDFromCtx(r.Context())

	// Verify the artifact exists in this owner's library. Minting a share for
	// someone else's artifact would publish it through the deliberately
	// unauthenticated /s/:id path, so this is the sharpest of the ownership
	// checks — and CreateShare re-asserts it in SQL besides.
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, req.ArtifactID)
	if err != nil {
		serverError(w, r, "create share artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	// No recipient: this route mints the artifact's anonymous link, which is
	// the only share v1 has a caller for. Directing one at an account is
	// av-6xjd's, and it arrives with the thing this route would otherwise have
	// to invent — a way to name a person that is not a raw user id.
	sh := &store.Share{
		ID:         uuid.New().String(),
		ArtifactID: req.ArtifactID,
	}

	if err := ro.cfg.Store.CreateShare(r.Context(), ownerID, sh); err != nil {
		// The artifact already has its one link (av-lrae). That is a schema
		// invariant doing its job, so it is the caller's answer — 409 — rather
		// than a 500 reporting our own constraint as a fault.
		if errors.Is(err, store.ErrDuplicateShare) {
			writeError(w, http.StatusConflict, "this artifact already has a public link")
			return
		}
		writeArtifactError(w, r, "create share", err)
		return
	}

	slog.DebugContext(r.Context(), "share created",
		slog.String("share_id", sh.ID), slog.String("artifact_id", req.ArtifactID))

	resp := createShareResponse{
		Share:    sh,
		ShareURL: ro.cfg.RenderOrigin + "/s/" + sh.ID,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (ro *Router) deleteShare(w http.ResponseWriter, r *http.Request) {
	if principalFromCtx(r.Context()).ReadOnly {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}
	id := chi.URLParam(r, "shareID")

	ownerID := ownerIDFromCtx(r.Context())
	sh, err := ro.cfg.Store.GetShare(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "delete share lookup", err)
		return
	}
	if sh == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if err := ro.cfg.Store.DeleteShare(r.Context(), ownerID, id); err != nil {
		writeArtifactError(w, r, "delete share", err)
		return
	}

	slog.DebugContext(r.Context(), "share deleted", slog.String("share_id", id))
	w.WriteHeader(http.StatusNoContent)
}
