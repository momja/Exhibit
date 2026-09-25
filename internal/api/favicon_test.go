package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nw-6184. The artifact pages (detail, edit) and the agent page never carried
// a favicon link while every other app-origin page did, and the render origin
// served no icon file at all — so those tabs wore the browser's default while
// the gallery wore the brand mark. The fix is one artwork everywhere: the
// pages inline it as a data-URI link, and both origins serve the same bytes
// as a file for the requests no link covers.

// escapedFaviconURI is exhibitLogoDataURI as html/template emits it inside an
// href: the data: scheme survives because the value is typed template.URL,
// and the '+' in the MIME type is entity-escaped (the notfound test pins the
// same encoding for the hero image).
var escapedFaviconURI = strings.ReplaceAll(string(exhibitLogoDataURI), "+", "&#43;")

// TestAppOriginPagesCarryTheSameFavicon walks every app-origin page and
// requires the byte-identical icon link on each — not just "a link", but the
// same artwork, so a page can never quietly drift to a second mark again.
func TestAppOriginPagesCarryTheSameFavicon(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Favicon check")

	want := `<link rel="icon" type="image/svg+xml" href="` + escapedFaviconURI + `">`
	for name, path := range pwaHeadPages(id) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Contains(t, w.Body.String(), want,
				"%s must wear the same brand mark as every other app page", path)
		})
	}
}

// TestFaviconRouteServesTheBrandMark is the file twin of the inline link:
// GET /favicon.ico answers the browser's automatic probe (and bookmarks,
// restored tabs) with the same compiled-in logo the pages inline.
func TestFaviconRouteServesTheBrandMark(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/favicon.ico", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/svg+xml", w.Header().Get("Content-Type"))
	assert.Equal(t, exhibitLogoSVG, w.Body.String(),
		"the file route must serve the same artwork the pages inline")
}

// TestRenderFaviconServesTheBrandMark is the render origin's half of the same
// promise: a document without an icon of its own falls back to the origin's
// /favicon.ico, so render pages wear the brand mark without the surface ever
// rewriting a visitor-authored <head>.
func TestRenderFaviconServesTheBrandMark(t *testing.T) {
	r := newTestRouter(t)

	w := renderGet(t, r, "/favicon.ico")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/svg+xml", w.Header().Get("Content-Type"))
	assert.Equal(t, exhibitLogoSVG, w.Body.String(),
		"both origins must serve one artwork, not two marks that can drift")
}
