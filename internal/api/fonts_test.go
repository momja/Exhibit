package api

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Exh-wjqh: every app-origin page loads the brand fonts from Google Fonts,
// through the one shared fontsHead partial.
func TestAppOriginPagesLoadBrandFonts(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Fonts check")

	pages := pwaHeadPages(id)
	pages["settings"] = "/settings"
	for name, path := range pages {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			body := w.Body.String()
			assert.Contains(t, body, `<link rel="preconnect" href="https://fonts.googleapis.com">`)
			assert.Contains(t, body, `<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>`)
			assert.Contains(t, body, "family=Fraunces:opsz,wght,SOFT,WONK@72,600,100,0&family=Schibsted+Grotesk:wght@400..800&display=swap")
		})
	}
}

// The app origin sets a Content-Security-Policy only to restrict framing, and
// none of those policies may restrict styles or fonts, or the Google Fonts
// hosts would be blocked on that page.
func TestAppPageCSPDoesNotBlockGoogleFonts(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Relay.Enabled = true
	id := createTestArtifact(t, r, "Fonts CSP")

	for _, path := range []string{"/artifacts/" + id, "/agent"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		csp := w.Header().Get("Content-Security-Policy")
		for _, directive := range []string{"default-src", "style-src", "font-src"} {
			assert.NotContains(t, csp, directive, "%s CSP must leave fonts alone", path)
		}
	}
}
