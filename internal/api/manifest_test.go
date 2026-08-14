package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestServesValidJSON exercises av-fdcx's AC1: GET /manifest.json
// returns valid manifest JSON pointing at the icons av-emh4 generates.
func TestManifestServesValidJSON(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/manifest.json", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/manifest+json", w.Header().Get("Content-Type"))

	var m webManifest
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))

	assert.Equal(t, "Exhibit", m.Name)
	assert.Equal(t, "standalone", m.Display)
	assert.Equal(t, "/", m.StartURL)
	require.Len(t, m.Icons, 3)
	for _, icon := range m.Icons {
		assert.Regexp(t, `^/assets/icons/`, icon.Src)
		assert.Equal(t, "image/png", icon.Type)
	}
}

// pwaHeadPages is every app-origin HTML page that must carry the shared
// "pwaHead" partial, keyed by a readable name. It lives here so the two tests
// that walk the app's pages — the home-screen tags (av-fdcx) and the
// standalone zoom guard (av-8zqr) — can never drift to different page sets.
func pwaHeadPages(artifactID string) map[string]string {
	return map[string]string{
		"gallery index": "/",
		"add artifact":  "/new",
		"detail":        "/artifacts/" + artifactID,
		"edit":          "/artifacts/" + artifactID + "/edit",
		"agent":         "/agent",
		"not found":     "/artifacts/does-not-exist",
	}
}

// TestAppOriginPagesCarryPWAHeadTags is av-fdcx's AC2: every app-origin
// page's <head> links the manifest and carries the apple-* home-screen
// tags. It exercises the actual routes rather than the "pwaHead" partial in
// isolation, so a template that forgets to include it fails this test.
func TestAppOriginPagesCarryPWAHeadTags(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "PWA head check")

	for name, path := range pwaHeadPages(id) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			body := w.Body.String()
			assert.Contains(t, body, `<link rel="manifest" href="/manifest.json">`)
			assert.Contains(t, body, `<link rel="apple-touch-icon" href="/assets/icons/apple-touch-icon-180.png">`)
			assert.Contains(t, body, `<meta name="apple-mobile-web-app-capable" content="yes">`)
			assert.Contains(t, body, `<meta name="apple-mobile-web-app-status-bar-style" content="default">`)
			assert.Contains(t, body, `<meta name="apple-mobile-web-app-title" content="Exhibit">`)
		})
	}
}
