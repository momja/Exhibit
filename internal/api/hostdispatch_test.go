package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-xath. Normally the port separates the two surfaces and the operator's
// proxy maps a hostname to each. Under SINGLE_LISTENER the port is gone as a
// discriminator and the Host header is all that is left, so these tests are
// what stands between "one listener" and "one origin" — and the difference
// between those two is the artifact sandbox boundary (architecture.md §3.2,
// §4). A dispatcher that quietly sent app routes to the render origin would
// pass every other test in this package while putting /api/* on the origin
// where artifact code runs.

// dispatchTo issues one request through a dispatcher as if it arrived for the
// given hostname.
func dispatchTo(h http.Handler, method, host, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// servedByRender reports which mux answered. `Referrer-Policy: no-referrer` is
// set by render.NoReferrer (av-nr0p) and by nothing on the app surface, so it
// is an oracle for the handler rather than a guess from the status code —
// which matters because both muxes answer 404 and only one of them may.
func servedByRender(w *httptest.ResponseRecorder) bool {
	return w.Header().Get("Referrer-Policy") == "no-referrer"
}

func TestHostDispatcherChoosesSurfaceByHost(t *testing.T) {
	marker := func(s string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(s))
		})
	}
	h, err := NewHostDispatcher(marker("APP"), marker("RENDER"),
		"https://exhibit.example.com", "https://artifacts.example.com")
	require.NoError(t, err)

	cases := []struct {
		name, host, want string
	}{
		{"the render origin's host", "artifacts.example.com", "RENDER"},
		{"render host carrying a port", "artifacts.example.com:443", "RENDER"},
		{"render host in mixed case", "ArTiFaCtS.Example.COM", "RENDER"},
		{"the app origin's host", "exhibit.example.com", "APP"},
		// Everything unrecognized is the app's, and these three are why. A
		// platform assigns its own hostname, health checks and probes arrive
		// by container IP, and an HTTP/1.0 client may send no Host at all.
		// None of them should be handed the render surface.
		{"the platform's own hostname", "exhibit-prod.fly.dev", "APP"},
		{"a bare address", "203.0.113.7:8080", "APP"},
		{"no Host at all", "", "APP"},
		// Matching is exact, not a suffix: a subdomain of the render host is a
		// different origin and gets no artifacts.
		{"a subdomain of the render host", "evil.artifacts.example.com", "APP"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := dispatchTo(h, http.MethodGet, tc.host, "/anything")
			assert.Equal(t, tc.want, w.Body.String())
		})
	}
}

func TestHostDispatcherRefusesACollapsedBoundary(t *testing.T) {
	nothing := http.NotFoundHandler()
	cases := []struct {
		name, appOrigin, renderOrigin string
	}{
		{"one hostname for both", "https://exhibit.example.com", "https://exhibit.example.com"},
		// Differing only by port is two real origins, and still refused: with
		// one listener the port cannot distinguish them, so there is no way to
		// serve both here.
		{"same host, different port", "https://exhibit.example.com", "https://exhibit.example.com:8443"},
		{"same host, different scheme", "http://exhibit.example.com", "https://exhibit.example.com"},
		{"same host, different case", "https://Exhibit.example.com", "https://exhibit.example.com"},
		// A bare hostname parses as a path with no host, which would leave the
		// discriminator empty and match nothing — silently sending every
		// request to the app surface.
		{"render origin with no scheme", "https://exhibit.example.com", "artifacts.example.com"},
		{"app origin with no scheme", "exhibit.example.com", "https://artifacts.example.com"},
		// Unset is the case a platform deployment actually hits: the committed
		// fly.toml carries no origins, so a forgotten `fly secrets set` must
		// stop the boot rather than fall back to localhost.
		{"render origin unset", "https://exhibit.example.com", ""},
		{"app origin unset", "", "https://artifacts.example.com"},
		{"both unset", "", ""},
		{"origin is only whitespace", "https://exhibit.example.com", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHostDispatcher(nothing, nothing, tc.appOrigin, tc.renderOrigin)
			assert.Error(t, err, "started with a collapsed or unusable origin boundary")
		})
	}
}

