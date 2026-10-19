package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/artifacticon"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactViewerFaviconUsesTags(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Tagged tab")
	tags := []*store.Tag{{ID: "green", OwnerID: 1, Name: "Green", Color: "#10b981"}}
	require.NoError(t, r.cfg.Store.CreateTag(context.Background(), tags[0]))
	require.NoError(t, r.cfg.Store.AddArtifactTag(context.Background(), 1, id, "green"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/"+id, nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), strings.ReplaceAll(artifacticon.DataURI(tags), "+", "&#43;"))
}

// Static pages and automatic favicon probes retain the full brand mark.
// Artifact viewers use the E and current tag colors (Exh-qvmf).

// escapedFaviconURI is exhibitLogoDataURI as html/template emits it inside an
// href: the data: scheme survives because the value is typed template.URL,
// and the '+' in the MIME type is entity-escaped (the notfound test pins the
// same encoding for the hero image).
var escapedFaviconURI = strings.ReplaceAll(string(exhibitLogoDataURI), "+", "&#43;")

// TestAppOriginPagesCarryTheSameFavicon checks the static brand icon on app
// pages and the artifact-specific icon on the viewer.
func TestAppOriginPagesCarryTheSameFavicon(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Favicon check")

	want := `<link rel="icon" type="image/svg+xml" href="` + escapedFaviconURI + `">`
	for name, path := range pwaHeadPages(id) {
		t.Run(name, func(t *testing.T) {
			pageWant := want
			if name == "detail" {
				pageWant = `<link rel="icon" type="image/svg+xml" href="` + strings.ReplaceAll(artifacticon.DataURI(nil), "+", "&#43;") + `">`
			}
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Contains(t, w.Body.String(), pageWant,
				"%s must carry its expected tab icon", path)
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
// promise for automatic browser probes; artifact documents link their own icon.
func TestRenderFaviconServesTheBrandMark(t *testing.T) {
	r := newTestRouter(t)

	w := renderGet(t, r, "/favicon.ico")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/svg+xml", w.Header().Get("Content-Type"))
	assert.Equal(t, exhibitLogoSVG, w.Body.String(),
		"both origins must serve one artwork, not two marks that can drift")
}
