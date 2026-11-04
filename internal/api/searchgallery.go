package api

import (
	"net/http"
	"net/url"
	"slices"

	"github.com/momja/Exhibit/internal/store"
)

var searchGallery = galleryDefinition{
	ID: "search-gallery", Title: "Search", PagePath: "/search", FragmentPath: "/partials/gallery-grid/search",
	Select: selectSearchResults, SearchPage: true,
	Description: "All your artifacts, including those shared with you.",
	Empty:       galleryEmptyState{Welcome: true, Message: "Welcome to Exhibit", ActionURL: "/new", ActionLabel: "Create your first artifact"},
}

func selectSearchResults(ro *Router, r *http.Request, offset, limit int, query string) ([]galleryCard, url.Values, error) {
	params := url.Values{}
	for _, name := range []string{"tag", "collection"} {
		for _, value := range r.URL.Query()[name] {
			if !slices.Contains(params[name], value) {
				params.Add(name, value)
			}
		}
	}
	viewer := ownerIDFromCtx(r.Context())
	artifacts, err := ro.cfg.Store.ListViewerGallery(r.Context(), store.ViewerID(viewer), store.ViewerGalleryOptions{Query: query, Tags: params["tag"], CollectionIDs: params["collection"], Limit: limit, Offset: offset})
	if err != nil {
		return nil, params, err
	}
	return readableGalleryCards(artifacts, viewer, ro.renderURLs(r)), params, nil
}
