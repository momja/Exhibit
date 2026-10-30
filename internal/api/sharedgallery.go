package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/momja/Exhibit/internal/store"
)

var errGalleryOwner = errors.New("invalid gallery owner")

var sharedGallery = galleryDefinition{
	ID: "shared-gallery", Title: "Shared with me", PagePath: "/shared", FragmentPath: "/partials/gallery-grid/shared",
	Select: selectSharedWith, ReadOnly: true,
	Empty: galleryEmptyState{Message: "Nothing has been shared with you yet."},
}

var sharedByGallery = galleryDefinition{
	ID: "shared-by-me-gallery", Title: "Shared by me", PagePath: "/shared-by-me", FragmentPath: "/partials/gallery-grid/shared-by-me",
	Filters: store.ListOptions{SharedOnly: true},
	Empty:   galleryEmptyState{Message: "Nothing shared yet. Share an artifact from its share panel."},
}

func selectSharedWith(ro *Router, r *http.Request, offset, limit int, query string) ([]galleryCard, url.Values, error) {
	params := url.Values{}
	// Preserve requested filters on a transient owner-list failure too.
	for _, raw := range r.URL.Query()["owner"] {
		params.Add("owner", raw)
	}
	viewer := store.ViewerID(ownerIDFromCtx(r.Context()))
	owners, err := ro.cfg.Store.SharedWithOwners(r.Context(), viewer)
	if err != nil {
		return nil, params, err
	}
	allowed := map[int64]bool{}
	for _, owner := range owners {
		allowed[owner.ID] = true
	}
	var ids []int64
	params = url.Values{}
	seen := map[int64]bool{}
	for _, raw := range r.URL.Query()["owner"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || !allowed[id] {
			return nil, params, errGalleryOwner
		}
		if !seen[id] {
			ids = append(ids, id)
			params.Add("owner", strconv.FormatInt(id, 10))
			seen[id] = true
		}
	}
	artifacts, err := ro.cfg.Store.ListArtifactsSharedWith(r.Context(), viewer, store.SharedListOptions{OwnerIDs: ids, Query: query, Offset: offset, Limit: limit})
	if err != nil {
		return nil, params, err
	}
	var cards []galleryCard
	for _, a := range artifacts {
		initial, _ := utf8.DecodeRuneInString(a.OwnerName)
		widget := newGalleryWidgetView(a.Artifact, ro.renderURLs(r).ownedBy(a.OwnerID))
		if widget.ReloadURL != "" {
			widget.ReloadURL = "/artifacts/" + a.ID + "/widget/viewer/open"
		}
		cards = append(cards, galleryCard{ReadOnly: true, ArtifactID: a.ID, Title: a.Title, OwnerName: a.OwnerName, OwnerInitial: string(unicode.ToUpper(initial)), Widget: widget})
	}
	return cards, params, nil
}
