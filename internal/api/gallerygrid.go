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
	Empty: galleryEmptyState{Message: "No artifacts yet.", ActionURL: "/new", ActionLabel: "Add your first artifact"},
}

// Selection returns cards and validated URL parameters even when listing fails,
// so retry links keep the same selection. limit includes the shared lookahead.
type gallerySelection func(*Router, *http.Request, int, int, string) ([]galleryCard, url.Values, error)

type galleryEmptyState struct {
	Message     string
	ActionURL   string
	ActionLabel string
}

type galleryGridData struct {
	Title        string
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

func galleryURL(path, query string, offset int) string {
	return (galleryGridData{Query: query}).url(path, offset)
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
	var cards []galleryCard
	var err error
	if gallery.Select != nil {
		cards, grid.Parameters, err = gallery.Select(ro, r, offset, galleryPageSize+1, grid.Query)
	} else {
		cards, err = ro.selectOwnedGallery(r, gallery.Filters, offset, galleryPageSize+1, grid.Query)
	}
	if err != nil {
		return grid, err
	}
	if len(cards) > galleryPageSize {
		cards = cards[:galleryPageSize]
		grid.NextURL = grid.url(gallery.FragmentPath, offset+galleryPageSize)
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
	if errors.Is(err, errGalleryOwner) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	status := http.StatusOK
	if err != nil {
		slog.ErrorContext(r.Context(), "gallery grid lookup", slog.Any("error", err))
		grid.RetryURL = grid.url(gallery.FragmentPath, offset)
		status = http.StatusInternalServerError
	}
	fragment, err := renderPage("galleryGrid", grid)
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
