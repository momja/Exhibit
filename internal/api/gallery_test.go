package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Contains(t, page, `.tag-pill{position:relative;display:inline-flex;align-items:center;justify-content:center;max-width:100%;height:22px;gap:5px;padding:0 7px;border-radius:999px;font-size:11px`)
	// The dot and label flow together as one flex group; symmetric padding
	// centers that group so the right side isn't padded more than the left.
	// The dot is NOT absolutely positioned — it is a normal flex item.
	assert.Contains(t, page, `.tag-dot{flex:0 0 auto;width:8px;height:8px;border-radius:50%;background:#888;transition:opacity .12s ease}`)
	assert.Contains(t, page, `.card-title{font-size:15px;font-weight:600`)
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
	assert.Contains(t, page, `.tag-pill-edit,.tag-pill-detach{position:absolute`)
	assert.Contains(t, page, `opacity:0;pointer-events:none`)
	assert.Contains(t, page, `.tag-pill:hover .tag-dot,.tag-pill:focus-within .tag-dot{opacity:0}`)
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
	assert.Contains(t, page, `function openEditTagModal(`)
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
	assert.Contains(t, page, `function openAddTagModal(`)
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
	assert.Contains(t, page, `runSearch`) // debounce + fetch + grid-swap script
	assert.NotContains(t, page, `type="submit"`)
	assert.NotContains(t, page, `>Search</button>`)
}

// The exhibit header must read as distinct from the white content cards below
// it: it is sticky (stays visible while scrolling) and carries a real shadow +
// stronger border rather than the same near-invisible hairline the cards use.
func TestGalleryHeaderHasVisualSeparation(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	// The header must be visually distinct from the white content cards below
	// it: it is sticky (stays visible while scrolling) and carries a real
	// shadow + stronger border rather than the same near-invisible hairline
	// the cards use. A bare 1px #e0e0e0 border alone read as flush with content.
	assert.Contains(t, page, `header{position:sticky;top:0;z-index:20`)
	assert.Contains(t, page, `box-shadow:0 1px 6px rgba(0,0,0,.07)`) // grep-friendly: 'box-shadow'
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

// The detail-view iframe is sandboxed with an opaque origin. An allow=
// delegation of clipboard keys on the frame's src origin, which is opaque and
// matches nothing, so the delegation was a no-op (av-hll6). Clipboard is instead
// proxied through the host via the capability bridge, so the detail page must
// NOT carry the dead allow= delegation, and must wire the host-side handler —
// without weakening the sandbox (allow-scripts stays, allow-same-origin omitted).
func TestDetailPageMediatesClipboardViaBridge(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Clip Tool", Tier: store.Tier1, CreatedAt: time.Now()}
	page := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")

	assert.NotContains(t, page, `allow="clipboard-read; clipboard-write"`,
		"the opaque-origin allow= delegation is a no-op and must be removed in favor of the bridge")
	// The host frame mediates clipboard requests posted by the shim.
	assert.Contains(t, page, "__avClipboard",
		"detail page must handle the shim's clipboard bridge messages")
	// The sandbox is unchanged: scripts allowed, same-origin still withheld.
	assert.Contains(t, page, `sandbox="allow-scripts"`)
	assert.NotContains(t, page, "allow-same-origin")
}
