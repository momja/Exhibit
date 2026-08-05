package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderGet exercises the render origin as a browser would: through the real
// RenderHandler mux, not by calling a handler directly, so route wiring is part
// of what these tests cover.
func renderGet(t *testing.T, r *Router, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.RenderHandler().ServeHTTP(w, httptest.NewRequest("GET", target, nil))
	return w
}

// createShare mints a public share row through the API — the single write path,
// like any other client.
func createShare(t *testing.T, r *Router, artifactID string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"artifact_id": artifactID, "public": true})
	req := httptest.NewRequest("POST", "/api/shares", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp["share"].(map[string]any)["id"].(string)
}

// av-c5aq AC#4, and the reason the whole design is a URL token rather than a
// session: NOTHING on the render origin may set a cookie.
//
// A top-level GET /a/:id is not sandboxed. It is a real-origin document with
// the artifact's own script inlined into it, so any cookie scoped to that
// origin is readable by the artifact — which can post it to any origin on its
// allowlist. A session cookie here would be handed to untrusted code on every
// render. This asserts it explicitly because the failure is silent: nothing
// would break, the artifact would simply acquire a credential.
func TestRenderOriginNeverSetsACookie(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)
	shareID := createShare(t, r, id)
	tok := r.tokens.Mint(id, defaultOwnerID)

	// Every route the render mux answers, in both its authorized and its
	// rejected form — a 404 path must not set one either.
	for _, target := range []string{
		"/a/" + id + "?" + rendertoken.Param + "=" + tok,
		"/a/" + id,
		"/a/" + id + "?" + rendertoken.Param + "=bogus",
		"/w/" + id + "?" + rendertoken.Param + "=" + tok,
		"/w/" + id,
		"/s/" + shareID,
		"/s/does-not-exist",
		"/a/does-not-exist",
	} {
		w := renderGet(t, r, target)
		assert.Empty(t, w.Header().Values("Set-Cookie"),
			"render origin set a cookie on %s — the artifact running there could read it", target)
	}
}

// av-c5aq AC#5. A share is authorized by its row, not by a principal, so it
// takes no token and no credentials — that is what lets a link work for someone
// with no account. This ticket must not have narrowed that.
func TestShareRendersWithNoTokenAndNoCredentials(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             "Run Log",
		"body":              "<html><head></head><body>SHARED-BODY</body></html>",
		"network_allowlist": []string{},
	})
	shareID := createShare(t, r, id)

	w := renderGet(t, r, "/s/"+shareID)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "SHARED-BODY")
	// No Authorization header was sent, no cookie comes back, and the URL
	// carries no token: the row was the whole authorization.
	assert.Empty(t, w.Header().Values("Set-Cookie"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "default-src 'none'")
}

// av-c5aq AC#1 at the routing layer: the id alone no longer renders anything.
// The render-package tests cover the handler's reasoning; this one proves the
// mux does not have a side door.
func TestRenderRoutesRequireATokenEndToEnd(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             "Run Log",
		"body":              "<html><head></head><body>PRIVATE-BODY</body></html>",
		"network_allowlist": []string{},
	})
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)

	for _, target := range []string{"/a/" + id, "/w/" + id} {
		w := renderGet(t, r, target)
		assert.Equal(t, http.StatusNotFound, w.Code, target)
		assert.NotContains(t, w.Body.String(), "PRIVATE-BODY", target)
	}

	// And with the token the app origin would have minted, both render.
	tok := r.tokens.Mint(id, defaultOwnerID)
	for _, target := range []string{"/a/" + id, "/w/" + id} {
		w := renderGet(t, r, target+"?"+rendertoken.Param+"="+tok)
		assert.Equal(t, http.StatusOK, w.Code, target)
	}
}

// av-c5aq AC#6. Every surface that embeds a render frame has to mint for it,
// and it has to do so during its own render — a gallery of N cards that cost N
// round trips would make the library unusable at the size it is built for.
func TestEveryEmbeddingSurfaceMintsItsFramesInOnePass(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)

	for _, tc := range []struct{ name, path, wantFrame string }{
		{"gallery card", "/", "http://render.test/w/" + id + "?t="},
		{"artifact detail", "/artifacts/" + id, "http://render.test/a/" + id + "?t="},
		{"edit page widget panel", "/artifacts/" + id + "/edit", "http://render.test/w/" + id + "?t="},
		{"agent preview pane", "/agent?artifact=" + id, "http://render.test/a/" + id + "?t="},
		{"card-widget fragment", "/partials/card-widget?artifact=" + id, "http://render.test/w/" + id + "?t="},
		{"agent-preview fragment", "/partials/agent-preview?artifact=" + id, "http://render.test/a/" + id + "?t="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Contains(t, getPage(t, r, tc.path), tc.wantFrame)
		})
	}

	// One page render, many cards, one signing key already in memory: the
	// gallery emits a distinct token per card without touching the store again.
	for i := 0; i < 5; i++ {
		other := createTestArtifact(t, r, "Extra")
		require.Equal(t, http.StatusOK, putWidgetReq(t, r, other, "<b>x</b>").Code)
	}
	gallery := getPage(t, r, "/")
	assert.Equal(t, 6, strings.Count(gallery, "http://render.test/w/"),
		"every card with a widget should carry its own minted frame URL")
}

// A token minted by the app origin for one artifact must not render another,
// end to end. The scope is the signature (the artifact id is mixed into the
// MAC), so this is structural rather than a comparison to remember.
func TestMintedTokenIsScopedToOneArtifact(t *testing.T) {
	r := newTestRouter(t)
	mine := createTestArtifact(t, r, "Mine")
	other := createTestArtifact(t, r, "Other")

	tok := r.tokens.Mint(mine, defaultOwnerID)

	assert.Equal(t, http.StatusOK, renderGet(t, r, "/a/"+mine+"?"+rendertoken.Param+"="+tok).Code)
	assert.Equal(t, http.StatusNotFound, renderGet(t, r, "/a/"+other+"?"+rendertoken.Param+"="+tok).Code)
}

// "Open in new tab" is a link, not a frame: it may be clicked long after the
// page was rendered, so it carries no token in the markup and mints one on the
// redirect instead. The redirect itself must not be cacheable — it hands out a
// credential with a deadline.
func TestOpenArtifactMintsOnRedirect(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	// The page links at the app origin, with no credential in the href.
	page := getPage(t, r, "/artifacts/"+id)
	assert.Contains(t, page, `href="/artifacts/`+id+`/open"`)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/"+id+"/open", nil))

	require.Equal(t, http.StatusFound, w.Code)
	loc := w.Header().Get("Location")
	assert.True(t, strings.HasPrefix(loc, "http://render.test/a/"+id+"?"+rendertoken.Param+"="), loc)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	// And the minted token actually works on the render origin.
	assert.Equal(t, http.StatusOK, renderGet(t, r, strings.TrimPrefix(loc, "http://render.test")).Code)
}

// An unknown artifact gets the app's 404 page rather than a redirect that would
// bounce the visitor to a render URL for something that isn't there.
func TestOpenArtifactUnknownIsNotFound(t *testing.T) {
	r := newTestRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/artifacts/does-not-exist/open", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
