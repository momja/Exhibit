package api

import (
	"net/http"
	"net/http/httptest"
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

	const stylesheetTag = `<link rel="stylesheet" href="/assets/phosphor/regular.css">`

	for _, path := range []string{"/", "/artifacts/" + id, "/artifacts/" + id + "/edit"} {
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
	// The edit page loads the CodeMirror island and mounts it on the body
	// textarea, which stays present as the form's source of truth.
	assert.Contains(t, page, `<script src="/assets/editor.js"></script>`)
	assert.Contains(t, page, "ArtifactEditor.mount")
	assert.Contains(t, page, `<textarea id="body">`)
}

func TestGalleryLibraryPageMountsEditor(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	page := w.Body.String()
	// The library (upload) page loads the same CodeMirror island and mounts it
	// on the paste textarea, which stays present as the form's source of truth
	// (av-44z3). setMode toggles the mounted editor for the Paste/URL tabs.
	assert.Contains(t, page, `<script src="/assets/editor.js"></script>`)
	assert.Contains(t, page, "ArtifactEditor.mount(document.getElementById('body'))")
	assert.Contains(t, page, `<textarea id="body" placeholder=`)
	assert.Contains(t, page, `.upload .cm-editor{`)
}
