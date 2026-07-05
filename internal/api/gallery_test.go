package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

	// Tagged card: a pill list keyed to the artifact, with per-tag hooks and
	// auto-contrasted text for both a light and a dark background.
	assert.Contains(t, page, `<ul class="tag-pills" data-artifact-id="`+id+`">`)
	assert.Contains(t, page, `data-tag-id="`+dark.ID+`" style="background:#ffffff;color:#111111"`)
	assert.Contains(t, page, `<span class="tag-pill-label">charts</span>`)
	assert.Contains(t, page, `data-tag-id="`+light.ID+`" style="background:#111111;color:#ffffff"`)
	assert.Contains(t, page, `<span class="tag-pill-label">urgent</span>`)

	// Untagged card: no empty pill row, but still a trailing '+' to add the
	// first tag (tww.2.5).
	assert.NotContains(t, page, `<ul class="tag-pills" data-artifact-id="`+untaggedID+`">`)
	assert.Contains(t, page, `<button type="button" class="tag-add-btn" data-artifact-id="`+untaggedID+`" aria-label="Add tag">`)
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
	// .tag-pill-edit/.tag-pill-detach), not a layout property, so revealing
	// them never shifts the pill.
	assert.Contains(t, page, `.tag-pill-edit,.tag-pill-detach{display:inline-flex`)
	assert.Contains(t, page, `opacity:0;pointer-events:none`)
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
