package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putWidgetReq(t *testing.T, r *Router, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"body": body})
	req := httptest.NewRequest("PUT", "/api/artifacts/"+id+"/widget", bytes.NewReader(payload))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// The widget round-trips through the API — the single write path — and the
// artifact reports carrying one.
func TestWidgetPutGetRoundTrip(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	const widget = "<html><body><b>42 km</b></body></html>"
	w := putWidgetReq(t, r, id, widget)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var put widgetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &put))
	assert.Equal(t, "http://render.test/w/"+id, put.WidgetURL)

	req := httptest.NewRequest("GET", "/api/artifacts/"+id+"/widget", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got widgetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, widget, got.Body)

	// The artifact now points at a widget blob, which is what makes its card
	// render a live tile instead of the default.
	req = httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), `"widget_blob_id":"`)
	assert.NotContains(t, w.Body.String(), `"widget_blob_id":""`)
}

// Re-saving must not mint a new blob: the widget's render URL is embedded in
// gallery cards, so it has to stay stable across edits.
func TestWidgetSaveReusesBlobID(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>v1</b>").Code)
	first := artifactField(t, r, id, "widget_blob_id")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>v2</b>").Code)
	assert.Equal(t, first, artifactField(t, r, id, "widget_blob_id"))
}

// A widget rides the artifact's allowlist, so it gets no approval flow of its
// own — but the origins it references that the allowlist doesn't cover are
// reported, because those are blocked at render and would otherwise show up as
// a mysteriously blank tile. Reporting them must never approve them.
func TestWidgetReportsUnapprovedOriginsWithoutApprovingThem(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	w := putWidgetReq(t, r, id, `<html><body><img src="https://cdn.example.com/x.png"></body></html>`)
	require.Equal(t, http.StatusOK, w.Code)

	var resp widgetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.NetworkFootprint, "https://cdn.example.com")
	assert.Contains(t, resp.Unapproved, "https://cdn.example.com")

	// The allowlist is untouched: a scan never grants network access (spec §6.2).
	assert.Equal(t, "[]", artifactField(t, r, id, "network_allowlist"))
}

// An origin already approved for the artifact is not reported as unapproved —
// the widget inherits that grant rather than needing its own.
func TestWidgetInheritsArtifactAllowlist(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	patch, _ := json.Marshal(map[string]any{"network_allowlist": []string{"https://cdn.example.com"}})
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(patch))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	w = putWidgetReq(t, r, id, `<html><body><img src="https://cdn.example.com/x.png"></body></html>`)
	require.Equal(t, http.StatusOK, w.Code)

	var resp widgetResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Unapproved)
}

