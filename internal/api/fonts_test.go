package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exh-wjqh: the brand fonts are self-hosted. Every app-origin page loads them
// from its own origin through the one shared fontsHead partial.
func TestAppOriginPagesLoadSelfHostedBrandFonts(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Fonts check")

	pages := map[string]string{}
	for name, path := range pwaHeadPages(id) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		pages[name] = w.Body.String()
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/settings", nil))
	pages["settings"] = w.Body.String()
	for name, body := range pwaHeadConfiguredPages(t) {
		pages[name] = body
	}

	for name, body := range pages {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, body, `<link rel="stylesheet" href="`+assetURL("fonts/fonts.css")+`">`)
			assert.Contains(t, body, `<link rel="preload" href="`+assetURL("fonts/schibsted-grotesk.woff2")+`" as="font" type="font/woff2" crossorigin>`)
			assert.NotContains(t, body, "fonts.googleapis.com")
			assert.NotContains(t, body, "fonts.gstatic.com")
		})
	}
}

// The font files are embedded in the binary and served from the app origin,
// every url() in fonts.css resolving to a real woff2 at a versioned URL.
func TestBrandFontAssetsAreEmbeddedAndServed(t *testing.T) {
	r := newTestRouter(t)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}

	cssPath := assetURL("fonts/fonts.css")
	css := get(cssPath)
	require.Equal(t, http.StatusOK, css.Code)
	assert.Contains(t, css.Header().Get("Content-Type"), "text/css")
	assert.Contains(t, css.Header().Get("Cache-Control"), "immutable", "versioned font URLs are immutable")
	body := css.Body.String()
	for _, want := range []string{`font-family:"Schibsted Grotesk"`, "font-family:Fraunces"} {
		assert.Contains(t, body, want)
	}
	assert.Equal(t, 2, strings.Count(body, "font-display:swap"), "every face swaps")

	urls := regexp.MustCompile(`url\("\./([^"]+)"\)`).FindAllStringSubmatch(body, -1)
	require.Len(t, urls, 2)
	dir := cssPath[:strings.LastIndex(cssPath, "/")+1]
	for _, m := range urls {
		w := get(dir + m[1])
		require.Equal(t, http.StatusOK, w.Code, m[1])
		assert.True(t, strings.HasPrefix(w.Body.String(), "wOF2"), "%s is a woff2 file", m[1])
		assert.Greater(t, w.Body.Len(), 10_000, "%s is the real font, not a stub", m[1])
	}
	for _, licence := range []string{"fonts/LICENSE-schibsted-grotesk.txt", "fonts/LICENSE-fraunces.txt"} {
		_, err := fs.Stat(embeddedAssets, "assets/"+licence)
		assert.NoError(t, err, "the OFL licence ships with the font")
	}
}

// Exh-odyv: one Fraunces headline per page, on the page's natural heading.
func TestEachPageHasOneHeadline(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Headline check")

	pages := map[string]string{
		"starred":   "/",
		"search":    "/search",
		"shared":    "/shared",
		"activity":  "/activity",
		"detail":    "/artifacts/" + id,
		"edit":      "/artifacts/" + id + "/edit",
		"settings":  "/settings",
		"new":       "/new",
		"not found": "/artifacts/does-not-exist",
	}
	for name, path := range pages {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			assert.Equal(t, 1, strings.Count(w.Body.String(), "font-headline"), "%s should carry one headline", name)
		})
	}
}

// Exh-odyv: gallery cards have no border, so forced-colors mode (which drops
// shadows) must draw their edge as an outline.
func TestGalleryCardKeepsEdgeInForcedColors(t *testing.T) {
	css := galleryAsset(t, newTestRouter(t), "/assets/gallery/index.css")
	assert.Contains(t, css, "@media (forced-colors:active){.card{outline:1px solid CanvasText;outline-offset:-1px}")
}

// No app-origin template or stylesheet may name a Google Fonts host: a page
// view must send nothing to a third party and no third-party request may hold a
// page blank. The browser tests also make those hosts unresolvable
// (chromiumtest) as a second guard.
func TestNoAppOriginMarkupReferencesGoogleFonts(t *testing.T) {
	hosts := regexp.MustCompile(`fonts\.googleapis\.com|fonts\.gstatic\.com`)
	check := func(path string, data []byte) {
		if loc := hosts.FindIndex(data); loc != nil {
			t.Errorf("%s references a Google Fonts host (%q)", path, data[loc[0]:loc[1]])
		}
	}
	for _, root := range []string{"templates", "../../web/gallery"} {
		require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.Contains(path, "node_modules") || strings.HasSuffix(path, "_test.mjs") || strings.HasSuffix(path, ".test.mjs") {
				return err
			}
			data, err := os.ReadFile(path)
			if err == nil {
				check(path, data)
			}
			return err
		}))
	}
	require.NoError(t, fs.WalkDir(embeddedAssets, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".css") {
			return err
		}
		data, err := embeddedAssets.ReadFile(path)
		if err == nil {
			check(path, data)
		}
		return err
	}))
}
