package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-6m3e: the agent page's preview pane and the /partials/agent-preview
// fragment must be the same markup, because the fragment is what replaces the
// pane after every agent save. If the two ever drifted, an artifact would look
// one way on load and another way one save later.
func TestAgentPreviewFragmentMatchesThePagesPane(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             "Preview me",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})

	page := getPage(t, r, "/agent?artifact="+id)
	fragment := getPage(t, r, "/partials/agent-preview?artifact="+id)

	// Everything but the two deliberately per-render values — the cache-busting
	// stamp and the freshly minted render token — is identical, and the pane's
	// contents in the page are exactly the fragment.
	assert.Contains(t, normalizePerRender(page), normalizePerRender(fragment))
	assert.Contains(t, fragment, `<span class="title" id="pv-title">Preview me</span>`)
	// "Open" is an app-origin link that mints its token when it is clicked, so
	// it carries no credential in the markup at all (av-c5aq).
	assert.Contains(t, fragment, `href="/artifacts/`+id+`/open"`)
	assert.Contains(t, fragment, `href="/artifacts/`+id+`"`)
}

// The swap only shows the agent's new body if the iframe's src changed: the
// render document is Cache-Control: no-store, but a browser never re-requests
// an unchanged src at all. Each render therefore carries a fresh stamp.
//
// The stamp is what this asserts uniqueness on, deliberately. A render token is
// also minted per render, but its expiry has one-second resolution and it
// carries no nonce, so two renders in the same second produce byte-identical
// tokens — a cache-buster that only works when the machine is slow is not one.
func TestAgentPreviewFrameURLIsStampedPerRender(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             "Stamped",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})

	first := frameSrc(t, getPage(t, r, "/partials/agent-preview?artifact="+id))
	second := frameSrc(t, getPage(t, r, "/partials/agent-preview?artifact="+id))

	// The token leads, because every render URL carries one; the stamp is the
	// optional extra some call sites append (av-c5aq).
	assert.True(t, strings.HasPrefix(first, "http://render.test/a/"+id+"?t="), first)
	assert.Contains(t, first, "&amp;r=", "the stamp follows the token, escaped by html/template")
	assert.NotEqual(t, stripToken(first), stripToken(second),
		"two renders produced the same stamp, so the swap would not reload the artifact")
}

// Without an artifact the pane holds the empty state and a disabled snippet
// button — never an iframe pointed at nothing.
func TestAgentPreviewWithoutArtifactRendersEmptyState(t *testing.T) {
	r := newTestRouter(t)

	for _, path := range []string{"/agent", "/partials/agent-preview"} {
		body := getPage(t, r, path)
		assert.Contains(t, body, `id="empty-preview"`, path)
		assert.NotContains(t, body, `id="pv-frame"`, path)
		assert.Contains(t, body, `id="snip-btn" disabled`, path)
	}
}

// An unknown artifact answers with a plain 404 rather than an empty-state
// fragment: htmx leaves the target alone on an error response, so the visitor
// keeps the preview they had instead of watching it blank out.
func TestAgentPreviewUnknownArtifactIsNotFound(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/partials/agent-preview?artifact=does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "empty-preview")
}

// The page must carry the wiring that turns a save into a swap: htmx served
// from our own origin (never a CDN), and a pane that fetches the fragment on
// the artifact-saved event agent.js dispatches.
func TestAgentPageWiresPreviewSwapToHtmx(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/agent")

	assert.Contains(t, page, `<script src="/assets/htmx/htmx.min.js"></script>`)
	assert.NotContains(t, page, "unpkg.com")
	assert.Contains(t, page, `hx-get="/partials/agent-preview"`)
	assert.Contains(t, page, `hx-trigger="exhibit:artifact-saved from:body"`)
	assert.Contains(t, page, `hx-swap="innerHTML"`)

	// And the asset is actually embedded, not just referenced.
	req := httptest.NewRequest("GET", "/assets/htmx/htmx.min.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "htmx")
}

// A title is artifact-authored text; it reaches the fragment through
// html/template's contextual escaping, so markup in it stays inert.
func TestAgentPreviewEscapesArtifactTitle(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             `<img src=x onerror=alert(1)>`,
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})

	fragment := getPage(t, r, "/partials/agent-preview?artifact="+id)
	assert.NotContains(t, fragment, "<img src=x")
	assert.Contains(t, fragment, "&lt;img src=x")
}

var (
	// The stamp trails the token in an href/src attribute, where html/template
	// escapes the separating '&' — so both spellings have to match.
	stampPattern = regexp.MustCompile(`(?:\?|&amp;|&)r=\d+`)
	// A render token is "<owner>.<exp>.<base64url mac>" (rendertoken), and all
	// three parts move between renders.
	tokenPattern = regexp.MustCompile(`t=\d+\.\d+\.[A-Za-z0-9_-]+`)
)

// stripToken blanks the render token so two URLs compare on everything else.
func stripToken(s string) string { return tokenPattern.ReplaceAllString(s, "t=TOKEN") }

// normalizePerRender blanks both values that are per-render by design — the
// cache-busting stamp and the minted render token — so two renders of the same
// artifact compare equal on the markup that is supposed to be identical.
func normalizePerRender(s string) string {
	return stripToken(stampPattern.ReplaceAllString(s, "&r=STAMP"))
}

var framePattern = regexp.MustCompile(`<iframe id="pv-frame" src="([^"]*)"`)

func frameSrc(t *testing.T, body string) string {
	t.Helper()
	m := framePattern.FindStringSubmatch(body)
	require.Len(t, m, 2, "no preview iframe in:\n%s", body)
	return m[1]
}
