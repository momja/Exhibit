package api

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exh-wjqh: every app-origin page loads the brand fonts from Google Fonts
// through the one shared fontsHead partial.
func TestAppOriginPagesLoadBrandFonts(t *testing.T) {
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
			assert.Contains(t, body, `<link rel="preconnect" href="https://fonts.googleapis.com">`)
			assert.Contains(t, body, `<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>`)
			assert.Contains(t, body, "family=Fraunces:opsz,wght,SOFT,WONK@72,600,100,0&family=Schibsted+Grotesk:wght@400..800&display=swap")
			// The stylesheet is the one request that could carry a Referer to
			// Google; it must carry none, whatever the browser's default policy.
			sheets := googleFontStylesheets.FindAllString(body, -1)
			require.Len(t, sheets, 1, "one Google Fonts stylesheet")
			assert.Contains(t, sheets[0], ` referrerpolicy="no-referrer"`)
		})
	}
}

// googleFontStylesheets matches every stylesheet link to the Google Fonts
// stylesheet host, whatever its attribute order.
var googleFontStylesheets = regexp.MustCompile(`<link [^>]*rel="stylesheet"[^>]*fonts\.googleapis\.com[^>]*>|<link [^>]*fonts\.googleapis\.com[^>]*rel="stylesheet"[^>]*>`)

// The app origin's only Content-Security-Policy headers restrict framing.
// With relay on, the detail and agent pages carry one; it must be there, name
// its framing directives, and say nothing about styles or fonts, so the
// Google Fonts hosts stay reachable.
func TestAppFramingCSPLeavesGoogleFontsAlone(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Relay.Enabled = true
	id := createTestArtifact(t, r, "Fonts CSP")

	for _, path := range []string{"/artifacts/" + id, "/agent"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		csp := w.Header().Get("Content-Security-Policy")
		require.NotEmpty(t, csp, "%s should carry its framing CSP", path)
		assert.Contains(t, csp, "frame-src", path)
		for _, directive := range []string{"default-src", "style-src", "font-src"} {
			assert.NotContains(t, csp, directive, "%s CSP must leave fonts alone", path)
		}
		assert.False(t, strings.Contains(csp, "fonts.g"), path)
	}
}
