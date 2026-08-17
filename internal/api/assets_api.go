package api

// The out-of-line asset endpoints (av-20fk): what an artifact carries beside
// its body, and the one control that removes a piece of it.
//
// Deliberately read-and-delete. Assets are produced by the snapshot vendorer at
// ingest and by nothing else, so there is no create or update here — a client
// that could POST bytes into an artifact's asset set would be a second way to
// put arbitrary content behind that artifact's render URL.

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/store"
)

// assetView is one asset as the edit page and the agent see it: everything
// needed to decide about it, and none of its bytes.
//
// source_url leads because it is the field a person actually uses. Deleting an
// asset that is still in use breaks the artifact at render, and matching this
// URL against their own code is how the owner knows which is which.
type assetView struct {
	ID          string `json:"id"`
	SourceURL   string `json:"source_url"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Generation  string `json:"generation_id"`
}

type assetListResponse struct {
	Assets     []assetView `json:"assets"`
	TotalBytes int64       `json:"total_bytes"`
}

func toAssetViews(assets []store.ArtifactAsset) assetListResponse {
	out := assetListResponse{Assets: make([]assetView, 0, len(assets))}
	for _, a := range assets {
		out.Assets = append(out.Assets, assetView{
			ID: a.ID, SourceURL: a.SourceURL, ContentType: a.ContentType,
			SizeBytes: a.SizeBytes, Generation: a.GenerationID,
		})
		out.TotalBytes += a.SizeBytes
	}
	return out
}

// listAssets returns an artifact's assets. The edit page's panel fetches this
// on first open rather than with the page: asset metadata is cold data nothing
// else on that page needs.
func (ro *Router) listAssets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	assets, err := ro.cfg.Store.ListArtifactAssets(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "list artifact assets", err)
		return
	}
	writeJSON(w, http.StatusOK, toAssetViews(assets))
}

// deleteAsset removes one asset at the owner's request.
//
// This is the only deletion path in the asset lifecycle that is not decided by
// a rule, and it exists precisely because one case cannot be: the owner removed
// the code that used a payload, and nothing the server can inspect will tell it
// so. A body scan would not settle it either — the render manifest matches
// resolved URLs at call time, so an asset whose original literal is gone may
// still be fetched by a rewritten body.
func (ro *Router) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	assetID := chi.URLParam(r, "assetID")
	ownerID := ownerIDFromCtx(r.Context())

	queued, err := ro.cfg.Store.DeleteArtifactAsset(r.Context(), ownerID, id, assetID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, r, "delete artifact asset", err)
		return
	}
	// Same pattern as every other delete on this surface: the request drains
	// what it just enqueued, so no request ever walks a backlog and a crash
	// leaves the work for the next startup rather than losing it.
	ro.reclaimBlobs(r.Context(), queued)
	w.WriteHeader(http.StatusNoContent)
}
