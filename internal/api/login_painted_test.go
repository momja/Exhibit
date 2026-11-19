package api

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The painted front door (Exh-90b2): both colour modes and the phone crop are
// offered through <picture>, AVIF before WebP, with explicit dimensions, and
// only art that is served; the flat mark and the headline are present on first
// paint, in the markup rather than behind a script.
func TestLoginPageIsPainted(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, httptest.NewRequest("GET", "/auth/login", nil))
	require.Equal(t, 200, w.Code)
	page := w.Body.String()

	// The first visit also plays the descent (login_descent_test.go); the
	// painted page itself is the same either way.
	assert.Contains(t, page, `class="login-page login-painted login-descend"`)
	assert.Contains(t, page, `A space for <span class="login-headline-ending"><em>your</em> <span class="sr-only">tools</span>`)
	assert.Contains(t, page, `<span>tools</span><span>apps</span><span>art</span><span>games</span><span>tools</span>`)
	assert.Contains(t, page, `class="login-wordmark"`)
	assert.Contains(t, page, `<picture class="login-scene-composite">`)
	assert.Regexp(t, `<img class="login-scene-image"[^>]* width="2560" height="1440"`, page)
	// The layer art is inert inside a <template> (fetched only by the descent
	// module) and absent from the visible markup.
	template := regexp.MustCompile(`(?s)<template>.*?</template>`)
	assert.Contains(t, template.FindString(page), "-sky-")
	page = template.ReplaceAllString(page, "")
	assert.NotContains(t, page, "-sky-")
	assert.NotContains(t, page, "-clouds-")

	sources := regexp.MustCompile(`<source [^>]*>`).FindAllString(page, -1)
	require.Len(t, sources, 8)
	for i, s := range sources {
		wantType := "image/avif"
		if i%2 == 1 {
			wantType = "image/webp"
		}
		assert.Contains(t, s, `type="`+wantType+`"`, "AVIF comes before WebP in each pair")
		assert.Contains(t, s, `width=`)
		assert.Contains(t, s, `height=`)
	}
	assert.Contains(t, sources[0], "prefers-color-scheme:dark")
	assert.Contains(t, sources[0], "dusk-phone-")
	assert.Contains(t, sources[2], "day-phone-")
	assert.Contains(t, sources[4], "dusk-composite-")
	assert.Contains(t, sources[6], "day-composite-")

	for _, u := range regexp.MustCompile(`/assets/v/[^"\s,]+hero-art/[^"\s,]+`).FindAllString(page, -1) {
		r := httptest.NewRecorder()
		ro.ServeHTTP(r, httptest.NewRequest("GET", u, nil))
		assert.Equal(t, 200, r.Code, u)
	}
	// Scriptless except for the first visit's descent module (login_descent_test.go).
	assert.Equal(t, 1, strings.Count(page, "<script"), "only the descent module")
	assert.Regexp(t, `<script type="module" src="/assets/v/[^"]+/gallery/login-descent\.js"></script>`, page)
	assert.NotRegexp(t, `<script(?:\s[^>]*)?>\s*[^<\s]`, page, "no inline script")
	assert.Regexp(t, `<noscript><style>[^<]*login-scene-composite[^<]*</style></noscript>`, page, "without JS the composite is shown; a noscript style is not a script")
}
