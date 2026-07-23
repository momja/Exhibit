package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// get404 issues a GET and returns the recorder without asserting a status, so
// the callers below can assert on the status themselves — the point of most of
// these tests is that a 404 stayed a 404 while its body became a page.
func get404(t *testing.T, r *Router, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Every app-origin HTML route that can 404 renders the same page, and none of
// them softened the status to 200 on the way (a styled 404 that answers 200 is
// worse than the bare text it replaced: it lies to crawlers and clients).
func TestHTMLRoutesRenderStyled404(t *testing.T) {
	r := newTestRouter(t)

	tests := []struct {
		name string
		path string
	}{
		{"unrouted path", "/nowhere"},
		{"missing artifact detail", "/artifacts/does-not-exist"},
		{"missing artifact edit", "/artifacts/does-not-exist/edit"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := get404(t, r, tc.path)
			require.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))

			page := w.Body.String()
			assert.Contains(t, page, `class="exhibit-404__frame"`,
				"the 404 hero is the brand mark knocked off level")
			assert.Contains(t, page, "Nothing hanging here.")
			assert.Contains(t, page, `<link rel="stylesheet" href="/assets/gallery/notfound.css">`)
			// The way out is the app's real controls, not decoration: the
			// gallery link and a GET form the index already answers via ?q=.
			assert.Contains(t, page, `<form class="exhibit-404__find" action="/" method="get">`)
			assert.Contains(t, page, `<a class="btn" href="/">`)
			// Whatever was asked for is echoed back so the reader can see the
			// typo they arrived with.
			assert.Contains(t, page, `<code>`+tc.path+`</code>`)
		})
	}
}

// The mark is inlined once (header) and drawn from the data URI once (hero).
// A second inline <svg> would duplicate every gradient/filter id on the page,
// and the second copy's url(#…) paint would resolve against the first copy's
// <defs> — so the artwork has exactly one inline instance.
func TestNotFoundPageInlinesLogoOnce(t *testing.T) {
	page, err := renderNotFoundPage("/nowhere")
	require.NoError(t, err)

	assert.Equal(t, 1, strings.Count(page, `id="filter30"`),
		"the logo's defs must appear once per document")
	// html/template escapes the data URI's '+' as &#43; in the attribute, same
	// as it already does for the favicon link — the browser decodes it back.
	assert.Contains(t, page, `<img class="exhibit-404__frame" src="data:image/svg&#43;xml;base64,`,
		"the hero hangs the same artwork as an image, not a second inline svg")
}

// The requested path is attacker-controlled URL text. It reaches the template
// as a value, so html/template escapes it; this pins that it is never emitted
// raw, whatever a future edit does to the markup around it.
func TestNotFoundEscapesRequestedPath(t *testing.T) {
	r := newTestRouter(t)

	w := get404(t, r, "/%3Cscript%3Ealert(1)%3C/script%3E")
	require.Equal(t, http.StatusNotFound, w.Code)

	page := w.Body.String()
	assert.NotContains(t, page, "<script>alert(1)</script>")
	assert.Contains(t, page, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

// /api/* has JSON and text clients; a page in the body would be a breaking
// change for them. chi copies the not-found handler into every subrouter, so
// this covers both a bad top-level API path and one under a registered route.
func TestAPI404sStayPlain(t *testing.T) {
	r := newTestRouter(t)

	for _, path := range []string{"/api/nope", "/api/artifacts/abc/nope"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", authHeader())
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusNotFound, w.Code)
			assert.Equal(t, "404 page not found\n", w.Body.String())
			assert.NotContains(t, w.Body.String(), "exhibit-404")
		})
	}
}

// The motion is the whole point of the page, and it lives entirely in the
// stylesheet — which also proves notfound.css made it through the asset build
// (a page that 404s its own stylesheet is the failure this catches).
func TestNotFoundStylesheetCarriesTheKnock(t *testing.T) {
	r := newTestRouter(t)
	css := galleryAsset(t, r, "/assets/gallery/notfound.css")

	// The pivot is the nail drawn into the mark, not the box centre.
	assert.Contains(t, css, "transform-origin:49.7% 4.2%")
	assert.Contains(t, css, "animation:knock 2.8s ease-in-out .25s both")
	assert.Contains(t, css, "@keyframes knock{")
	assert.Contains(t, css, "8%{rotate:-15deg}", "struck on the left")
	// No swing under reduced motion — the frame just hangs crooked.
	assert.Contains(t, css, "@media (prefers-reduced-motion:reduce){")
	assert.Contains(t, css, ".exhibit-404__frame{animation:none;rotate:-3deg}")
	// Mobile hangs off the 640px breakpoint the other page sheets share.
	assert.Contains(t, css, "@media (max-width:640px){")

	js := galleryAsset(t, r, "/assets/gallery/notfound.js")
	assert.Contains(t, js, "prefers-reduced-motion: reduce",
		"the replay must not re-animate a frame the visitor asked to hold still")
}
