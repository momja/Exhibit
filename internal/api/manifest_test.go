package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	assert.Equal(t, "#f1e9d6", m.ThemeColor, "theme follows the paper ground, not the logo blue")
	assert.Equal(t, "#f1e9d6", m.BackgroundColor)
	require.Len(t, m.Icons, 3)
	for _, icon := range m.Icons {
		assert.Regexp(t, `^/assets/icons/`, icon.Src)
		assert.Equal(t, "image/png", icon.Type)
	}
}

// pwaHeadPages is every app-origin HTML page that must carry the shared
// "pwaHead" partial, keyed by a readable name. Kept as one list so a test that
// walks the app's pages cannot quietly cover a different set than the next one
// to need it.
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
			assert.Contains(t, body, `<link rel="apple-touch-icon" href="`+assetURL("icons/apple-touch-icon-180.png")+`">`)
			assert.Contains(t, body, `<meta name="apple-mobile-web-app-capable" content="yes">`)
			assert.Contains(t, body, `<meta name="apple-mobile-web-app-status-bar-style" content="default">`)
			assert.Contains(t, body, `<meta name="apple-mobile-web-app-title" content="Exhibit">`)
		})
	}
}

// Paper and dusk are written in four places that cannot import each other:
// tokens.css, the manifest, the pwaHead theme-color metas and the icon build.
// They are all pinned to the --color-ground tokens here (Exh-d5ku).
func TestPaperAndDuskFollowGroundTokens(t *testing.T) {
	light, dark := parseTokens(t)
	paper, dusk := light["--color-ground"], dark["--color-ground"]
	require.NotEmpty(t, paper)
	require.NotEmpty(t, dusk)

	assert.Equal(t, paper, manifestPaper)
	assert.Equal(t, paper, exhibitManifest.ThemeColor)
	assert.Equal(t, paper, exhibitManifest.BackgroundColor)

	var out strings.Builder
	require.NoError(t, pageTemplates.ExecuteTemplate(&out, "pwaHead", nil))
	assert.Contains(t, out.String(), `<meta name="theme-color" media="(prefers-color-scheme: light)" content="`+paper+`">`)
	assert.Contains(t, out.String(), `<meta name="theme-color" media="(prefers-color-scheme: dark)" content="`+dusk+`">`)

	build, err := os.ReadFile("../../web/pwa-icons/build.mjs")
	require.NoError(t, err)
	assert.Contains(t, string(build), `BACKGROUND = "`+paper+`"`)
}
