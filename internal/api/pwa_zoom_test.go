package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppOriginPagesLoadZoomGuard is av-8zqr's AC1/AC2: every app-origin page
// loads pwa.js, and loads it *after* its own viewport meta — the script
// rewrites that tag, so a page that declared it later would hand the guard
// nothing to rewrite.
func TestAppOriginPagesLoadZoomGuard(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Zoom guard check")

	for name, path := range pwaHeadPages(id) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			body := w.Body.String()
			script := strings.Index(body, `<script src="/assets/gallery/pwa.js"></script>`)
			require.NotEqual(t, -1, script, "page does not load the zoom guard")

			meta := strings.Index(body, `<meta name="viewport"`)
			require.NotEqual(t, -1, meta, "page declares no viewport meta")
			assert.Less(t, meta, script, "viewport meta must precede pwa.js")
		})
	}
}

// TestZoomGuardOnlyActsWhenStandalone is av-8zqr's AC3: pinch-to-zoom is only
// disabled for a home-screen launch. The guard is client-side, so this asserts
// on the served script: it gates on the standalone display mode before it
// touches the viewport or cancels a pinch.
func TestZoomGuardOnlyActsWhenStandalone(t *testing.T) {
	r := newTestRouter(t)
	js := galleryAsset(t, r, "/assets/gallery/pwa.js")

	gate := strings.Index(js, `if (!standalone) return;`)
	require.NotEqual(t, -1, gate, "guard has no standalone gate")
	assert.Contains(t, js, `window.navigator.standalone === true`, "iOS home-screen launch not detected")
	assert.Contains(t, js, `displayMode('standalone')`, "installed-PWA display mode not detected")

	// Both mechanisms — the viewport rewrite (Chrome) and the cancelled
	// WebKit gesture events — must sit behind that gate, so each is looked
	// for only in what follows the early return.
	gated := js[gate:]
	for _, effect := range []string{
		`user-scalable=no`,
		`'gesturestart'`,
		`e.preventDefault();`,
	} {
		assert.Contains(t, gated, effect, "%s is missing or not gated on standalone mode", effect)
	}
}

// TestZoomGuardServedFromAppOrigin keeps the guard on the app surface: it is
// page chrome, and the render origin's artifact documents own their own head.
func TestZoomGuardServedFromAppOrigin(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/assets/gallery/pwa.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
}
