package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// galleryAsset fetches one of the static gallery assets (stylesheet or page
// script) through the same embedded-assets route the pages reference. The
// gallery's CSS and JS moved out of the rendered pages into these assets
// (epi-q0u2), so tests that assert on rules or functions read them here.
func galleryAsset(t *testing.T, r *Router, path string) string {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, path)
	return w.Body.String()
}

func TestGalleryIndexRendersTagDots(t *testing.T) {
	r := newTestRouter(t)

	white := createTestTag(t, r, "charts", "#FFFFFF")
	dark := createTestTag(t, r, "urgent", "#111111")
	id := createTestArtifact(t, r, "Tagged")

	for _, tag := range []struct{ id string }{{white.ID}, {dark.ID}} {
		w := doJSON(t, r, "POST", "/api/tags/"+tag.id+"/artifacts/"+id, nil)
		require.Equal(t, http.StatusNoContent, w.Code)
	}

	untaggedID := createTestArtifact(t, r, "Untagged")
	page := getPage(t, r, "/")

	// av-uvc6: a tag on a card is its color alone. The name is not drawn,
	// but it is the dot's tooltip and the screen reader's text, so the
	// list still reads as tag names to anyone who cannot see the colors.
	assert.Contains(t, page, `<ul class="tag-dots" aria-label="Tags">`)
	assert.Contains(t, page, `<li class="tag-dot-item" data-tag-id="`+white.ID+`" style="--tag-color:#ffffff" title="charts"><span class="sr-only">charts</span></li>`)
	assert.Contains(t, page, `<li class="tag-dot-item" data-tag-id="`+dark.ID+`" style="--tag-color:#111111" title="urgent"><span class="sr-only">urgent</span></li>`)
	// The pill row and its visible label are gone from the card.
	assert.NotContains(t, page, `class="tag-pill`)
	assert.NotContains(t, page, `tag-pill-label`)

	// Untagged card: no dot list and no add control.
	assert.Equal(t, 1, strings.Count(page, `<ul class="tag-dots"`), "only the tagged card renders a dot list")
	assert.Contains(t, page, `/artifacts/`+untaggedID+`/edit`)
	assert.NotContains(t, page, `tag-add-btn`)

	// A white tag must not vanish into the white card: the dot carries an
	// inset ring whatever its color. The name stays reachable as sr-only
	// text, which components.css defines for every page.
	css := galleryAsset(t, r, "/assets/gallery/index.css")
	assert.Contains(t, css, `.tag-dot-item::before{content:"";width:7px;height:7px;border-radius:50%;background:var(--tag-color,#888);box-shadow:inset 0 0 0 1px rgba(0,0,0,.12)}`)
	assert.Contains(t, galleryAsset(t, r, "/assets/gallery/components.css"), `.sr-only{position:absolute;width:1px;height:1px;`)
}

// Gallery tags are static: dots only, no controls, modals or tag writes.
func TestGalleryTagsAreStatic(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "charts", "#FFFFFF")
	id := createTestArtifact(t, r, "Tagged")

	w := doJSON(t, r, "POST", "/api/tags/"+tag.ID+"/artifacts/"+id, nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	req := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	assert.Contains(t, page, `<li class="tag-dot-item" data-tag-id="`+tag.ID+`" style="--tag-color:#ffffff" title="charts"><span class="sr-only">charts</span></li>`)
	for _, gone := range []string{"tag-pill-edit", "tag-pill-detach", "tag-add-btn", "tag-edit-modal", "tag-add-modal", "DEFAULT_TAG_COLOR"} {
		assert.NotContains(t, page, gone)
	}

	js := galleryAsset(t, r, "/assets/gallery/index.js")
	assert.NotContains(t, js, "/api/tags")
	assert.NotContains(t, js, "function openAddTagModal(")
	assert.NotContains(t, js, "function openEditTagModal(")
}