// Removing the widget detaches it; the card falls back to the default tile.
func TestWidgetDelete(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>v1</b>").Code)

	req := httptest.NewRequest("DELETE", "/api/artifacts/"+id+"/widget", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	req = httptest.NewRequest("GET", "/api/artifacts/"+id+"/widget", nil)
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Widget writes are mutations, so they pass the same auth boundary as every
// other write — the single write path has no side door.
func TestWidgetRoutesRequireAuth(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	payload, _ := json.Marshal(map[string]string{"body": "<b>x</b>"})
	req := httptest.NewRequest("PUT", "/api/artifacts/"+id+"/widget", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// A card with a widget frames it from the render origin; a card without one
// renders the default tile and loads no frame at all.
func TestGalleryCardRendersWidgetOrDefaultTile(t *testing.T) {
	r := newTestRouter(t)
	withWidget := createTestArtifact(t, r, "Run Log")
	plain := createTestArtifact(t, r, "Mortgage Calculator")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, withWidget, "<b>42 km</b>").Code)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// The tile frame carries a render token minted during this page render
	// (av-c5aq): one HMAC per card, no round trip per card.
	assert.Contains(t, page, `src="http://render.test/w/`+withWidget+`?t=`)
	assert.NotContains(t, page, `http://render.test/w/`+plain)
	// "Mortgage Calculator" -> MC on the default tile.
	assert.Contains(t, page, `<span class="card-widget-monogram">MC</span>`)
	// "Run Log" -> RL, present even though that card HAS a widget: the default
	// tile is always rendered under the frame, which is what lets a failing
	// widget fall back by hiding one element instead of building markup.
	assert.Contains(t, page, `<span class="card-widget-monogram">RL</span>`)
	// The frame must be inert: out of the tab order, and pointer-events are
	// disabled in the stylesheet so a click reaches the card beneath it.
	assert.Contains(t, page, `class="card-widget-frame"`)
	assert.Contains(t, page, `tabindex="-1"`)
}

// The default tile's hue must be stable per artifact — a card that changes
// face between visits is not recognizable.
func TestDefaultTileHueIsStablePerArtifact(t *testing.T) {
	assert.Equal(t, titleHue("abc"), titleHue("abc"))
	assert.NotEqual(t, titleHue("abc"), titleHue("abd"))
	assert.Less(t, titleHue("abc"), 360)
}

func TestMonogram(t *testing.T) {
	cases := map[string]string{
		"Run Log":              "RL",
		"Mortgage Calculator":  "MC",
		"reading-list":         "RL",
		"Budget":               "B",
		"":                     "—",
		"🙂":                    "—",
		"Über Tracker Deluxe":  "ÜT",
		"  leading whitespace": "LW",
	}
	for title, want := range cases {
		assert.Equal(t, want, monogram(title), "monogram(%q)", title)
	}
}

// The edit page's preview swaps in this fragment after a save, so it must be
// the same cardWidget markup the gallery renders — one definition — carrying a
// cache-busting stamp (the browser only refetches a frame whose src changed).
func TestCardWidgetPartialRendersStampedFrame(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)

	req := httptest.NewRequest("GET", "/partials/card-widget?artifact="+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	frag := w.Body.String()
	assert.True(t, strings.HasPrefix(frag, `<div class="card-widget has-frame">`), frag)
	assert.Contains(t, frag, "http://render.test/w/"+id+"?t=")
	assert.Contains(t, frag, "&amp;r=")
	// The monogram ships under the frame so the health watcher's fallback is
	// just a class flip, with no markup for page JS to build.
	assert.Contains(t, frag, `class="card-widget-monogram"`)
}

// artifactField returns one raw JSON field of an artifact, for assertions that
// care about the stored value rather than a typed struct.
func artifactField(t *testing.T, r *Router, id, field string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	return string(raw[field])
}

// The edit page presents its three sections — security, artifact source,
// gallery widget — as peer .details-panel sections sharing one caret partial,
// with only the artifact source open (that being what "Edit" means).
func TestEditPageSectionsAreSymmetricPanels(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	for _, panel := range []string{
		`<details class="details-panel" id="security-panel">`,
		`<details class="details-panel" id="state-panel">`,
		`<details class="details-panel" id="source-panel" open>`,
		`<details class="details-panel" id="widget-panel">`,
	} {
		assert.Contains(t, page, panel)
	}
	// One caret definition, rendered by every summary (panelCaret partial) —
	// security, state (av-hg5f), source, and widget.
	assert.Equal(t, 4, strings.Count(page, `class="ph ph-caret-right details-caret details-caret-closed"`))
	// Both source fields are real textareas the editor island mounts over, so
	// the widget source is not a second-class field.
	assert.Contains(t, page, `<textarea id="body">`)
	assert.Contains(t, page, `<textarea id="widget-src"`)
	assert.Contains(t, page, `<script src="/assets/editor.js"></script>`)
}

// Both source fields get CodeMirror, and it is mounted per panel rather than
// eagerly: a closed <details> is display:none, and CodeMirror measures the DOM
// when constructed, so mounting into a hidden panel yields a mis-sized editor.
func TestEditScriptMountsBothEditorsLazily(t *testing.T) {
	js, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	src := string(js)

	assert.Contains(t, src, `mountEditorWhenOpen('source-panel', 'body')`)
	assert.Contains(t, src, `mountEditorWhenOpen('widget-panel', 'widget-src')`)
	// Mounting is gated on the panel being open, and re-checked on toggle.
	assert.Contains(t, src, `if (editors[textareaID] || !panel.open) return;`)
	assert.Contains(t, src, `panel.addEventListener('toggle', mount);`)
	// Clearing the widget goes through setSource so the mounted editor's
	// document is replaced too — a bare textarea.value assignment would leave
	// the visible document stale (the sync only runs editor -> textarea).
	assert.Contains(t, src, `setSource('widget-src', '')`)
}

// The widget-tile health watcher must not start its deadline until the frame
// is actually allowed to load. A tile in a closed panel or below the fold
// (loading="lazy") has not been fetched at all, and timing it out would show a
// monogram for every healthy widget the visitor hasn't scrolled to yet.
func TestWidgetHealthDeadlineWaitsForVisibility(t *testing.T) {
	js, err := embeddedAssets.ReadFile("assets/gallery/components.js")
	require.NoError(t, err)
	src := string(js)

	assert.Contains(t, src, "IntersectionObserver")
	assert.Contains(t, src, "startDeadline()")
	// The deadline is armed from the observer callback, never at watch() time.
	i := strings.Index(src, "io.observe(frame)")
	require.Greater(t, i, 0)
	assert.Contains(t, src[:i], "if (!entries[i].isIntersecting) continue;")
	// A later 'ready' clears a failed tile rather than only suppressing the timer.
	assert.Contains(t, src, `setFailed(frames[i], d.status !== 'ready');`)
}

// The generate route degrades honestly rather than 500ing: no pi binary is a
// 503, and it says so. (The default test router has no agent manager.)
func TestWidgetGenerateUnavailableWithoutAgent(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/widget/generate", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "pi binary")
}

// Generation starts an agent session, so it is a mutation and sits behind the
// same auth boundary as every other write.
func TestWidgetGenerateRequiresAuth(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/widget/generate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// A missing artifact is a 404 before any agent work is attempted — no session
// is spawned for something that cannot be widgeted.
func TestWidgetGenerateUnknownArtifact(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("POST", "/api/artifacts/nope/widget/generate", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// The button is rendered disabled with its reason rather than hidden: a missing
// affordance is harder to diagnose than one that says what it needs. The test
// router has no agent manager, so that is the reason here.
func TestEditPageGenerateButtonDisabledWithReason(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `id="widget-generate"`)
	assert.Contains(t, page, `disabled title="Agent support is off on this server`)
	// No widget yet, so the button offers to make one rather than replace one.
	assert.Contains(t, page, `<span id="widget-generate-label">Generate widget</span>`)
}

// With a widget already saved the same button reads as a replacement.
func TestEditPageGenerateButtonSaysRegenerateWhenWidgetExists(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run Log")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, id, "<b>42 km</b>").Code)

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Body.String(), `<span id="widget-generate-label">Regenerate</span>`)
}

// The button sends no prompt: the instruction is a server-side constant, so
// there is nothing a caller can inject into the model's context through this
// route. The client must not be building one either.
func TestWidgetGenerateTakesNoCallerPrompt(t *testing.T) {
	js, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	src := string(js)

	i := strings.Index(src, "/widget/generate")
	require.Greater(t, i, 0)
	// The POST carries no body at all — just the auth header.
	assert.NotContains(t, src[i:i+400], "JSON.stringify")
	// Progress rides the session's existing SSE route, not a new mechanism.
	// apiEventSource is that route credentialed for this visitor (av-5imk):
	// a query-string token on a single-user instance, nothing at all when a
	// session cookie is what authenticates the stream.
	assert.Contains(t, src, "apiEventSource('/api/agent/sessions/'")
	assert.Contains(t, src, "'exhibit_widget_saved'")
	// A one-shot session is closed once it has done its job.
	assert.Contains(t, src, "apiFetch('/api/agent/sessions/' + encodeURIComponent(sessionId), {")
}

// A PATCH body may not set widget_blob_id. The id is minted server-side and
// blob ids are global, so a caller who could write this column would repoint
// their own card at any blob id they can name — including the widget of an
// artifact belonging to someone else, which the render surface would then serve
// under their artifact's allowlist. The widget handlers write the column
// through Store.SetWidgetBlobID instead, which is why removing it from the
// PATCH allowlist costs them nothing.
func TestWidgetBlobIDIsNotPatchable(t *testing.T) {
	r := newTestRouter(t)

	victim := createTestArtifact(t, r, "Someone Else's Tool")
	require.Equal(t, http.StatusOK, putWidgetReq(t, r, victim, "<b>private</b>").Code)
	stolen := artifactField(t, r, victim, "widget_blob_id")
	require.NotEmpty(t, stolen)

	id := createTestArtifact(t, r, "Run Log")
	payload, _ := json.Marshal(map[string]string{"widget_blob_id": stolen})
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(payload))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "the column is not caller-writable")

	assert.Equal(t, `""`, artifactField(t, r, id, "widget_blob_id"),
		"and the artifact still has no widget of its own")
}
