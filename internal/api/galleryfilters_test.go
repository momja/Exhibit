package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedGalleryFilters(t *testing.T, ro *Router, owner int64) {
	t.Helper()
	ctx := context.Background()
	for i, name := range []string{"Work", "Read later"} {
		require.NoError(t, ro.cfg.Store.CreateTag(ctx, &store.Tag{ID: fmt.Sprintf("tag-%d-%d", owner, i), OwnerID: owner, Name: name, Color: []string{"#2563eb", "#d97706"}[i]}))
	}
	for i := range 2 {
		// Identical names still represent distinct collection IDs.
		require.NoError(t, ro.cfg.Store.CreateCollection(ctx, &store.Collection{ID: fmt.Sprintf("collection-%d-%d", owner, i), OwnerID: owner, Name: "Reading"}))
	}
}

func TestGalleryFiltersValidationAndPaging(t *testing.T) {
	in := newGrantedInstance(t)
	seedSharedGallery(t, in, 49)
	seedGalleryFilters(t, in.ro, in.ownerOne)
	seedGalleryFilters(t, in.ro, in.ownerTwo)
	ctx := context.Background()
	// Forty-nine satisfy all filters; the existing grant and a decoy do not.
	for i := range 49 {
		id := fmt.Sprintf("shared-%03d", i)
		for j := range 2 {
			require.NoError(t, in.ro.cfg.Store.AddArtifactTag(ctx, in.ownerOne, id, fmt.Sprintf("tag-%d-%d", in.ownerOne, j)))
			require.NoError(t, in.ro.cfg.Store.AddArtifactToCollection(ctx, in.ownerOne, id, fmt.Sprintf("collection-%d-%d", in.ownerOne, j)))
		}
	}
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(ctx, in.ownerOne, in.granted, map[string]any{"title": "Shared chart partial filters"}))
	require.NoError(t, in.ro.cfg.Store.AddArtifactTag(ctx, in.ownerOne, in.granted, fmt.Sprintf("tag-%d-0", in.ownerOne)))
	require.NoError(t, in.ro.cfg.Store.AddArtifactToCollection(ctx, in.ownerOne, in.granted, fmt.Sprintf("collection-%d-0", in.ownerOne)))
	// Test each repeated axis independently, so the other axis or search cannot
	// accidentally hide a partial-match artifact.
	for _, filters := range []url.Values{
		{"tag": {"Work", "Read later"}},
		{"collection": {fmt.Sprintf("collection-%d-0", in.ownerOne), fmt.Sprintf("collection-%d-1", in.ownerOne)}},
	} {
		page := in.get(t, "/?"+filters.Encode(), in.cookieOne)
		require.Equal(t, http.StatusOK, page.Code)
		ids, _ := galleryCardsAndNext(page.Body.String())
		assert.NotContains(t, ids, in.granted)
	}
	params := url.Values{"tag": {"Work", "Read later"}, "collection": {fmt.Sprintf("collection-%d-0", in.ownerOne), fmt.Sprintf("collection-%d-1", in.ownerOne)}, "q": {"chart"}}
	for _, space := range []struct{ page, fragment string }{{"/", "/partials/gallery-grid"}, {"/shared-by-me", "/partials/gallery-grid/shared-by-me"}} {
		path := space.page + "?" + params.Encode()
		var all []string
		for n := 0; path != ""; n++ {
			require.Less(t, n, 4)
			page := in.get(t, path, in.cookieOne)
			require.Equal(t, http.StatusOK, page.Code, page.Body.String())
			ids, next := galleryCardsAndNext(page.Body.String())
			all = append(all, ids...)
			if next != "" {
				parsed, err := url.Parse(next)
				require.NoError(t, err)
				for name, values := range params {
					assert.Equal(t, values, parsed.Query()[name])
				}
			}
			if n == 0 {
				assert.Contains(t, page.Body.String(), "Clear all")
			} else {
				assert.NotContains(t, page.Body.String(), "Active filters")
			}
			path = next
		}
		require.Len(t, all, 49)
		assert.NotContains(t, all, in.granted)
		page := in.get(t, space.fragment+"?"+params.Encode()+"&tag=Work", in.cookieOne)
		require.Equal(t, http.StatusOK, page.Code)
		assert.Equal(t, space.page+"?"+params.Encode(), page.Header().Get("HX-Replace-Url"))
	}
	for _, query := range []string{"tag=missing", "tag=", "collection=", "collection=Reading", "tag=Work&tag=missing", fmt.Sprintf("collection=collection-%d-0", in.ownerTwo)} {
		for _, path := range []string{"/", "/shared-by-me", "/partials/gallery-grid", "/partials/gallery-grid/shared-by-me"} {
			assert.Equal(t, http.StatusBadRequest, in.get(t, path+"?"+query, in.cookieOne).Code, path+"?"+query)
		}
	}
	// Incoming shares expose owners rather than either party's private organization.
	incoming := in.get(t, fmt.Sprintf("/shared?owner=%d&tag=Work&collection=foreign", in.ownerOne), in.cookieTwo)
	assert.Equal(t, http.StatusOK, incoming.Code)
	assert.Contains(t, incoming.Body.String(), ">Owners</h3>")
	assert.NotContains(t, incoming.Body.String(), ">Tags</h3>")
	assert.NotContains(t, incoming.Body.String(), ">Collections</h3>")
	assert.Contains(t, incoming.Body.String(), `aria-label="Active filters"`)
}

