package api

import (
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
// can supply different store filters without teaching paging or the browser
// about collections, tags, or any other selection rule. Paths are app-relative
// paths without queries; the owner is always resolved from the request.
type galleryDefinition struct {
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

type galleryEmptyState struct {
	Message     string
	ActionURL   string
	ActionLabel string
}

type galleryGridData struct {
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
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func (ro *Router) loadGalleryGrid(r *http.Request, gallery galleryDefinition, offset int) (galleryGridData, error) {
	grid := galleryGridData{
		ID: gallery.ID, FragmentPath: gallery.FragmentPath,
		Query: strings.TrimSpace(r.URL.Query().Get("q")), Offset: offset,
		Empty: gallery.Empty,
	}
	if grid.Empty.Message == "" {
		grid.Empty.Message = "No artifacts in this gallery."
	}
	options := gallery.Filters
	options.OwnerID = ownerIDFromCtx(r.Context())
	options.Query = grid.Query
	options.Offset = offset
	// Look ahead one record so exact multiples need no empty trailing request.
	options.Limit = galleryPageSize + 1
	artifacts, err := ro.cfg.Store.ListArtifacts(r.Context(), options)
	if err != nil {
		return grid, err
	}
	if len(artifacts) > galleryPageSize {
		artifacts = artifacts[:galleryPageSize]
		grid.NextURL = galleryURL(gallery.FragmentPath, grid.Query, offset+galleryPageSize)
	}
	urls := ro.renderURLs(r)
	for _, a := range artifacts {
		grid.Cards = append(grid.Cards, galleryCard{
			ArtifactID: a.ID, Title: a.Title, Tags: tagViews(a.Tags),
			Capability: newCardCapabilityView(a), Widget: newGalleryWidgetView(a, urls),
		})
	}
	return grid, nil
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
	status := http.StatusOK
	if err != nil {
		slog.ErrorContext(r.Context(), "gallery grid lookup", slog.Any("error", err))
		grid.RetryURL = galleryURL(gallery.FragmentPath, grid.Query, offset)
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
		w.Header().Set("HX-Replace-Url", galleryURL(gallery.PagePath, grid.Query, 0))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	fmt.Fprint(w, fragment)
}