var routeParam = regexp.MustCompile(`\{[^}]*\}`)

// concretePath turns a chi pattern into a requestable path. The ids are
// deliberately nonsense: these assertions are about which mux answers, and a
// route that 404s inside the right mux has still been dispatched correctly.
func concretePath(route string) string {
	p := routeParam.ReplaceAllString(route, "x")
	p = strings.ReplaceAll(p, "/*", "/x")
	if p == "" {
		p = "/"
	}
	return p
}

func TestHostDispatcherKeepsTheOriginBoundary(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)
	shareID := createShare(t, r, id)
	tok := r.tokens.Mint(id, defaultOwnerID)

	h, err := NewHostDispatcher(r, r.RenderHandler(), "http://app.test", "http://render.test")
	require.NoError(t, err)

	// Non-vacuity first. Every assertion below is of the form "the other
	// surface did not answer", which a dispatcher that answered nothing at all
	// would satisfy perfectly.
	t.Run("each surface answers on its own host", func(t *testing.T) {
		app := dispatchTo(h, http.MethodGet, "app.test", "/")
		require.Equal(t, http.StatusOK, app.Code)
		require.False(t, servedByRender(app))

		render := dispatchTo(h, http.MethodGet, "render.test", "/a/"+id+"?"+rendertoken.Param+"="+tok)
		require.Equal(t, http.StatusOK, render.Code)
		require.True(t, servedByRender(render))
		// The whole point of the render surface, and the thing a collapsed
		// boundary would take away.
		require.NotEmpty(t, render.Header().Get("Content-Security-Policy"))
	})

	// /s/{shareID} is the one path both muxes claim — the app redirects to the
	// render origin, the render surface serves the artifact. It is the sharpest
	// available proof that dispatch happens at all, since path routing alone
	// cannot tell these two apart.
	t.Run("the shared path resolves per origin", func(t *testing.T) {
		app := dispatchTo(h, http.MethodGet, "app.test", "/s/"+shareID)
		assert.Equal(t, http.StatusFound, app.Code)
		assert.Equal(t, "http://render.test/s/"+shareID, app.Header().Get("Location"))
		assert.False(t, servedByRender(app))

		render := dispatchTo(h, http.MethodGet, "render.test", "/s/"+shareID)
		assert.Equal(t, http.StatusOK, render.Code)
		assert.True(t, servedByRender(render))
	})

	// Both walks below are walks rather than lists for the reason every other
	// route walk in this package is one (csrf_test.go, pageowner_test.go): the
	// failure that matters is a route added later by someone who never read
	// this file.
	t.Run("no app route answers on the render origin", func(t *testing.T) {
		var walked int
		require.NoError(t, chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.TrimSuffix(route, "/") == "/s/{shareID}" {
				return nil // legitimately answered by both, asserted above
			}
			walked++
			w := dispatchTo(h, method, "render.test", concretePath(route))
			assert.True(t, servedByRender(w),
				"the app surface answered %s %s on the render origin", method, route)
			return nil
		}))
		assert.Greater(t, walked, 20, "the walk found almost no app routes")
	})

	t.Run("no render route answers on the app origin", func(t *testing.T) {
		routes, ok := r.RenderHandler().(chi.Routes)
		require.True(t, ok, "render mux no longer exposes its routes")
		var walked int
		require.NoError(t, chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if strings.TrimSuffix(route, "/") == "/s/{shareID}" {
				return nil
			}
			walked++
			w := dispatchTo(h, method, "app.test", concretePath(route))
			assert.False(t, servedByRender(w),
				"the render surface answered %s %s on the app origin", method, route)
			return nil
		}))
		assert.Equal(t, 2, walked, "expected /a/{id} and /w/{id}")
	})
}
