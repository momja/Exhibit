package api

import (
	"encoding/json"
	"net/http"

	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (ro *Router) listCollections(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	collections, err := ro.cfg.Store.ListCollections(r.Context(), ownerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, c)
}

func (ro *Router) addArtifactToCollection(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionID")
	artifactID := chi.URLParam(r, "artifactID")

	if err := ro.cfg.Store.AddArtifactToCollection(r.Context(), artifactID, collectionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) removeArtifactFromCollection(w http.ResponseWriter, r *http.Request) {
	collectionID := chi.URLParam(r, "collectionID")
	artifactID := chi.URLParam(r, "artifactID")

	if err := ro.cfg.Store.RemoveArtifactFromCollection(r.Context(), artifactID, collectionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) listTags(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	tags, err := ro.cfg.Store.ListTags(r.Context(), ownerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []*store.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

type createTagRequest struct {
	Name string `json:"name"`
}

func (ro *Router) createTag(w http.ResponseWriter, r *http.Request) {
	var req createTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ownerID := ownerIDFromCtx(r.Context())
	t := &store.Tag{
		ID:      uuid.New().String(),
		OwnerID: ownerID,
		Name:    req.Name,
	}

	if err := ro.cfg.Store.CreateTag(r.Context(), t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

func (ro *Router) addArtifactTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	artifactID := chi.URLParam(r, "artifactID")

	if err := ro.cfg.Store.AddArtifactTag(r.Context(), artifactID, tagID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) removeArtifactTag(w http.ResponseWriter, r *http.Request) {
	tagID := chi.URLParam(r, "tagID")
	artifactID := chi.URLParam(r, "artifactID")

	if err := ro.cfg.Store.RemoveArtifactTag(r.Context(), artifactID, tagID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
