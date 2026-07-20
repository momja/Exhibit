package api

import (
	"net/http"
	"net/http/httptest"
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

func TestGalleryIndexRendersTagPills(t *testing.T) {
	r := newTestRouter(t)

	dark := createTestTag(t, r, "charts", "#FFFFFF")  // light bg -> dark text
	light := createTestTag(t, r, "urgent", "#111111") // dark bg -> light text
	id := createTestArtifact(t, r, "Tagged")

	for _, tag := range []struct{ id string }{{dark.ID}, {light.ID}} {
		w := doJSON(t, r, "POST", "/api/tags/"+tag.id+"/artifacts/"+id, nil)
		require.Equal(t, http.StatusNoContent, w.Code)
	}

	untaggedID := createTestArtifact(t, r, "Untagged")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// Tagged card: a pill list keyed to the artifact, with per-tag hooks.
	// Pills are neutral (single low-saturation color) with the tag color
	// carried by a leading dot — not a filled-color pill — so a row of tags
	// reads as metadata, not as headers bigger than the card title.
	assert.Contains(t, page, `<ul class="tag-pills" data-artifact-id="`+id+`">`)
	assert.Contains(t, page, `<li class="tag-pill" data-tag-id="`+dark.ID+`">`)
	assert.Contains(t, page, `<span class="tag-dot" style="background:#ffffff" aria-hidden="true"></span>`)
	assert.Contains(t, page, `<span class="tag-pill-label">charts</span>`)
	assert.Contains(t, page, `<span class="tag-dot" style="background:#111111" aria-hidden="true"></span>`)
	assert.Contains(t, page, `<span class="tag-pill-label">urgent</span>`)
	// The filled-color pill style is gone.
	assert.NotContains(t, page, `style="background:#ffffff;color:`)
	assert.NotContains(t, page, `style="background:#111111;color:`)

	// Untagged card: no empty pill row, but still a trailing '+' to add the
	// first tag (tww.2.5).
	assert.NotContains(t, page, `<ul class="tag-pills" data-artifact-id="`+untaggedID+`">`)
	assert.Contains(t, page, `<button type="button" class="tag-add-btn" data-artifact-id="`+untaggedID+`" aria-label="Add tag">`)

	// Tags are smaller than the title: pill 11px vs card-title 15px, so the row
	// never outweighs the artifact name it belongs to.
	css := galleryAsset(t, r, "/assets/gallery/index.css")
	assert.Contains(t, css, `.tag-pill{position:relative;display:inline-flex;align-items:center;justify-content:center;max-width:100%;height:22px;gap:5px;padding:0 7px;border-radius:999px;font-size:11px`)
	// The dot and label flow together as one flex group; symmetric padding
	// centers that group so the right side isn't padded more than the left.
	// The dot is NOT absolutely positioned — it is a normal flex item.
	assert.Contains(t, css, `.tag-dot{flex:0 0 auto;width:8px;height:8px;border-radius:50%;background:#888;transition:opacity .12s ease}`)
	assert.Contains(t, css, `.card-title{font-size:15px;font-weight:600`)
}

func TestTagPillHoverControls(t *testing.T) {
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

	// Pencil (edit) on the left of the label, x (detach) on the right —
	// both real <button>s so they're keyboard-focusable, and both carry the
	// data the click handlers need.
	assert.Contains(t, page, `<button type="button" class="tag-pill-edit" data-tag-id="`+tag.ID+`" data-tag-name="charts" data-tag-color="#ffffff" aria-label="Edit tag charts"><i class="ph ph-pencil-simple"></i></button>`)
	assert.Contains(t, page, `<span class="tag-pill-label">charts</span>`)
	assert.Contains(t, page, `<button type="button" class="tag-pill-detach" data-tag-id="`+tag.ID+`" data-artifact-id="`+id+`" aria-label="Remove tag charts from this artifact"><i class="ph ph-x"></i></button>`)

	// Hidden-until-hover/focus is CSS-driven (opacity/pointer-events on
	// .tag-pill-edit/.tag-pill-detach). The controls are absolutely positioned
	// over the pill's end caps (overlay) so revealing them never shifts the
	// pill; the dot fades out when the edit pencil enters the left cap.
	css := galleryAsset(t, r, "/assets/gallery/index.css")
	assert.Contains(t, css, `.tag-pill-edit,.tag-pill-detach{position:absolute`)
	assert.Contains(t, css, `opacity:0;pointer-events:none`)
	assert.Contains(t, css, `.tag-pill:hover .tag-dot,.tag-pill:focus-within .tag-dot{opacity:0}`)
}

func TestGalleryIndexRendersEditTagModal(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// One shared, initially-hidden modal shell per page load; per-tag state
	// is populated by openEditTagModal in the page script.
	assert.Contains(t, page, `<div id="tag-edit-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `<input type="text" id="tag-edit-name" maxlength="60">`)
	assert.Contains(t, page, `<div class="color-presets">`)
	assert.Contains(t, page, `data-color="#6B7280"`) // store.DefaultTagColor preset
	assert.Contains(t, page, `id="tag-edit-color-hex"`)
	assert.Contains(t, page, `id="tag-edit-delete"`)
	assert.Contains(t, page, `id="tag-edit-save"`)
	// The modal's behavior lives in the static page script the page loads.
	assert.Contains(t, page, `<script src="/assets/gallery/index.js"></script>`)
	assert.Contains(t, galleryAsset(t, r, "/assets/gallery/index.js"), `function openEditTagModal(`)
}

func TestGalleryIndexRendersAddTagModal(t *testing.T) {
	r := newTestRouter(t)
	tag := createTestTag(t, r, "charts", "")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// One shared, initially-hidden modal shell with a dropdown built from
	// every existing tag, plus a "create new" option that reveals the same
	// name+color fields as the edit-tag modal.
	assert.Contains(t, page, `<div id="tag-add-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `<option value="`+tag.ID+`">charts</option>`)
	assert.Contains(t, page, `<option value="__new__">+ Create new tag</option>`)
	assert.Contains(t, page, `<input type="text" id="tag-add-name" maxlength="60">`)
	assert.Contains(t, page, `id="tag-add-confirm"`)
	assert.Contains(t, galleryAsset(t, r, "/assets/gallery/index.js"), `function openAddTagModal(`)
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

	// The removed 'Open ↗' action and any new-tab opener are gone from cards.
	assert.NotContains(t, page, "Open ↗")
	assert.NotContains(t, page, `target="_blank"`)
}

// av-isb3: the gallery card footer shows a neutral, informational capability
// posture cluster opposite the created date — never a green/amber verdict
// (spec 6.2 treats the allowlist as transparency, not a grade). A fully
// sandboxed artifact (no allowlist entries, no capability grants) collapses
// to exactly one muted ph-shield-check + "Sandboxed" mark.
func TestGalleryCardShowsSandboxedWhenNoGrants(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Plain")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `<div class="capability-cluster" tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger>`)
	assert.Contains(t, page, `<span class="capability-glyph"><i class="ph ph-shield-check"></i></span> Sandboxed`)
	assert.NotContains(t, page, "has-grants")
	assert.NotContains(t, page, "ph-globe")
	assert.NotContains(t, page, "ph-download-simple")
	assert.NotContains(t, page, "ph-clipboard")

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

	assert.Contains(t, page, `<div class="capability-cluster has-grants" tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger>`)
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

	assert.Contains(t, page, `<div class="capability-cluster has-grants" tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger>`)
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
	page, err := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
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
	// The sandbox is unchanged: scripts allowed, same-origin still withheld.
	assert.Contains(t, page, `sandbox="allow-scripts"`)
	assert.NotContains(t, page, "allow-same-origin")
}

// av-hwx2: allowlist management moved entirely to the Edit page (av-p0a1).
// The viewer keeps only the read-only capability cluster (av-isb3/av-41se)
// and a toolbar "Manage" link to Edit — no inline editor, no add-origin
// control, and no client-side path that PATCHes network_allowlist.
func TestDetailPageIsReadOnlyWithManageLink(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Read Only Tool", Tier: store.Tier1,
		CreatedAt: time.Now(), NetworkAllowlist: []string{"https://example.com"}}
	page, err := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
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

// av-p0a1: the edit page's security panel renders allowlist rows via
// html/template range (not hand-rolled string building), so origins the user
// typed into the "Add origin" field — unrestricted, unlike scanner-derived
// origins — stay inert even when they contain markup metacharacters. The
// "referenced, not approved" rows (Unapproved) go through the identical
// {{range}} construct in the template, so this coverage extends to them too.
func TestEditPageRendersAllowlistRowsInert(t *testing.T) {
	payload := `https://x"><img src=x onerror=alert(1)>`
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Edit XSS", Tier: store.Tier1,
		CreatedAt: time.Now(), NetworkAllowlist: []string{payload}}
	page, err := renderEditPage(a, "<p>src</p>", "tok")
	require.NoError(t, err)

	assert.Contains(t, page, `<code title="https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;">https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;</code>`,
		"allowlist rows must HTML-escape the origin")
	assert.Contains(t, page, `data-origin="https://x&#34;&gt;&lt;img src=x onerror=alert(1)&gt;"`,
		"the row's data-origin attribute must HTML-escape the origin")
	assert.NotContains(t, page, `<code>https://x"><img src=x onerror=alert(1)></code>`,
		"raw payload must never reach allowlist row markup")
}

// av-p0a1: origins the artifact's body references but hasn't approved
// (ingest-scan footprint minus the allowlist) surface as one-click "Allow"
// rows and must never be written to the allowlist itself.
func TestEditPageSurfacesUnapprovedOriginsWithoutSeedingAllowlist(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "No auto-seed", Tier: store.Tier1,
		CreatedAt: time.Now(), NetworkAllowlist: []string{}}
	src := `<script src="https://cdn.example.com/lib.js"></script>`
	page, err := renderEditPage(a, src, "tok")
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
		CreatedAt: time.Now(), NetworkAllowlist: []string{payload}}
	page, err := renderEditPage(a, "<p>src</p>", "tok")
	require.NoError(t, err)

	assert.Contains(t, page, `let allowlist = ["https://evil\u003c/script\u003e\u003cimg src=x onerror=alert(1)\u003e"];`,
		"the inlined allowlist must escape '<', '>', '&' so origins cannot end the script element")
	assert.NotContains(t, page, `</script><img src=x onerror=alert(1)>`,
		"an origin must never terminate the inline script block early")
}
