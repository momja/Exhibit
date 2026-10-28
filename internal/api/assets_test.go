package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditorAssetServed(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/assets/editor.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
	// The bundle exposes the mount entry point used by the edit page.
	assert.Contains(t, w.Body.String(), "ArtifactEditor")
}

func TestPhosphorIconAssetsServed(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/assets/phosphor/regular.css", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/css")
	css := w.Body.String()
	// The five icons this ticket introduces must be present in the vendored
	// stylesheet, and the @font-face must only reference the embedded woff2 —
	// no dangling references to formats we don't ship (which would 404).
	for _, class := range []string{"ph-pencil-simple", "ph-x", "ph-plus", "ph-trash", "ph-check"} {
		assert.Contains(t, css, "."+class+":before")
	}
	assert.Contains(t, css, `url("./Phosphor.woff2") format("woff2")`)
	assert.NotContains(t, css, ".woff\"")
	assert.NotContains(t, css, ".ttf\"")

	req = httptest.NewRequest("GET", "/assets/phosphor/Phosphor.woff2", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "font/woff2")
}

func TestGalleryPagesLoadPhosphorIconsFromAppOrigin(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Icon Check",
		"body":              "<html><body>content</body></html>",
		"network_allowlist": []string{},
	})

	stylesheetTag := `<link rel="stylesheet" href="` + assetURL("phosphor/regular.css") + `">`

	for _, path := range []string{"/", "/new", "/artifacts/" + id, "/artifacts/" + id + "/edit"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, path)
		page := w.Body.String()
		// The stylesheet is same-origin (a root-relative path), never a
		// third-party CDN URL.
		assert.Contains(t, page, stylesheetTag, path)
	}

	// The documented markup pattern — <i class="ph ph-<name>"></i> — renders
	// somewhere in the gallery, using the icon set this ticket introduces.
	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	page := w.Body.String()
	assert.Contains(t, page, `<i class="ph ph-check"></i>`)
	assert.Contains(t, page, `<i class="ph ph-x"></i>`)
	assert.Contains(t, page, `<i class="ph ph-trash"></i>`)
}

func TestGalleryEditPageMountsEditor(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Editable",
		"body":              "<html><body>content</body></html>",
		"network_allowlist": []string{},
	})

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	page := w.Body.String()
	// The edit page loads the CodeMirror island and the static page script
	// that mounts it on the body textarea, which stays present as the form's
	// source of truth.
	assert.Contains(t, page, `<script src="`+assetURL("editor.js")+`"></script>`)
	assert.Contains(t, page, `<script type="module" src="`+assetURL("gallery/edit.js")+`"></script>`)
	editJS, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	assert.Contains(t, string(editJS), "ArtifactEditor.mount")
	assert.Contains(t, page, `<textarea id="body" data-revision="`)
}

// The ingest page — /new since av-qo0j, the gallery index before it — loads
// the same CodeMirror island and the static page script that mounts it on the
// paste textarea, which stays present as the form's source of truth (av-44z3).
func TestNewPageMountsEditor(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/new", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	page := w.Body.String()
	assert.Contains(t, page, `<script src="`+assetURL("editor.js")+`"></script>`)
	assert.Contains(t, page, `<script src="`+assetURL("gallery/new.js")+`"></script>`)
	newJS, err := embeddedAssets.ReadFile("assets/gallery/new.js")
	require.NoError(t, err)
	assert.Contains(t, string(newJS), "ArtifactEditor.mount(document.getElementById('body'))")
	assert.Contains(t, page, `<textarea id="body" placeholder=`)
	newCSS, err := embeddedAssets.ReadFile("assets/gallery/new.css")
	require.NoError(t, err)
	assert.Contains(t, string(newCSS), `.ingest-panel .cm-editor{`)
}

// The library page has no editor left to mount: its only one was the ingest
// form's source field, which moved to /new with the rest of ingest.
func TestGalleryLibraryPageHasNoEditor(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	page := w.Body.String()
	assert.NotContains(t, page, "editor.js")
	assert.NotContains(t, page, `<textarea`)
}

// Exh-hl64: a page's assets are cached as immutable under a URL naming the
// build, so a relaunch reuses them without a round trip each.
func TestVersionedAssetsAreImmutable(t *testing.T) {
	r := newTestRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", assetURL("gallery/index.js"), nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
	want, err := embeddedAssets.ReadFile("assets/gallery/index.js")
	require.NoError(t, err)
	assert.Equal(t, string(want), w.Body.String())
}

// A page rendered before a deploy asks for its own version, and only the new
// bytes exist. It gets them, but nothing may cache them as that old version.
func TestStaleAssetVersionServesCurrentBytesUncached(t *testing.T) {
	r := newTestRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/v/000000000000/gallery/index.js", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
}

// Unversioned paths still serve, and revalidate by content rather than
// downloading again.
func TestUnversionedAssetsRevalidateByETag(t *testing.T) {
	r := newTestRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/gallery/index.js", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)

	req := httptest.NewRequest("GET", "/assets/gallery/index.js", nil)
	req.Header.Set("If-None-Match", etag)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotModified, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestAssetHandlerRefusesMissingFilesAndDirectories(t *testing.T) {
	r := newTestRouter(t)

	for _, path := range []string{
		"/assets/gallery/nope.js", assetURL("gallery/nope.js"),
		"/assets/gallery/", assetURL("gallery/"), assetURL(""), "/assets/v/",
		assetURL("../assets.go"),
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}
}

// Every asset a page names is versioned, or a relaunch would revalidate it.
func TestPagesReferenceOnlyVersionedAssets(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "Versioned", "body": "<p>x</p>", "network_allowlist": []string{}})

	for _, path := range []string{"/", "/new", "/artifacts/" + id, "/artifacts/" + id + "/edit", "/agent", "/settings", "/nope"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		page := w.Body.String()
		assert.Contains(t, page, versionedAssetPrefix+assetVersion+"/", path)
		assert.NotRegexp(t, `(src|href)="/assets/[^v]`, page, path)
	}
}

// The compressor may gzip an asset, so its validator is weak: a range against
// it is answered whole rather than spliced from the other representation, and
// a range request is never compressed.
func TestAssetRangesAreNeverSplicedAcrossEncodings(t *testing.T) {
	r := newTestRouter(t)
	want, err := embeddedAssets.ReadFile("assets/gallery/index.css")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/gallery/index.css", nil))
	etag := w.Header().Get("ETag")
	require.True(t, strings.HasPrefix(etag, `W/"`), etag)

	req := httptest.NewRequest("GET", "/assets/gallery/index.css", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=10-19")
	req.Header.Set("If-Range", etag)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "If-Range needs a strong validator")
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, string(want), w.Body.String())

	req = httptest.NewRequest("GET", "/assets/gallery/index.css", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=10-19")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, string(want[10:20]), w.Body.String())
}