// The edit page's Tags panel: per-tag controls, an add dropdown of unattached
// tags, the edit-tag modal, and an htmx-refreshed body.
func TestEditPageRendersTagsPanel(t *testing.T) {
	r := newTestRouter(t)
	charts := createTestTag(t, r, "charts", "#FFFFFF")
	other := createTestTag(t, r, "other", "")
	id := createTestArtifact(t, r, "Tagged")
	w := doJSON(t, r, "POST", "/api/tags/"+charts.ID+"/artifacts/"+id, nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	assert.Contains(t, page, `<details class="details-panel" id="tags-panel">`)
	assert.Contains(t, page, `hx-get="/partials/tag-panel?artifact=`+id+`"`)
	assert.Contains(t, page, `hx-trigger="exhibit:tags-changed from:body"`)
	assert.Contains(t, page, `data-action="edit-tag" data-tag-id="`+charts.ID+`" data-tag-name="charts" data-tag-color="#ffffff"`)
	assert.Contains(t, page, `data-action="detach-tag" data-tag-id="`+charts.ID+`" aria-label="Remove tag charts from this artifact"`)
	// The dropdown offers what is not yet attached.
	assert.Contains(t, page, `<option value="`+other.ID+`">other</option>`)
	assert.NotContains(t, page, `<option value="`+charts.ID+`">`)
	assert.Contains(t, page, `<option value="__new__">+ Create new tag</option>`)
	assert.Contains(t, page, `id="tag-add-color-hex" class="field" value="#6b7280"`)
	// The library-wide edit dialog, and the script that drives both.
	assert.Contains(t, page, `<div id="tag-edit-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `data-color="#6B7280"`) // store.DefaultTagColor preset
	assert.Contains(t, page, `id="tag-edit-delete"`)
	assert.Contains(t, page, `<script src="/assets/gallery/tags.js"></script>`)
	assert.Contains(t, galleryAsset(t, r, "/assets/gallery/tags.js"), "exhibit:tags-changed")
}

// /partials/tag-panel reflects current tags and 404s for a missing artifact.
func TestTagPanelPartial(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "charts", "")
	id := createTestArtifact(t, r, "Tagged")

	get := func(artifact string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/partials/tag-panel?artifact="+artifact, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	w := get(id)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "This artifact has no tags.")
	assert.Contains(t, w.Body.String(), `<option value="`+tag.ID+`">charts</option>`)

	require.Equal(t, http.StatusNoContent, doJSON(t, r, "POST", "/api/tags/"+tag.ID+"/artifacts/"+id, nil).Code)
	w = get(id)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `data-action="detach-tag" data-tag-id="`+tag.ID+`"`)
	assert.NotContains(t, w.Body.String(), `<option value="`+tag.ID+`">`)

	missing := get("no-such-artifact")
	assert.Equal(t, http.StatusNotFound, missing.Code)
	assert.Contains(t, missing.Body.String(), `class="frag-error"`)
}

// A store failure behind a fragment answers 500 with the generic error
// notice, never the internal error string: htmx swaps it into the pane, so
// internals must not reach the body (av-3cdp).
func TestFragmentServerErrorHidesInternals(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/partials/tag-panel?artifact=x", nil)
	fragmentServerError(w, r, "test label", errors.New("sqlite: disk I/O error"))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	body := w.Body.String()
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, body, `class="frag-error"`)
	assert.Contains(t, body, "Something went wrong.")
	assert.NotContains(t, body, "disk I/O error")
}

// Search filters eagerly as the user types: an inline input with a debounce
// + fetch + grid-swap script, no form submit and no Search button.
func TestGallerySearchIsEagerInput(t *testing.T) {
	r := newTestRouter(t)
	createTestArtifact(t, r, "Findable")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `<input type="text" id="search-input" name="q"`)
	assert.Contains(t, page, `placeholder="Search artifacts…"`)
	// debounce + fetch + grid-swap script lives in the static page script
	assert.Contains(t, galleryAsset(t, r, "/assets/gallery/index.js"), `runSearch`)
	assert.NotContains(t, page, `type="submit"`)
	assert.NotContains(t, page, `>Search</button>`)
}

// `/` focuses search (av-6mdw). The key's behavior is index.search.test.mjs's
// to prove; this is the markup half the node harness cannot see. The key cap
// has to come after the input, because index.css hides it with sibling
// selectors off the input's focus and placeholder state, and a cap placed
// before the input would stay on screen over the query.
func TestGallerySearchAdvertisesSlashShortcut(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `aria-keyshortcuts="/"`, "screen readers learn the shortcut from the input")
	kbd := `<kbd class="search-kbd" aria-hidden="true">/</kbd>`
	require.Contains(t, page, kbd)
	input := strings.Index(page, `id="search-input"`)
	require.GreaterOrEqual(t, input, 0)
	assert.Greater(t, strings.Index(page, kbd), input, "the key cap must be the input's later sibling")
}

// The exhibit header must read as distinct from the white content cards below
// it: it is sticky (stays visible while scrolling) and carries a real shadow +
// stronger border rather than the same near-invisible hairline the cards use.
func TestGalleryHeaderHasVisualSeparation(t *testing.T) {
	r := newTestRouter(t)

	// The header must be visually distinct from the white content cards below
	// it: it is sticky (stays visible while scrolling) and carries a real
	// shadow + stronger border rather than the same near-invisible hairline
	// the cards use. A bare 1px #e0e0e0 border alone read as flush with content.
	css := galleryAsset(t, r, "/assets/gallery/index.css")
	assert.Contains(t, css, `header{position:sticky;top:0;z-index:20`)
	assert.Contains(t, css, `box-shadow:0 1px 6px rgba(0,0,0,.07)`) // grep-friendly: 'box-shadow'
}

// The artifact card's open affordances are the card body (click anywhere
// non-interactive) and the title link — both go to the detail/viewer page.
// The 'Details' link was removed: it navigated to the SAME page as the title
// click, so it was redundant. The earlier 'Open ↗' new-tab action was already
// gone. There must be no open-in-new-tab affordance and the card must carry a
// click target so any non-interactive part of it navigates.
func TestGalleryCardHasNoRedundantDetailsLink(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Openless")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// The card opens the artifact's detail/viewer page from anywhere that
	// isn't an interactive child; the data-href is what the click handler uses.
	assert.Contains(t, page, `<div class="card" data-href="/artifacts/`+id+`">`)

	// The title link is the single named way into the artifact's detail page.
	assert.Contains(t, page, `<a class="card-title" href="/artifacts/`+id+`">Openless</a>`)

	// The redundant 'Details' link is gone (it duplicated the title click).
	assert.NotContains(t, page, `>Details</a>`)
	assert.NotContains(t, page, `class="card-actions"`)

	// av-8u7p, av-uvc6: the one card action that is not the detail page, a
	// pencil at the end of the meta row. Its accessible name leads with
	// "Edit" and names the artifact; the title is the word a pointer sees.
	assert.Contains(t, page, `<a class="card-edit" href="/artifacts/`+id+`/edit" aria-label="Edit Openless" title="Edit"><i class="ph ph-pencil-simple" aria-hidden="true"></i></a>`)

	// av-uvc6: one meta row under the tile, and no created date on it.
	assert.Contains(t, page, `<div class="card-meta"><a class="card-title" href="/artifacts/`+id+`">Openless</a>`)
	assert.NotContains(t, page, `card-footer`)
	assert.NotContains(t, page, time.Now().Format("Jan 2, 2006"))

	// The removed 'Open ↗' action and any new-tab opener are gone from cards.
	assert.NotContains(t, page, "Open ↗")
	assert.NotContains(t, page, `target="_blank"`)
}

// av-8u7p, av-uvc6: the card's Edit pencil is hover-revealed, but
// 'hover-revealed' must not mean 'unreachable'. Keyboard focus reveals it,
// coarse pointers show it unconditionally, and at rest it stays in the
// accessibility tree rather than being display:none'd out of it.
func TestGalleryCardEditStaysReachableWithoutHover(t *testing.T) {
	r := newTestRouter(t)
	css := galleryAsset(t, r, "/assets/gallery/index.css")

	// Keyboard: focus anywhere within the card reveals the pencil, so it is
	// not a hover-only control for anyone tabbing through the grid.
	assert.Contains(t, css, `.card:hover .card-edit,.card:focus-within .card-edit{width:22px;margin:0 -4px 0 -2px;opacity:1}`)

	// Touch: hover does not exist, so the pencil is always visible there.
	// Both queries are load-bearing: a hover-less input does not reliably
	// report hover:none (Chrome's touch emulation reports neither
	// hover:none nor hover:hover), and the coarse pointer identifies those.
	assert.Contains(t, css, `@media (hover:none),(pointer:coarse){.card-edit{width:22px;margin:0 -4px 0 -2px;opacity:1}}`)

	// At rest it is collapsed, not removed: never display:none or
	// visibility:hidden, either of which would drop the link from a screen
	// reader's list of links.
	assert.Contains(t, css, `.card-edit{flex:0 0 auto;display:inline-flex;align-items:center;justify-content:center;width:1px;`)
	rest := css[strings.Index(css, `.card-edit{`):]
	rest = rest[:strings.Index(rest, "}")]
	assert.NotContains(t, rest, "display:none")
	assert.NotContains(t, rest, "visibility:hidden")
}

// av-isb3, av-uvc6: the gallery card's posture glyphs are neutral and
// informational, never a green/amber verdict (spec 6.2 treats the allowlist
// as transparency, not a grade). A sandboxed artifact that is not shared has
// nothing to report, so its card renders no trigger and no label at all: the
// absence is the signal, as it is for sharing (spec 7).
func TestGalleryCardShowsNoMarksWhenSandboxedAndPrivate(t *testing.T) {
	r := newTestRouter(t)
	createTestArtifact(t, r, "Plain")
	page := getPage(t, r, "/")

	assert.NotContains(t, page, "capability-cluster")
	assert.NotContains(t, page, "capability-popover")
	assert.NotContains(t, page, "Sandboxed")
	assert.NotContains(t, page, "ph-shield-check")

	// The badge is neutral: no color-as-verdict classes/hex from the old
	// green/amber design ever appear.
	assert.NotContains(t, page, "#12A150")
	assert.NotContains(t, page, "#B45309")
}

// Network origins present: a ph-globe glyph plus a count equal to
// len(NetworkAllowlist).
func TestGalleryCardShowsNetworkCount(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Networked")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"network_allowlist": []string{"https://a.example.com", "https://b.example.com"},
	})
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	// Glyphs alone name nothing, so the compact card trigger names itself.
	assert.Contains(t, page, `<div class="capability-cluster has-grants" tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger aria-label="Sandbox posture">`)
	assert.Contains(t, page, `<span class="capability-glyph"><i class="ph ph-globe"></i></span><span class="capability-count">2</span>`)
	assert.NotContains(t, page, "Sandboxed")
}

// Each capability glyph appears iff its approval flag is set, independent of
// the others and independent of the network allowlist.
func TestGalleryCardShowsCapabilityGlyphsPerFlag(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Capable")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"downloads_approved": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	assert.Contains(t, page, `<div class="capability-cluster has-grants" tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger aria-label="Sandbox posture">`)
	assert.Contains(t, page, `<span class="capability-glyph"><i class="ph ph-download-simple"></i></span>`)
	assert.NotContains(t, page, "ph-clipboard")
	assert.NotContains(t, page, "ph-globe")

	w3 := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"clipboard_approved": true,
	})
	require.Equal(t, http.StatusOK, w3.Code)

	req2 := httptest.NewRequest("GET", "/", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req2)
	require.Equal(t, http.StatusOK, w4.Code)
	page2 := w4.Body.String()

	assert.Contains(t, page2, `<span class="capability-glyph"><i class="ph ph-download-simple"></i></span>`)
	assert.Contains(t, page2, `<span class="capability-glyph"><i class="ph ph-clipboard"></i></span>`)
}

// The detail-view iframe is sandboxed with an opaque origin. An allow=
// delegation of clipboard keys on the frame's src origin, which is opaque and
// matches nothing, so the delegation was a no-op (av-hll6). Clipboard is instead
// proxied through the host via the capability bridge, so the detail page must
// NOT carry the dead allow= delegation, and must wire the host-side handler —
// without weakening the sandbox (allow-scripts stays, allow-same-origin omitted).
func TestDetailPageMediatesClipboardViaBridge(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Clip Tool", Tier: store.Tier1, CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)

	assert.NotContains(t, page, `allow="clipboard-read; clipboard-write"`,
		"the opaque-origin allow= delegation is a no-op and must be removed in favor of the bridge")
	// The host frame mediates clipboard requests posted by the shim; the
	// handler lives in the static page script the detail page loads.
	assert.Contains(t, page, `<script src="/assets/gallery/detail.js"></script>`)
	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	assert.Contains(t, string(detailJS), "__avClipboard",
		"detail page script must handle the shim's clipboard bridge messages")
	// The sandbox allows scripts and forms; same-origin is still withheld.
	assert.Contains(t, page, `sandbox="allow-scripts allow-forms"`)
	assert.NotContains(t, page, "allow-same-origin")
}

// av-hwx2: allowlist management moved entirely to the Edit page (av-p0a1).
// The viewer keeps only the read-only capability cluster (av-isb3/av-41se)
// and a toolbar "Manage" link to Edit — no inline editor, no add-origin
// control, and no client-side path that PATCHes network_allowlist.
func TestDetailPageIsReadOnlyWithManageLink(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Read Only Tool", Tier: store.Tier1,
		CreatedAt: time.Now(), NetworkAllowlist: []string{"https://example.com"}}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)

	// No inline allowlist editor or add-origin control.
	assert.NotContains(t, page, `id="al-display"`)
	assert.NotContains(t, page, `id="al-input"`)
	assert.NotContains(t, page, `addOrigin()`)
	// A visible toolbar link to the Edit page's security panel, in addition
	// to the one inside the popover.
	assert.Contains(t, page, `<a href="/artifacts/abc123/edit#security-panel">Manage in allowlist settings →</a>`)
	assert.Contains(t, page, `class="capability-popover-manage" href="/artifacts/abc123/edit#security-panel"`)
	// The capability cluster (the read-only replacement UI) is still present.
	assert.Contains(t, page, `class="capability-cluster`)

	// No client-side code path PATCHes network_allowlist from the viewer.
	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	assert.NotContains(t, string(detailJS), "network_allowlist",
		"the viewer must never mutate network_allowlist; management is Edit-only")
}

// av-kmwj: the runtime network-permission prompt lives in trusted app chrome,
// not in the artifact frame — the artifact controls that DOM and could draw a
// forgery of the dialog. This pins the markup half: the ids the page script
// addresses, and the bootstrap value it reloads the frame through. What the
// script does with them is exercised end to end in
// web/gallery/detail.net.test.mjs (run by TestGalleryPageScriptSuite).
func TestDetailPagePromptsForBlockedNetworkOrigins(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Fetcher", Tier: store.Tier1, CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)

	// The dialog, with all three answers the ticket specifies. It comes from
	// the shared networkPrompt partial (av-6xvs), so this also pins that the
	// page still includes it.
	assert.Contains(t, page, `id="net-modal"`)
	assert.Contains(t, page, `id="net-allow"`)
	assert.Contains(t, page, `id="net-once"`)
	assert.Contains(t, page, `id="net-never"`)
	// Empty in the markup: the origin comes out of the artifact's own blocked
	// request and is set as text by the script, never interpolated here.
	assert.Contains(t, page, `<code id="net-origin"></code>`)
	// Allow reloads the frame through the token-minting open route. FrameURL's
	// token is minted once, at page render, and expires — a page left open
	// longer would reload into a 404.
	assert.Contains(t, page, `const OPEN_URL = "/artifacts/abc123/open";`)

	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	assert.Contains(t, page, `<script src="/assets/gallery/network-prompt.js"></script>`,
		"the dialog is inert without the module that drives it")
	assert.Contains(t, string(detailJS), "ExhibitNetworkPrompt.install",
		"the viewer installs the shared prompt rather than hand-rolling one")

	promptJS, err := embeddedAssets.ReadFile("assets/gallery/network-prompt.js")
	require.NoError(t, err)
	assert.Contains(t, string(promptJS), "__avNetwork",
		"the module handles the render preamble's CSP-violation reports")
	assert.Contains(t, string(promptJS), "/origins",
		"decisions go through the per-origin route, not a whole-allowlist PATCH")
}

// av-6xvs: the agent chat page embeds the same render document behind the same
// sandbox on the same app origin, so it has the trusted chrome the prompt needs
// — and it had none of the prompt. An artifact reaching an unapproved origin
// while being built there failed silently.
//
// The two pages now share one dialog and one module, which is what this pins:
// the same partial and the same script on both, so the next fix to either
// cannot land on one surface and miss the other.
func TestAgentPageHostsTheNetworkPrompt(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/agent", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `id="net-modal"`)
	assert.Contains(t, page, `id="net-allow"`)
	assert.Contains(t, page, `id="net-once"`)
	assert.Contains(t, page, `id="net-never"`)
	assert.Contains(t, page, `<code id="net-origin"></code>`)
	assert.Contains(t, page, `<script src="/assets/gallery/network-prompt.js"></script>`)

	agentJS, err := embeddedAssets.ReadFile("assets/gallery/agent.js")
	require.NoError(t, err)
	assert.Contains(t, string(agentJS), "ExhibitNetworkPrompt.install",
		"the agent page must install the prompt, not merely render its markup")
	assert.Contains(t, string(agentJS), "ExhibitNetworkPrompt.announceTo",
		"each swapped-in preview frame must be told the host is listening")
}

// allowDecisions builds the allow-decision rows the edit page reads for its
// allowlist (exhibit-x87 — origins live in artifact_network_origins, so the
// page is fed decisions rather than an Artifact field).
func allowDecisions(origins ...string) []store.OriginDecision {
	ds := make([]store.OriginDecision, len(origins))
	for i, o := range origins {
		ds[i] = store.OriginDecision{Origin: o, Decision: store.DecisionAllow, Source: "user"}
	}
	return ds
}

// exhibit-x87: an origin carrying an explicit block decision ("don't ask
// again") must not read as merely undecided. It gets its own labelled section
// with an Allow override, and never appears in the allowlist or in the
// "referenced, not approved" list — even when the body references it.
func TestEditPageShowsBlockedOriginsDistinctlyFromUndecided(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Blocked", Tier: store.Tier1,
		CreatedAt: time.Now()}
	src := `<script src="https://blocked.example.com/a.js"></script>` +
		`<script src="https://new.example.com/b.js"></script>`
	decisions := []store.OriginDecision{
		{Origin: "https://blocked.example.com", Decision: store.DecisionBlock, Source: "runtime"},
	}
	page, err := renderEditPage(a, decisions, nil, src, "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)

	assert.Contains(t, page, `let blocked = ["https://blocked.example.com"];`,
		"a blocked origin must surface in its own list")
	assert.Contains(t, page, `let unapproved = ["https://new.example.com"];`,
		"only origins with no decision at all are 'referenced, not approved'")
	assert.Contains(t, page, `let allowlist = [];`,
		"a block decision must never widen the allowlist")
	assert.Contains(t, page, `<h3 class="text-sm muted">Blocked</h3>`,
		"blocked origins need their own labelled section, not a plain Allow row")
}

// av-kmwj: a "don't ask again" block must be reversible two ways — Allow
// (override it) and Forget (drop it, so the runtime prompt may ask again).
// Without Forget the answer is a one-way trap, since a blocked origin never
// prompts on its own. The behaviour behind the button — that Save deletes it
// through the per-origin route, and only after the PATCH — is exercised in
// web/gallery/edit.origins.test.mjs.
func TestEditPageCanForgetABlockDecision(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Blocked", Tier: store.Tier1,
		CreatedAt: time.Now()}
	decisions := []store.OriginDecision{
		{Origin: "https://tracker.example.com", Decision: store.DecisionBlock, Source: "runtime"},
	}
	page, err := renderEditPage(a, decisions, nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)
	assert.Contains(t, page, `data-action="forget"`)
	assert.Contains(t, page, `data-action="allow"`,
		"Forget is the second answer, not a replacement for overriding the block")

	editJS, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	assert.Contains(t, string(editJS), "'/origins?origin='",
		"PATCH cannot return an origin to undecided; only the per-origin DELETE can")
	assert.Contains(t, string(editJS), "method: 'DELETE'")
}

// av-p0a1: the edit page's security panel renders allowlist rows via
// html/template range (not hand-rolled string building), so origins the user
// typed into the "Add origin" field — unrestricted, unlike scanner-derived
// origins — stay inert even when they contain markup metacharacters. The
// "referenced, not approved" rows (Unapproved) go through the identical
// {{range}} construct in the template, so this coverage extends to them too.
func TestEditPageRendersAllowlistRowsInert(t *testing.T) {
	payload := `https://x"><img src=x onerror=alert(1)>`
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Edit XSS", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderEditPage(a, allowDecisions(payload), nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)

	assert.Contains(t, page, `<code title="https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;">https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;</code>`,
		"allowlist rows must HTML-escape the origin")
	assert.Contains(t, page, `data-origin="https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;"`,
		"the row's data-origin attribute must HTML-escape the origin")
	assert.NotContains(t, page, `<code>https://x"><img src=x onerror=alert(1)></code>`,
		"raw payload must never reach allowlist row markup")
}

// The edit page never offers the render origin as an Allow row. The write
// paths drop it, so the row would never clear.
func TestEditPageNeverOffersTheRenderOrigin(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Vendored", Tier: store.Tier1,
		CreatedAt: time.Now()}
	src := `<img src="https://render.test/a/abc123/assets/0f.png">` +
		`<script src="https://cdn.example.com/lib.js"></script>`
	page, err := renderEditPage(a, nil, nil, src, "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)

	assert.Contains(t, page, `let unapproved = ["https://cdn.example.com"];`)
}

// av-p0a1: origins the artifact's body references but hasn't approved
// (ingest-scan footprint minus the allowlist) surface as one-click "Allow"
// rows and must never be written to the allowlist itself.
func TestEditPageSurfacesUnapprovedOriginsWithoutSeedingAllowlist(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "No auto-seed", Tier: store.Tier1,
		CreatedAt: time.Now()}
	src := `<script src="https://cdn.example.com/lib.js"></script>`
	page, err := renderEditPage(a, nil, nil, src, "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)

	assert.Contains(t, page, `data-origin="https://cdn.example.com"`)
	assert.Contains(t, page, `data-action="allow"`)
	assert.Contains(t, page, `let allowlist = [];`,
		"a referenced-but-unapproved origin must not appear in the allowlist")
	assert.Contains(t, page, `let unapproved = ["https://cdn.example.com"];`,
		"the referenced origin must surface as unapproved instead")
}

// av-p0a1: the edit page inlines both the allowlist and the unapproved
// (referenced-but-not-approved) origins into its bootstrap <script> as JS
// arrays, same pattern as TestDetailPageInlinesAllowlistWithoutScriptBreakout
// above — an origin containing a literal </script> must not terminate the
// block early.
func TestEditPageInlinesAllowlistWithoutScriptBreakout(t *testing.T) {
	payload := `https://evil</script><img src=x onerror=alert(1)>`
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Script Breakout", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderEditPage(a, allowDecisions(payload), nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)

	assert.Contains(t, page, `let allowlist = ["https://evil\u003c/script\u003e\u003cimg src=x onerror=alert(1)\u003e"];`,
		"the inlined allowlist must escape '<', '>', '&' so origins cannot end the script element")
	assert.NotContains(t, page, `</script><img src=x onerror=alert(1)>`,
		"an origin must never terminate the inline script block early")
}

// av-d2xf: the edit page exposes the links_approved grant exactly like
// downloads/clipboard — a three-state select the page script PATCHes through
// the single write path, plus a bootstrap flag for the persisted state.
func TestEditPageShowsLinksCapabilitySelect(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Links", Tier: store.Tier1,
		CreatedAt: time.Now(), LinksApproved: true}
	page, err := renderEditPage(a, nil, nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)

	assert.Contains(t, page, `<span class="spacer">External links</span>`)
	assert.Contains(t, page, `id="link-select"`)
	assert.Contains(t, page, "let linksApproved = true;")

	a.LinksApproved = false
	page, err = renderEditPage(a, nil, nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), true, "")
	require.NoError(t, err)
	assert.Contains(t, page, "let linksApproved = false;")

	// The save path PATCHes links_approved alongside the other grants.
	editJS, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	assert.Contains(t, string(editJS), "links_approved")
}
