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

	// Everything but the per-render cache-busting stamp is identical, and the
	// pane's contents in the page are exactly the fragment.
	assert.Contains(t, stripStamp(page), stripStamp(fragment))
	assert.Contains(t, fragment, `<span class="title" id="pv-title">Preview me</span>`)
	assert.Contains(t, fragment, `href="http://render.test/a/`+id+`"`)
	assert.Contains(t, fragment, `href="/artifacts/`+id+`"`)
}

// The swap only shows the agent's new body if the iframe's src changed: the
// render document is Cache-Control: no-store, but a browser never re-requests
// an unchanged src at all. Each render therefore carries a fresh stamp.
func TestAgentPreviewFrameURLIsStampedPerRender(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{
		"title":             "Stamped",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})

	first := frameSrc(t, getPage(t, r, "/partials/agent-preview?artifact="+id))
	second := frameSrc(t, getPage(t, r, "/partials/agent-preview?artifact="+id))

	assert.True(t, strings.HasPrefix(first, "http://render.test/a/"+id+"?r="), first)
	assert.NotEqual(t, first, second, "two renders produced the same iframe src, so the swap would not reload the artifact")
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

var stampPattern = regexp.MustCompile(`\?r=\d+`)

// stripStamp removes the per-render cache-buster so two renders of the same
// artifact compare equal.
func stripStamp(s string) string { return stampPattern.ReplaceAllString(s, "?r=STAMP") }

var framePattern = regexp.MustCompile(`<iframe id="pv-frame" src="([^"]*)"`)

func frameSrc(t *testing.T, body string) string {
	t.Helper()
	m := framePattern.FindStringSubmatch(body)
	require.Len(t, m, 2, "no preview iframe in:\n%s", body)
	return m[1]
}
