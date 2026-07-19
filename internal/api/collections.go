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

func (ro *Router) listCollections(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	collections, err := ro.cfg.Store.ListCollections(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "list collections", err)
		return
	}
	if collections == nil {
		collections = []*store.Collection{}
	}
	writeJSON(w, http.StatusOK, collections)
}

type createCollectionRequest struct {
	Name string `json:"name"`
}

func (ro *Router) createCollection(w http.ResponseWriter, r *http.Request) {
	var req createCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ownerID := ownerIDFromCtx(r.Context())
	c := &store.Collection{
		ID:      uuid.New().String(),
		OwnerID: ownerID,
		Name:    req.Name,
	}

	if err := ro.cfg.Store.CreateCollection(r.Context(), c); err != nil {
		serverError(w, r, "create collection", err)
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

func (ro *Router) addArtifactToCollection(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionID")
	artifactID := chi.URLParam(r, "artifactID")

	if err := ro.cfg.Store.AddArtifactToCollection(r.Context(), artifactID, collectionID); err != nil {
		serverError(w, r, "add artifact to collection", err)
		return
	}

	slog.DebugContext(r.Context(), "artifact added to collection",
		slog.String("artifact_id", artifactID), slog.String("collection_id", collectionID))
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) removeArtifactFromCollection(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionID")
	artifactID := chi.URLParam(r, "artifactID")

	if err := ro.cfg.Store.RemoveArtifactFromCollection(r.Context(), artifactID, collectionID); err != nil {
		serverError(w, r, "remove artifact from collection", err)
		return
	}

	slog.DebugContext(r.Context(), "artifact removed from collection",
		slog.String("artifact_id", artifactID), slog.String("collection_id", collectionID))
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) listTags(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	tags, err := ro.cfg.Store.ListTags(r.Context(), ownerID)
	if err != nil {
		serverError(w, r, "list tags", err)
		return
	}
	if tags == nil {
		tags = []*store.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

// writeTagError maps store errors from tag mutations to HTTP responses.
func writeTagError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrDuplicateName):
		writeError(w, http.StatusConflict, "a tag with that name already exists")
	default:
		slog.ErrorContext(r.Context(), "tag mutation failed",
			slog.String("err", err.Error()),
			slog.String("method", r.Method), slog.String("path", r.URL.Path))
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

type createTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (ro *Router) createTag(w http.ResponseWriter, r *http.Request) {
	var req createTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	color := req.Color
	if color == "" {
		color = store.DefaultTagColor
	}

	ownerID := ownerIDFromCtx(r.Context())
	t := &store.Tag{
		ID:      uuid.New().String(),
		OwnerID: ownerID,
		Name:    req.Name,
		Color:   color,
	}

	if err := ro.cfg.Store.CreateTag(r.Context(), t); err != nil {
		writeTagError(w, r, err)
		return
	}

	slog.DebugContext(r.Context(), "tag created", slog.String("tag_id", t.ID), slog.String("name", t.Name))
	writeJSON(w, http.StatusCreated, t)
}

// updateTagRequest uses pointers so an omitted field is distinguishable from
// an explicit value; omitted fields are left unchanged.
type updateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (ro *Router) updateTag(w http.ResponseWriter, r *http.Request) {
	var req updateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name != nil && *req.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}

	ownerID := ownerIDFromCtx(r.Context())
	t, err := ro.cfg.Store.UpdateTag(r.Context(), ownerID, chi.URLParam(r, "tagID"), req.Name, req.Color)
	if err != nil {
		writeTagError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

func (ro *Router) deleteTag(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	if err := ro.cfg.Store.DeleteTag(r.Context(), ownerID, chi.URLParam(r, "tagID")); err != nil {
		writeTagError(w, r, err)
		return
	}

	slog.DebugContext(r.Context(), "tag deleted", slog.String("tag_id", chi.URLParam(r, "tagID")))
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) addArtifactTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	artifactID := chi.URLParam(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	if err := ro.cfg.Store.AddArtifactTag(r.Context(), ownerID, artifactID, tagID); err != nil {
		writeTagError(w, r, err)
		return
	}

	slog.DebugContext(r.Context(), "tag added to artifact",
		slog.String("artifact_id", artifactID), slog.String("tag_id", tagID))
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) removeArtifactTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	artifactID := chi.URLParam(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())

	if err := ro.cfg.Store.RemoveArtifactTag(r.Context(), ownerID, artifactID, tagID); err != nil {
		writeTagError(w, r, err)
		return
	}

	slog.DebugContext(r.Context(), "tag removed from artifact",
		slog.String("artifact_id", artifactID), slog.String("tag_id", tagID))
	w.WriteHeader(http.StatusNoContent)
}
