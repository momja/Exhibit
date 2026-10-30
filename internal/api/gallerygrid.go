package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

const galleryPageSize = 24

// A gallery owns its routes and its artifact selection. Other gallery pages
// supply their own selection without teaching paging or the browser about
// ownership or other selection rules. Filters is the default owner selection.
type galleryDefinition struct {
	ReadOnly     bool
	Title        string
	Select       gallerySelection
	ID           string
	PagePath     string
	FragmentPath string
	Filters      store.ListOptions
	Empty        galleryEmptyState
}

var libraryGallery = galleryDefinition{
	ID: "library-gallery", PagePath: "/", FragmentPath: "/partials/gallery-grid",
	Empty: galleryEmptyState{Welcome: true, Message: "Welcome to Exhibit", ActionURL: "/new", ActionLabel: "Create your first artifact"},
}

// Selection returns cards and validated URL parameters even when listing fails,
// so retry links keep the same selection. limit includes the shared lookahead.
type gallerySelection func(*Router, *http.Request, int, int, string) ([]galleryCard, url.Values, error)

type galleryEmptyState struct {
	Welcome     bool
	Message     string
	ActionURL   string
	ActionLabel string
}

type galleryGridData struct {
	Title        string
	Filters      galleryFilters
	Parameters   url.Values
	ID           string
	FragmentPath string
	Query        string
	Offset       int
	Cards        []galleryCard
	NextURL      string
	RetryURL     string
	Empty        galleryEmptyState
}

func (ro *Router) loadGalleryGrid(r *http.Request, gallery galleryDefinition, offset int) (galleryGridData, error) {
	grid := galleryGridData{
		Title: gallery.Title, ID: gallery.ID, FragmentPath: gallery.FragmentPath,
		Query: strings.TrimSpace(r.URL.Query().Get("q")), Offset: offset,
		Empty: gallery.Empty,
	}
	if grid.Empty.Message == "" {
		grid.Empty.Message = "No artifacts in this gallery."
	}
	// Preserve filter parameters even if choices cannot be loaded for a retry.
	grid.Parameters = url.Values{}
	filterNames := []string{"tag", "collection"}
	if gallery.ReadOnly {
		filterNames = []string{"owner"}
	}
	for _, name := range filterNames {
		if values := r.URL.Query()[name]; len(values) > 0 {
			grid.Parameters[name] = append([]string(nil), values...)
		}
	}
	if err := ro.loadGalleryFilters(r, gallery, &grid); err != nil {
		return grid, err
	}
	var cards []galleryCard
	var err error
	limit := galleryPageSize
	if r.URL.Path == gallery.PagePath {
		if visible, parseErr := strconv.Atoi(r.URL.Query().Get("visible")); parseErr == nil && visible > limit {
			limit = min(visible, 480)
		}
	}
	if gallery.Select != nil {
		cards, grid.Parameters, err = gallery.Select(ro, r, offset, limit+1, grid.Query)
	} else {
		options := gallery.Filters
		options.Tags = append(append([]string(nil), options.Tags...), grid.Parameters["tag"]...)
		options.CollectionIDs = append(append([]string(nil), options.CollectionIDs...), grid.Parameters["collection"]...)
		options.MatchAllFilters = true
		cards, err = ro.selectOwnedGallery(r, options, offset, limit+1, grid.Query)
	}
	grid.prepareFilterLinks(gallery)
	if err != nil {
		return grid, err
	}
	if len(cards) > limit {
		cards = cards[:limit]
		grid.NextURL = grid.url(gallery.FragmentPath, offset+limit)
	}
	for i := range cards {
		for j := range cards[i].Actions.Items {
			item := &cards[i].Actions.Items[j]
			if item.Href == "/artifacts/"+cards[i].ArtifactID+"/edit" {
				item.Href = editURL(cards[i].ArtifactID, grid.url(gallery.PagePath, 0))
			}
		}
	}
	grid.Cards = cards
	return grid, nil
}

func (ro *Router) selectOwnedGallery(r *http.Request, options store.ListOptions, offset, limit int, query string) ([]galleryCard, error) {
	options.OwnerID = ownerIDFromCtx(r.Context())
	options.Query, options.Offset, options.Limit = query, offset, limit
	artifacts, err := ro.cfg.Store.ListArtifacts(r.Context(), options)
	if err != nil {
		return nil, err
	}
	var cards []galleryCard
	urls := ro.renderURLs(r)
	for _, a := range artifacts {
		cards = append(cards, galleryCard{ArtifactID: a.ID, Title: a.Title, Tags: tagViews(a.Tags), Capability: newCardCapabilityView(a), Widget: newGalleryWidgetView(a, urls), Actions: galleryCardActions(a.ID, a.Title)})
	}
	return cards, nil
}

func (grid galleryGridData) url(path string, offset int) string {
	values := url.Values{}
	for name, entries := range grid.Parameters {
		values[name] = append([]string(nil), entries...)
	}
	if grid.Query != "" {
		values.Set("q", grid.Query)
	}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

// galleryGridPartial shares both the selection and the template with the full
// page. A new gallery's fragment handler calls serveGalleryGrid with its own
// definition, keeping next-page URLs inside that gallery.
func (ro *Router) galleryGridPartial(w http.ResponseWriter, r *http.Request) {
	ro.serveGalleryGrid(w, r, libraryGallery)
}

func (ro *Router) serveGalleryGrid(w http.ResponseWriter, r *http.Request, gallery galleryDefinition) {
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var err error
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > int(^uint(0)>>1)-galleryPageSize {
			http.Error(w, "Invalid gallery offset", http.StatusBadRequest)
			return
		}
	}
	grid, err := ro.loadGalleryGrid(r, gallery, offset)
	if errors.Is(err, errGalleryOwner) || errors.Is(err, errGalleryFilter) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	status := http.StatusOK
	if err != nil {
		slog.ErrorContext(r.Context(), "gallery grid lookup", slog.Any("error", err))
		grid.RetryURL = grid.url(gallery.FragmentPath, offset)
		status = http.StatusInternalServerError
	}
	partial := "galleryGrid"
	if offset == 0 && r.Header.Get("HX-Request") != "" && status == http.StatusOK {
		partial = "galleryFilterUpdate"
	}
	fragment, err := renderPage(partial, grid)
	if err != nil {
		fragmentServerError(w, r, "gallery grid render", err)
		return
	}
	// Only a successful search changes the page URL. Paging keeps the URL of
	// the gallery, and its query, rather than exposing a fragment endpoint.
	if status == http.StatusOK && offset == 0 {
		w.Header().Set("HX-Replace-Url", grid.url(gallery.PagePath, 0))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprint(w, fragment)
}