func TestGalleryFilterSoloLayoutAndEmptyLibrary(t *testing.T) {
	ro := newTestRouter(t)
	original := getPage(t, ro, "/")
	assert.NotContains(t, original, "gallery-layout")
	assert.NotContains(t, original, "gallery-filter-sheet")
	assert.NotContains(t, original, "data-gallery-filter")
	seedGalleryFilters(t, ro, defaultOwnerID)
	page := getPage(t, ro, "/")
	assert.Contains(t, page, `class="gallery-layout gallery-layout-solo"`)
	assert.Contains(t, page, `class="gallery-rail"`)
	assert.NotContains(t, page, `aria-label="Library spaces"`)
	assert.Contains(t, page, "gallery-filter-sheet")
	assert.Contains(t, page, "--tag-color:#2563eb")
	// A collection ID cannot match another collection with the same name.
	seedGalleryArtifacts(t, ro, 2)
	require.NoError(t, ro.cfg.Store.AddArtifactToCollection(context.Background(), defaultOwnerID, "gallery-000", "collection-1-0"))
	require.NoError(t, ro.cfg.Store.AddArtifactToCollection(context.Background(), defaultOwnerID, "gallery-001", "collection-1-1"))
	ids, _ := galleryCardsAndNext(getPage(t, ro, "/?collection=collection-1-0"))
	assert.Equal(t, []string{"gallery-000"}, ids)
}

func TestGalleryFilterRetryAndHTMXControls(t *testing.T) {
	ro := newTestRouter(t)
	seedGalleryFilters(t, ro, defaultOwnerID)
	ro.cfg.Store = unavailableGalleryStore{ro.cfg.Store}
	for _, fragment := range []string{"/partials/gallery-grid", "/partials/gallery-grid/shared-by-me"} {
		path := fragment + "?collection=collection-1-0&offset=24&q=chart&tag=Work"
		req := httptest.NewRequest("GET", path, nil)
		page := httptest.NewRecorder()
		ro.ServeHTTP(page, req)
		require.Equal(t, http.StatusInternalServerError, page.Code)
		_, retry := galleryCardsAndNext(page.Body.String())
		assert.Equal(t, path, retry)
	}
	ro.cfg.Store = ro.cfg.Store.(unavailableGalleryStore).Store
	req := httptest.NewRequest("GET", "/partials/gallery-grid?tag=Work", nil)
	req.Header.Set("HX-Request", "true")
	page := httptest.NewRecorder()
	ro.ServeHTTP(page, req)
	require.Equal(t, http.StatusOK, page.Code)
	for _, target := range []string{"search-parameters", "rail-filters", "sheet-filters"} {
		assert.Contains(t, page.Body.String(), `hx-swap-oob="innerHTML:#library-gallery-`+target+`"`)
	}
	assert.Equal(t, 1, strings.Count(page.Body.String(), `aria-label="Active filters"`))
}
