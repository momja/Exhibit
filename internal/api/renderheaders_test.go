package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-nr0p. A render URL carries a credential in its query string (av-c5aq), so
// every response the render origin emits must withhold its Referer — otherwise
// an honest artifact fetching a font from an allowlisted CDN hands that CDN the
// render URL, token and all, in its access log.
//
// Both halves of the table matter. The success rows are the ones an artifact
// runs in; the failure rows are there because a rejected token still travelled
// in the URL that produced the 404, and a header that only appears when the
// render succeeded protects the case that was already fine.
func TestRenderOriginWithholdsTheReferrer(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)
	shareID := createShare(t, r, id)
	tok := r.tokens.Mint(id, defaultOwnerID)

	cases := []struct {
		name, route, target string
		wantCode            int
	}{
		{"artifact", "/a/{artifactID}", "/a/" + id + "?" + rendertoken.Param + "=" + tok, http.StatusOK},
		{"artifact, no token", "/a/{artifactID}", "/a/" + id, http.StatusNotFound},
		{"artifact, bad token", "/a/{artifactID}", "/a/" + id + "?" + rendertoken.Param + "=bogus", http.StatusNotFound},
		{"artifact, unknown id", "/a/{artifactID}", "/a/does-not-exist", http.StatusNotFound},
		{"widget", "/w/{artifactID}", "/w/" + id + "?" + rendertoken.Param + "=" + tok, http.StatusOK},
		{"widget, no token", "/w/{artifactID}", "/w/" + id, http.StatusNotFound},
		{"share", "/s/{shareID}", "/s/" + shareID, http.StatusOK},
		{"share, unknown id", "/s/{shareID}", "/s/does-not-exist", http.StatusNotFound},
	}

	covered := map[string]bool{}
	for _, tc := range cases {
		covered[tc.route] = true
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := renderGet(t, r, tc.target)
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			assert.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"),
				"%s leaks its render token to every origin the document contacts", tc.target)
		})
	}

	// The table above is a list, and a list of routes goes stale the moment
	// someone adds a fourth one. Walk the mux and require that every route it
	// answers has a row here, so the omission fails loudly instead of shipping a
	// bare route.
	routes, ok := r.RenderHandler().(chi.Routes)
	require.True(t, ok, "render mux no longer exposes its routes")
	seen := map[string]bool{}
	require.NoError(t, chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		seen[route] = true
		assert.True(t, covered[route], "render route %s %s has no Referrer-Policy row in this test", method, route)
		return nil
	}))
	// And the reverse, so the walk can never pass by finding nothing.
	assert.Len(t, seen, len(covered))
}
