package api

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/momja/Exhibit/internal/store"
)

var starredGallery = galleryDefinition{
	ID: "starred-gallery", Title: "Starred", PagePath: "/", FragmentPath: "/partials/gallery-grid",
	NoFilters: true, HideSearch: true, Select: selectStarred,
	Description: "Your go-to artifacts, most recently opened first.",
	Empty:       galleryEmptyState{Message: "Keep your favorites close. Choose Star in an artifact’s action menu.", ActionURL: "/search", ActionLabel: "Browse your artifacts"},
}

func selectStarred(ro *Router, r *http.Request, offset, limit int, _ string) ([]galleryCard, url.Values, error) {
	viewer := ownerIDFromCtx(r.Context())
	artifacts, err := ro.cfg.Store.ListViewerGallery(r.Context(), store.ViewerID(viewer), store.ViewerGalleryOptions{StarredOnly: true, Limit: limit, Offset: offset})
	return readableGalleryCards(artifacts, viewer, ro.renderURLs(r)), url.Values{}, err
}

func readableGalleryCards(artifacts []*store.SharedArtifact, viewer int64, urls renderURLs) []galleryCard {
	cards := make([]galleryCard, 0, len(artifacts))
	for _, a := range artifacts {
		if a.OwnerID == viewer {
			cards = append(cards, ownedGalleryCard(a.Artifact, urls))
		} else {
			cards = append(cards, sharedGalleryCard(a, urls))
		}
	}
	return cards
}

func (ro *Router) starArtifact(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Starred *bool `json:"starred"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Starred == nil {
		http.Error(w, "starred must be a boolean", http.StatusBadRequest)
		return
	}
	ro.writePreference(w, r, func(viewer store.ViewerID, id string) error {
		return ro.cfg.Store.SetArtifactStarred(r.Context(), viewer, id, *body.Starred)
	})
}

func (ro *Router) artifactOpened(w http.ResponseWriter, r *http.Request) {
	ro.writePreference(w, r, func(viewer store.ViewerID, id string) error {
		return ro.cfg.Store.RecordArtifactOpened(r.Context(), viewer, id)
	})
}

func (ro *Router) writePreference(w http.ResponseWriter, r *http.Request, write func(store.ViewerID, string) error) {
	err := write(store.ViewerID(ownerIDFromCtx(r.Context())), urlParamID(r, "artifactID"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, "artifact preference", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
