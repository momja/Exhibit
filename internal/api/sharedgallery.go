package api

import (
	"net/http"
	"net/url"
	"unicode"
	"unicode/utf8"

	"github.com/momja/Exhibit/internal/store"
)

var sharedGallery = galleryDefinition{
	ID: "shared-gallery", Title: "Shared", PagePath: "/shared", FragmentPath: "/partials/gallery-grid/shared",
	Description: "Artifacts shared with you and artifacts you’ve shared with others.",
	Select:      selectAllShared, NoFilters: true,
	Empty: galleryEmptyState{Message: "Shared artifacts will appear here. Share one of yours, or ask someone to share with you."},
}

func selectAllShared(ro *Router, r *http.Request, offset, limit int, query string) ([]galleryCard, url.Values, error) {
	viewer := ownerIDFromCtx(r.Context())
	artifacts, err := ro.cfg.Store.ListViewerGallery(r.Context(), store.ViewerID(viewer), store.ViewerGalleryOptions{Query: query, SharedOnly: true, Limit: limit, Offset: offset})
	return readableGalleryCards(artifacts, viewer, ro.renderURLs(r)), url.Values{}, err
}

func sharedGalleryCard(a *store.SharedArtifact, urls renderURLs) galleryCard {
	initial, _ := utf8.DecodeRuneInString(a.OwnerName)
	widget := newGalleryWidgetView(a.Artifact, urls.ownedBy(a.OwnerID))
	if widget.ReloadURL != "" {
		widget.ReloadURL = "/artifacts/" + a.ID + "/widget/viewer/open"
	}
	return galleryCard{ReadOnly: true, ArtifactID: a.ID, Title: a.Title, OwnerName: a.OwnerName, OwnerInitial: string(unicode.ToUpper(initial)), Widget: widget}
}
