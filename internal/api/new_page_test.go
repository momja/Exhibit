package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
// nw-d1dd made all three panel modes and put Build with agent first, selected
// on load — it is no longer a bare link to /agent.
func TestNewPageOffersThreeRoutes(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/new")

	assert.Contains(t, page, "<title>Add artifact — Exhibit</title>")
	assert.Contains(t, page, `<h2>Three ways in.</h2>`)

	assert.Contains(t, page, `<button type="button" class="route is-selected" data-mode="agent" aria-pressed="true" onclick="setMode('agent')">`)
	assert.Contains(t, page, `<button type="button" class="route" data-mode="paste" aria-pressed="false" onclick="setMode('paste')">`)
	assert.Contains(t, page, `<button type="button" class="route" data-mode="url" aria-pressed="false" onclick="setMode('url')">`)
	assert.Contains(t, page, `<b>Build with agent</b>`)

	// The agent tile used to be an anchor straight to /agent. It is a mode now,
	// so a click selects the brief below rather than leaving the page.
	assert.NotContains(t, page, `<a class="route" href="/agent">`)

	// Sub-page header idiom (detail.tmpl): back link, then page title — not a
	// Library/Studio segmented nav.
	assert.Contains(t, page, `<a href="/">←<span class="back-label"> Gallery</span></a>`)
	assert.Contains(t, page, `<h1>Add artifact</h1>`)
}

// Order is the ticket (nw-d1dd): agent, then paste, then url.
func TestNewPageOrdersAgentFirst(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/new")

	agent := strings.Index(page, `data-mode="agent"`)
	paste := strings.Index(page, `data-mode="paste"`)
	url := strings.Index(page, `data-mode="url"`)
	require.NotEqual(t, -1, agent)
	require.NotEqual(t, -1, paste)
	require.NotEqual(t, -1, url)
	assert.Less(t, agent, paste, "Build with agent leads the routes")
	assert.Less(t, paste, url, "Paste HTML sits between the agent and URL tiles")
}

// The agent route asks for a brief on a form of named fields, not a chat box:
// the fields are what the agent surface builds its opening message from, and
// the set is meant to grow. The description is required; the title is not.
func TestNewPageCarriesTheAgentBriefForm(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/new")

	assert.Contains(t, page, `<form class="card ingest-panel" id="agent-panel" onsubmit="startAgent(event)">`)
	assert.Contains(t, page, `<input class="field" type="text" id="agent-title" name="title"`)
	assert.Contains(t, page, `<textarea id="agent-description" name="description" required`)
	assert.Contains(t, page, `<button class="btn" type="submit"><i class="ph ph-robot"></i> Start building</button>`)

	// The ingest form is the other panel, and it starts hidden: the page opens
	// on the brief.
	assert.Contains(t, page, `<div class="card ingest-panel" id="ingest-panel" hidden>`)
}

// The brief crosses to /agent in sessionStorage, never a query string: it is
// the user's own content, and a URL is copied into this server's request log,
// the operator's proxy log and browser history. agent.js consumes it once.
func TestNewPageHandsTheBriefOverOutOfBandOfTheURL(t *testing.T) {
	r := newTestRouter(t)
	newJS := galleryAsset(t, r, "/assets/gallery/new.js")
	agentJS := galleryAsset(t, r, "/assets/gallery/agent.js")

	assert.Contains(t, newJS, `const BRIEF_KEY = 'exhibit:agent-brief';`)
	assert.Contains(t, newJS, `sessionStorage.setItem(BRIEF_KEY, JSON.stringify(brief));`)
	assert.Contains(t, newJS, `location.href = '/agent';`)
	assert.NotContains(t, newJS, `/agent?`, "the brief must never travel in the URL")

	assert.Contains(t, agentJS, `const BRIEF_KEY = 'exhibit:agent-brief';`)
	assert.Contains(t, agentJS, `sessionStorage.removeItem(BRIEF_KEY);`)
	// One entry per brief field, so a new field on the form is one line here.
	assert.Contains(t, agentJS, `const BRIEF_FIELDS = [`)
	assert.Contains(t, agentJS, `{key: 'description', label: 'What it should do'}`)
}

// Start building sends the brief. The next page is not a second submit button,
// including on the no-key path: there the composer is filled, the key modal
// opens, and saving a key spends the waiting brief.
func TestAgentPageSendsTheBriefItArrivesWith(t *testing.T) {
	r := newTestRouter(t)
	agentJS := galleryAsset(t, r, "/assets/gallery/agent.js")

	assert.Contains(t, agentJS, `if (opening) send();`)
	assert.Contains(t, agentJS, `briefAwaitingKey = !!opening;`)
	assert.Contains(t, agentJS, `if (briefAwaitingKey) {`)
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
		"the snapshot row must start hidden: the page opens on the agent brief, and paste never shows it either")

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
