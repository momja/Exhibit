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

	assert.Contains(t, page, `class="login-page login-painted"`)
	assert.Contains(t, page, `A shelf for small tools.`)
	assert.Contains(t, page, `class="login-wordmark"`)
	assert.Contains(t, page, `<picture class="login-scene-composite">`)
	assert.Regexp(t, `<img class="login-scene-image"[^>]* width="2560" height="1440"`, page)
	assert.NotContains(t, page, "-sky-", "the layer set belongs to the descent story, not first paint")
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
	assert.False(t, strings.Contains(page, "<script"), "the login page stays scriptless")
}
