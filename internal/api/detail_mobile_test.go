package api

import (
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-g7n7: on a phone the viewer's toolbar wrapped into a multi-line wall that
// crushed the artifact. It is now the same DOM restyled as a bottom sheet the
// header's kebab opens over a scrim. The markup carries three pieces the sheet
// cannot work without — the kebab, its aria wiring to the toolbar, and the
// scrim — and all three are inert (display:none) above the breakpoint, so this
// asserts the contract between the template, the stylesheet, and the script
// rather than any particular styling.
func TestDetailPageCarriesMobileActionsSheet(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Sheet Tool", Tier: store.Tier1, CreatedAt: time.Now()}
	page, err := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
	require.NoError(t, err)

	assert.Contains(t, page, `id="sheet-toggle"`, "the header needs the kebab that opens the sheet")
	assert.Contains(t, page, `aria-controls="actions-sheet"`)
	assert.Contains(t, page, `<div class="toolbar" id="actions-sheet">`,
		"the kebab's aria-controls must name the toolbar it turns into a sheet")
	assert.Contains(t, page, `id="sheet-scrim"`, "the sheet needs a scrim to dismiss it")

	r := newTestRouter(t)
	css := galleryAsset(t, r, "/assets/gallery/detail.css")
	assert.Contains(t, css, "@media (max-width:640px)",
		"the mobile layout hangs off the 640px breakpoint shared with edit.css")
	assert.Contains(t, css, "body.sheet-open .toolbar{transform:none",
		"one body class drives the sheet's open state")

	js := galleryAsset(t, r, "/assets/gallery/detail.js")
	assert.Contains(t, js, "function setSheetOpen(", "the kebab/scrim toggle lives in the page script")
}
