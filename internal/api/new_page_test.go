package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getPage issues a GET for one of the app-origin HTML pages and returns its
// body, asserting only that the page rendered at all.
func getPage(t *testing.T, r *Router, path string) string {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, path)
	return w.Body.String()
}

// av-qo0j: /new is a real server-rendered page carrying the three routes in.
// Paste HTML and From URL are the ingest panel's two modes (paste selected by
// default); Build with agent is a plain link to the agent surface, which owns
// its own page.
func TestNewPageOffersThreeRoutes(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/new")

	assert.Contains(t, page, "<title>Add artifact — Exhibit</title>")
	assert.Contains(t, page, `<h2>Three ways in.</h2>`)

	// The two panel modes carry data-mode; paste starts selected.
	assert.Contains(t, page, `<button type="button" class="route is-selected" data-mode="paste" aria-pressed="true" onclick="setMode('paste')">`)
	assert.Contains(t, page, `<button type="button" class="route" data-mode="url" aria-pressed="false" onclick="setMode('url')">`)
	// The agent tile is a link, not a mode: it carries no data-mode, so the
	// mode switch can never select it.
	assert.Contains(t, page, `<a class="route" href="/agent">`)
	assert.Contains(t, page, `<b>Build with agent</b>`)

	// Sub-page header idiom (detail.tmpl): back link, then page title — not a
	// Library/Studio segmented nav.
	assert.Contains(t, page, `<a href="/">←<span class="back-label"> Gallery</span></a>`)
	assert.Contains(t, page, `<h1>Add artifact</h1>`)
}

// The ingest panel is the whole ingest form the gallery index used to carry:
// title, source, url, snapshot toggle, scan result, status, and the submit.
func TestNewPageCarriesTheIngestPanel(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/new")

	assert.Contains(t, page, `<input class="field" type="text" id="title"`)
	assert.Contains(t, page, `<textarea id="body" placeholder=`)
	assert.Contains(t, page, `<input class="field" type="text" id="url-input"`)
	assert.Contains(t, page, `<input type="checkbox" id="snapshot-toggle" checked>`)
	assert.Contains(t, page, `<div id="scan-result"></div>`)
	assert.Contains(t, page, `<div id="status"></div>`)
	assert.Contains(t, page, `onclick="ingest()"`)
	assert.Contains(t, page, `Add to library`)
	// Cancel goes back to the library, which is also where a finished ingest
	// leaves from.
	assert.Contains(t, page, `<a class="btn btn-sec" href="/">Cancel</a>`)

	// The page posts through the API like any other client, so it needs the
	// bearer token and nothing else from the server.
	assert.Contains(t, page, `const TOKEN = `)
}

// The snapshot toggle is URL-mode-only and must stay that way: the API
// rejects `snapshot` without a `url` (artifacts.go, 400 "snapshot requires a
// source url") because the vendorer needs an absolute http(s) base. The
// markup ships it hidden, the mode switch only reveals it for url, and
// ingest() reads the checkbox only on the url branch.
func TestNewPageSnapshotToggleIsURLModeOnly(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/new")

	assert.Contains(t, page, `<label class="snapshot-row" id="snapshot-row" hidden>`,
		"the snapshot row must start hidden, since paste is the default mode")

	js := galleryAsset(t, r, "/assets/gallery/new.js")
	assert.Contains(t, js, `document.getElementById('snapshot-row').hidden = mode !== 'url';`)
	assert.Contains(t, js, `payload = {title: title || 'Untitled', body, network_allowlist: []};`,
		"the paste payload must never carry a snapshot field")
}

// Ingest is unchanged by the move: persist first (the artifact is stored
// network-inert), surface the scanned footprint for explicit approval, then
// PATCH only the origins the user selected.
func TestNewPageKeepsApproveThenPatchFlow(t *testing.T) {
	r := newTestRouter(t)
	js := galleryAsset(t, r, "/assets/gallery/new.js")

	assert.Contains(t, js, `await apiFetch('/api/artifacts', {`)
	assert.Contains(t, js, `function showApproval(id, footprint)`)
	assert.Contains(t, js, `body: JSON.stringify({network_allowlist: selected})`)
	// Done means gone: the page hands off to the artifact's own detail page.
	assert.Contains(t, js, `location.href = href;`)
	assert.Contains(t, js, `const href = '/artifacts/' + id;`)
}

// av-qo0j: the library index is a library again. Every id the ingest form
// owned is gone from it, and the header's Agent link is replaced by a control
// pointing at the page that now owns creation. (av-qo05 later shrank that
// control to an icon; the claim below is about where it points and what it is
// called, not what it looks like.)
func TestGalleryIndexHasNoIngestMarkup(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/")

	for _, id := range []string{`id="upload"`, `id="body"`, `id="url-input"`, `id="snapshot-row"`, `id="scan-result"`, `id="status"`} {
		assert.NotContains(t, page, id, "ingest markup must not survive on the library index")
	}
	assert.NotContains(t, page, `class="mode-tabs"`)
	assert.NotContains(t, page, `setMode(`)
	assert.NotContains(t, page, `ingest()`)

	assert.Contains(t, page, `href="/new" aria-label="Add artifact" title="Add artifact"`)
	assert.NotContains(t, page, `<a href="/agent">`)

	// The ingest stylesheet went with it.
	css := galleryAsset(t, r, "/assets/gallery/index.css")
	assert.NotContains(t, css, ".upload")
	assert.NotContains(t, css, ".tab-btn")
	assert.NotContains(t, css, "#snapshot-row")

	// So did the ingest script; search, tags and modals stayed.
	js := galleryAsset(t, r, "/assets/gallery/index.js")
	assert.NotContains(t, js, "function ingest(")
	assert.NotContains(t, js, "function setMode(")
	assert.Contains(t, js, "runSearch")
	assert.Contains(t, js, "function openAddTagModal(")
}

// The empty library points at /new rather than at an upload box that is no
// longer above it.
func TestGalleryEmptyStateLinksToNewPage(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/")

	assert.Contains(t, page, `<p class="empty">No artifacts yet. <a href="/new">Add your first artifact</a>.</p>`)
}

// The 404 page's second action used to point at a /#upload fragment on the
// gallery index. That anchor went away with the upload block, so the button
// targets the page that replaced it — and that page must actually resolve.
func TestNotFoundAddArtifactButtonTargetsNewPage(t *testing.T) {
	r := newTestRouter(t)

	w := get404(t, r, "/nowhere")
	require.Equal(t, http.StatusNotFound, w.Code)
	page := w.Body.String()

	assert.NotContains(t, page, `href="/#upload"`)
	assert.Contains(t, page, `<a class="btn btn-sec" href="/new">`)

	req := httptest.NewRequest("GET", "/new", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusOK, w2.Code, "the 404's add-artifact target must not itself 404")
}
