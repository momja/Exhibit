package api

import (
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exh-ghuv, Exh-i8q4: the detail page's chrome is one bar at the bottom of a
// full-window artifact, replacing the two-row header and toolbar on every
// viewport (and av-g7n7's bottom sheet on phones). This asserts the contract
// between the template, the stylesheet and pip.js — the ids the script binds
// to, the order of the bar, and that nothing of the old chrome (including the
// draggable pill's grip, hide item, restore tab and announcer) is left to
// drift — rather than any particular styling. The interaction itself is
// pip.test.mjs's.
func TestDetailPageCarriesFloatingControls(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Pill Tool", Tier: store.Tier1, CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)

	// Back, divider, posture, divider, then the buttons and the ⋮ menu.
	order := []string{`class="pip-btn" href="/"`, `class="pip-sep"`, `id="pip-title"`, `class="pip-sep"`, `id="pip-more"`}
	at := 0
	for _, marker := range order {
		i := strings.Index(page[at:], marker)
		require.GreaterOrEqual(t, i, 0, "the pill must carry %s after the previous control", marker)
		at += i + len(marker)
	}
	for _, id := range []string{"pip", "pip-info", "pip-menu"} {
		assert.Contains(t, page, `id="`+id+`"`, "pip.js binds to #%s", id)
	}
	for _, id := range []string{"pip-grip", "pip-hide", "pip-tab", "pip-announce"} {
		assert.NotContains(t, page, `id="`+id+`"`, "the draggable pill's #%s is gone", id)
	}
	assert.Contains(t, page, `<div class="pip" id="pip" role="group" aria-label="Artifact controls">`,
		"a labelled group, not a toolbar: there is no roving focus")
	assert.Contains(t, page, `<body class="pip-inset"`, "the band is reserved from the first paint")
	assert.Contains(t, page, `data-controller="floating-header"`)
	assert.Contains(t, page, `<script type="module" src="`+assetURL("gallery/controllers.js")+`"></script>`)
	assert.Contains(t, page, `<h1 class="pip-info-title font-headline" id="pip-info-title">Pill Tool</h1>`)
	assert.NotContains(t, page, `class="pip-title-text"`)
	assert.Contains(t, page, `aria-controls="pip-menu"`)

	for _, gone := range []string{"<header>", `id="sheet-toggle"`, `id="actions-sheet"`, `id="sheet-scrim"`, `class="toolbar"`} {
		assert.NotContains(t, page, gone, "the old chrome is gone, not hidden")
	}

	r := newTestRouter(t)
	css := galleryAsset(t, r, "/assets/gallery/detail.css")
	assert.Contains(t, css, ".panels{position:fixed;inset:0}", "the artifact fills the window")
	assert.Contains(t, css, "body.pip-inset .panels{bottom:var(--pip-reserve,", "less the band the bar reserves")
	assert.NotContains(t, css, "data-pip-corner", "the notices no longer follow a corner")
	assert.NotContains(t, css, "sheet-", "no stylesheet rule survives for the bottom sheet")

	js := galleryAsset(t, r, "/assets/gallery/detail.js")
	assert.NotContains(t, js, "setSheetOpen", "the sheet toggle went with the sheet")
	pip := galleryAsset(t, r, "/assets/gallery/pip.js")
	assert.Contains(t, pip, "from './pip-placement.js'", "the placement rules live in their own module")
	components := galleryAsset(t, r, "/assets/gallery/components.css")
	for _, gone := range []string{"pip-grip", "pip-tab", "pip-dragging", "pip-snapping", "pip-will-stash"} {
		assert.NotContains(t, components, gone, "no stylesheet rule survives for the draggable pill")
	}
}

// The ⋮ menu is the owner's actions and the recipient's one. A recipient
// gets "Open in new tab" and none of the owner's controls, which would 404
// (av-awr4). Neither gets a way to put the controls away: there is none.
func TestDetailPageMenuFollowsReadOnly(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Menu Tool", Tier: store.Tier1, CreatedAt: time.Now(), SourceURL: "https://example.com/tool"}
	owner, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)
	for _, item := range []string{`href="/artifacts/abc123/edit"`, `href="/agent?artifact=abc123"`, `href="/api/artifacts/abc123/export"`, `onclick="refetchSource()"`} {
		assert.Contains(t, owner, item)
	}

	recipientCreds := testPageCreds
	recipientCreds.ReadOnly = true
	recipient, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), recipientCreds, nil)
	require.NoError(t, err)
	assert.Contains(t, recipient, `id="pip-menu"`)
	assert.Contains(t, recipient, ` Open in new tab</button>`)
	for _, item := range []string{`href="/artifacts/abc123/edit"`, `href="/agent?artifact=abc123"`, `/export"`, `refetchSource()`, `id="share-open"`, `id="pip-hide"`} {
		assert.NotContains(t, recipient, item)
	}
}

// av-02xs: the detail page used to embed the artifact's full source body in a
// <pre> panel — for a multi-MB artifact the page itself became multi-MB, and
// Safari stalls on the response so the artifact "never loads". The panel was
// never a feature (mobile CSS deliberately hid it; the Edit action is the way
// to the code), so it is removed outright: the detail page must not carry the
// body in any form, and the render handler must not read the blob.
func TestDetailPageDoesNotEmbedSource(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Big Tool", Tier: store.Tier1, CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)

	assert.NotContains(t, page, "<pre>", "the source body must not appear in the detail page")
	assert.NotContains(t, page, "load-source", "no source controls on the detail page")
	assert.NotContains(t, page, "source-body", "no source pre on the detail page")
}
