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
