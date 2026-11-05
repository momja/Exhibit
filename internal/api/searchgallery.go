package api

import (
	"net/http"
	"net/url"

	"github.com/momja/Exhibit/internal/store"
)

var searchGallery = galleryDefinition{
	ID: "search-gallery", Title: "Search", PagePath: "/search", FragmentPath: "/partials/gallery-grid/search",
	Select: selectSearchResults, SearchPage: true,
	Empty: galleryEmptyState{Message: "Search your Gallery and artifacts shared with you."},
}

func selectSearchResults(ro *Router, r *http.Request, offset, limit int, query string) ([]galleryCard, url.Values, error) {
	params := url.Values{}
	if query == "" {
		return nil, params, nil
	}
	viewer := ownerIDFromCtx(r.Context())
	artifacts, err := ro.cfg.Store.SearchArtifactsReadableBy(r.Context(), store.ViewerID(viewer), query, limit, offset)
	if err != nil {
		return nil, params, err
	}
	urls := ro.renderURLs(r)
	var cards []galleryCard
	for _, a := range artifacts {
		if a.OwnerID == viewer {
			cards = append(cards, ownedGalleryCard(a.Artifact, urls))
		} else {
			cards = append(cards, sharedGalleryCard(a, urls))
		}
	}
	return cards, params, nil
}
