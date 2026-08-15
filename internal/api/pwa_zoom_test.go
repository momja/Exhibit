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
// touches the viewport, cancels a pinch, or reveals the text-size control.
func TestZoomGuardOnlyActsWhenStandalone(t *testing.T) {
	r := newTestRouter(t)
	js := galleryAsset(t, r, "/assets/gallery/pwa.js")

	gate := strings.Index(js, `if (!standalone) return;`)
	require.NotEqual(t, -1, gate, "guard has no standalone gate")
	assert.Contains(t, js, `window.navigator.standalone === true`, "iOS home-screen launch not detected")
	assert.Contains(t, js, `displayMode('standalone')`, "installed-PWA display mode not detected")

	// Every effect — the viewport scale, the cancelled WebKit gesture events,
	// and unhiding the control — must sit behind that gate, so each is looked
	// for only in what follows the early return.
	gated := js[gate:]
	for _, effect := range []string{
		`user-scalable=no`,
		`'gesturestart'`,
		`e.preventDefault();`,
		`control.hidden = false;`,
	} {
		assert.Contains(t, gated, effect, "%s is missing or not gated on standalone mode", effect)
	}
}

// TestTextScaleControlReachesTwoHundredPercent is av-8zqr's AC4, the half that
// makes disabling the pinch shippable: an installed app with no pinch zoom
// still has to resize text to 200% (WCAG 1.4.4), which the header control
// does. The pinch stays disabled at every step because each scale pins
// minimum and maximum to itself, leaving no range to zoom through.
func TestTextScaleControlReachesTwoHundredPercent(t *testing.T) {
	r := newTestRouter(t)
	js := galleryAsset(t, r, "/assets/gallery/pwa.js")

	assert.Contains(t, js, `var SCALES = [1, 1.25, 1.5, 1.75, 2];`, "scale steps must reach 2 (200%)")
	assert.Contains(t, js, `',minimum-scale=' + scale + ',maximum-scale=' + scale +`,
		"a scale that does not pin minimum to maximum would leave the pinch alive")

	// A viewport scale is only honoured when it is present at parse time —
	// rewriting the tag later changes the attribute and nothing on screen —
	// so the control has to store and reload rather than apply in place.
	assert.Contains(t, js, `window.location.reload();`,
		"a scale change that does not reload cannot take effect")
}

// TestPagesHoldingUnsavedWorkWarnBeforeScaleReload covers the cost of that
// reload: on the pages carrying an editor buffer, a pasted body, or an agent
// conversation it would discard work, so those three declare themselves and
// the control confirms first. The read-only pages must NOT declare it — a
// confirm on every text-size change is friction aimed at the people least able
// to afford it.
func TestPagesHoldingUnsavedWorkWarnBeforeScaleReload(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Reload warning check")

	warns := map[string]bool{
		"/artifacts/" + id + "/edit": true,
		"/new":                       true,
		"/agent":                     true,
		"/":                          false,
		"/artifacts/" + id:           false,
		"/artifacts/does-not-exist":  false,
	}

	for path, wantWarn := range warns {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			body := w.Body.String()
			if wantWarn {
				assert.Contains(t, body, `<body data-scale-reload-warn>`)
			} else {
				assert.NotContains(t, body, `data-scale-reload-warn`)
			}
		})
	}
}

// TestAppOriginPagesCarryTextScaleControl is av-8zqr's AC4 in the markup: the
// control ships on every app-origin page and ships hidden, so a browser tab —
// where the browser's own zoom is the resize path — never shows a second one.
func TestAppOriginPagesCarryTextScaleControl(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Text scale check")

	for name, path := range pwaHeadPages(id) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			body := w.Body.String()
			assert.Contains(t, body, `role="group" aria-label="Text size" data-text-scale hidden`,
				"page is missing the text-size control, or ships it visible")
			assert.Contains(t, body, `data-text-scale-step="-1"`)
			assert.Contains(t, body, `data-text-scale-step="1"`)
		})
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
