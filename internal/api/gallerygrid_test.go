package api

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var galleryCardPattern = regexp.MustCompile(`data-href="/artifacts/([^"]+)"`)
var galleryNextPattern = regexp.MustCompile(`data-gallery-page\s+hx-get="([^"]+)"`)

func galleryCardsAndNext(body string) ([]string, string) {
	var ids []string
	for _, match := range galleryCardPattern.FindAllStringSubmatch(body, -1) {
		ids = append(ids, match[1])
	}
	if match := galleryNextPattern.FindStringSubmatch(body); match != nil {
		return ids, html.UnescapeString(match[1])
	}
	return ids, ""
}

func seedGalleryArtifacts(t *testing.T, ro *Router, count int) {
	t.Helper()
	// All timestamps deliberately tie: pages must use the ID tie-breaker.
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range count {
		require.NoError(t, ro.cfg.Store.PutArtifact(context.Background(), &store.Artifact{
			ID: fmt.Sprintf("gallery-%03d", i), OwnerID: defaultOwnerID,
			Title: fmt.Sprintf("Artifact %03d", i), SourceBlobID: "fixture", Tier: store.Tier1,
			CreatedAt: created, UpdatedAt: created,
		}))
	}
}

func TestGalleryPagesReachEveryArtifactExactlyOnce(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 250)
	path := "/"
	var all []string
	for page := 0; path != ""; page++ {
		require.Less(t, page, 12, "paging did not terminate")
		body := getPage(t, ro, path)
		ids, next := galleryCardsAndNext(body)
		if next != "" {
			assert.Len(t, ids, galleryPageSize)
			assert.Contains(t, body, `hx-trigger="revealed, click"`)
		} else {
			assert.Len(t, ids, 250%galleryPageSize)
		}
		all = append(all, ids...)
		path = next
	}
	require.Len(t, all, 250)
	for i, id := range all {
		assert.Equal(t, fmt.Sprintf("gallery-%03d", 249-i), id)
	}
}

func TestGalleryExactPageHasNoSentinelAndPastEndIsEmpty(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, galleryPageSize)
	ids, next := galleryCardsAndNext(getPage(t, ro, "/"))
	assert.Len(t, ids, galleryPageSize)
	assert.Empty(t, next)
	assert.Empty(t, strings.TrimSpace(getPage(t, ro, "/partials/gallery-grid?offset=24")))
}

func TestGallerySearchFragmentResetsPagingAndEscapesQuery(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 30)
	req := httptest.NewRequest(http.MethodGet, "/partials/gallery-grid?q=Artifact", nil)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	ids, next := galleryCardsAndNext(w.Body.String())
	assert.Len(t, ids, galleryPageSize)
	assert.Equal(t, "/partials/gallery-grid?offset=24&q=Artifact", next)
	assert.Equal(t, "/?q=Artifact", w.Header().Get("HX-Replace-Url"))
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.NotContains(t, w.Body.String(), "<html")
	assert.Contains(t, getPage(t, ro, "/partials/gallery-grid?q=missing"), "No artifacts match your search.")
	assert.Equal(t, "/other?offset=24&q=a%26b%3C", (galleryGridData{Query: "a&b<"}).url("/other", 24))
	// A filtered first page is the same partial the full page embeds.
	fragment := getPage(t, ro, "/partials/gallery-grid?q=Artifact")
	assert.Contains(t, normalizePerRender(getPage(t, ro, "/?q=Artifact")), normalizePerRender(fragment))
}

// The second gallery is deliberately test-only: no new user-facing feature is
// needed to prove the shared component keeps its selection and routes intact.
func TestGalleryDefinitionKeepsItsOwnFiltersPathsAndEmptyState(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryArtifacts(t, ro, 30)
	ctx := context.Background()
	collection := &store.Collection{ID: "reading", OwnerID: defaultOwnerID, Name: "Reading"}
	require.NoError(t, ro.cfg.Store.CreateCollection(ctx, collection))
	for i := range 26 {
		require.NoError(t, ro.cfg.Store.AddArtifactToCollection(ctx, defaultOwnerID, fmt.Sprintf("gallery-%03d", i), collection.ID))
	}
	gallery := galleryDefinition{ID: "reading-gallery", PagePath: "/reading", FragmentPath: "/partials/reading",
		Filters: store.ListOptions{Collections: []string{collection.Name}}}
	req := httptest.NewRequest(http.MethodGet, "/partials/reading?q=Artifact", nil)
	req = req.WithContext(withPrincipal(req.Context(), Principal{OwnerID: defaultOwnerID, Kind: PrincipalSession}))
	w := httptest.NewRecorder()
	ro.serveGalleryGrid(w, req, gallery)
	require.Equal(t, http.StatusOK, w.Code)
	ids, next := galleryCardsAndNext(w.Body.String())
	assert.Len(t, ids, galleryPageSize)
	assert.Equal(t, "gallery-025", ids[0])
	assert.Equal(t, "/partials/reading?offset=24&q=Artifact", next)
	assert.Equal(t, "/reading?q=Artifact", w.Header().Get("HX-Replace-Url"))
	assert.Contains(t, w.Body.String(), `hx-sync="#reading-gallery:abort"`)
	assert.NotContains(t, w.Body.String(), "library-gallery")
	grid, err := ro.loadGalleryGrid(req, gallery, 24)
	require.NoError(t, err)
	assert.Len(t, grid.Cards, 2)
	assert.Empty(t, grid.NextURL)
	gallery.Filters.Collections = []string{"empty"}
	grid, err = ro.loadGalleryGrid(httptest.NewRequest("GET", "/reading", nil), gallery, 0)
	require.NoError(t, err)
	body, err := renderPage("galleryGrid", grid)
	require.NoError(t, err)
	assert.Contains(t, body, "No artifacts in this gallery.")
	assert.NotContains(t, body, "/new")
}

type unavailableGalleryStore struct{ store.Store }

func (s unavailableGalleryStore) ListArtifacts(context.Context, store.ListOptions) ([]*store.Artifact, error) {
	return nil, errors.New("private database error")
}

func TestGalleryFailedPageHasAnExplicitRetryAtTheSameOffset(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Store = unavailableGalleryStore{ro.cfg.Store}
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/partials/gallery-grid?offset=48&q=chart", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	_, next := galleryCardsAndNext(w.Body.String())
	assert.Equal(t, "/partials/gallery-grid?offset=48&q=chart", next)
	assert.Contains(t, w.Body.String(), `hx-trigger="click"`)
	assert.NotContains(t, w.Body.String(), "revealed")
	assert.NotContains(t, w.Body.String(), "private database error")
	assert.Empty(t, w.Header().Get("HX-Replace-Url"))
}

func TestGalleryRejectsInvalidOffsets(t *testing.T) {
	ro := newTestRouter(t)
	for _, offset := range []string{"-1", "nope", "9223372036854775807", "99999999999999999999"} {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", "/partials/gallery-grid?offset="+offset, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, offset)
	}
}
