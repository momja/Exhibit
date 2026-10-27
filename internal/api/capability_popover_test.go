package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// av-41se: the capability popover is the "receipt" for what an artifact was
// approved for at ingest (spec 6.2) — capabilityCluster's aria-describedby
// target. These tests assert on the rendered HTML/ARIA attributes for the
// popover's trigger, its content rows, and the footer Manage link, since the
// open/close interaction itself (hover, keyboard focus, tap, Esc) is
// CSS/JS-driven and is instead verified visually (shot-scraper) per the
// handoff.

// The card's trigger is a focusable control independent of the card around
// it: tabindex=0, role=button, aria-haspopup, and aria-describedby pointing
// at this artifact's popover id. A card renders one only when there is a
// posture to report (av-uvc6), so this artifact carries a grant.
func TestCapabilityPopoverTriggerIsFocusable(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Focusable")
	require.Equal(t, http.StatusOK, doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"downloads_approved": true,
	}).Code)

	page := getPage(t, r, "/")
	assert.Contains(t, page, `tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger`)
	assert.Contains(t, page, `<div class="capability-popover" id="capability-popover-`+id+`">`)
	assert.Contains(t, page, `<div class="capability-popover-header">Sandbox posture</div>`)
}

// On the detail page the receipt is opened by the posture button (Exh-ghuv),
// named by the artifact title in the popup and the posture mark. The receipt itself is the same partial the card
// uses, rendered for every artifact.
func TestDetailPagePostureButtonOpensTheReceipt(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Titled")
	page := getPage(t, r, "/artifacts/"+id)

	assert.Contains(t, page, `<button type="button" class="pip-title" id="pip-title" title="About this artifact" aria-labelledby="pip-info-title pip-posture" aria-expanded="false" aria-controls="pip-info">`,
		"the button names itself from the popup title, then the posture")
	assert.Contains(t, page, `<div class="pip-popup pip-info" id="pip-info" role="region" aria-label="About this artifact" tabindex="-1" hidden>`)
	assert.Contains(t, page, `<span class="pip-posture" data-posture-glyph><i class="ph ph-shield-check" aria-hidden="true"></i><span class="sr-only">Sandboxed.</span></span>`,
		"a sandboxed artifact's mark is the shield, and the posture button's name says so")
	assert.Contains(t, page, `<div class="capability-popover" id="capability-popover-`+id+`">`)
	assert.Contains(t, page, `<div class="capability-popover-header">Sandbox posture</div>`)
	assert.NotContains(t, page, "data-capability-trigger",
		"the posture button is the trigger here; a second one would be a second way to open the same receipt")
}

// With grants the mark lists them in the cluster's order, and its sr-only
// sentences spell them out for the title button's accessible name. The
// visible count is hidden from that name, since the sentence says it.
func TestDetailPagePostureMarkListsGrants(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Granted")
	require.Equal(t, http.StatusOK, doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"network_allowlist":  []string{"https://a.example.com", "https://b.example.com"},
		"downloads_approved": true,
	}).Code)
	page := getPage(t, r, "/artifacts/"+id)

	assert.Contains(t, page, `<span class="pip-posture" data-posture-glyph><i class="ph ph-globe" aria-hidden="true"></i><span class="capability-count" aria-hidden="true">2</span><span class="sr-only">Network access to 2 origins.</span><i class="ph ph-download-simple" aria-hidden="true"></i><span class="sr-only">Downloads allowed.</span></span>`)
}

// No grants at all collapses the popover body to a single reassurance row
// instead of per-capability rows. The detail toolbar is where a sandboxed,
// private artifact's popover lives; its card has none (av-uvc6).
func TestCapabilityPopoverSandboxedShowsFullyContained(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Plain")

	req := httptest.NewRequest("GET", "/artifacts/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, "Fully contained — no network, download, clipboard, external link, camera, microphone, or location access")
	assert.NotContains(t, page, "capability-popover-label")
	assert.NotContains(t, page, "capability-popover-origins")
}

// The network row lists every approved origin verbatim, in monospace, with
// correct singular/plural phrasing.
func TestCapabilityPopoverNetworkRowListsOriginsAndPluralizes(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "One origin")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"network_allowlist": []string{"https://one.example.com"},
	})
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	assert.Contains(t, page, `<div class="capability-popover-label">Network — 1 origin</div>`)
	assert.Contains(t, page, `<code>https://one.example.com</code>`)
	assert.NotContains(t, page, "1 origins")

	id2 := createTestArtifact(t, r, "Two origins")
	w3 := doJSON(t, r, "PATCH", "/api/artifacts/"+id2, map[string]any{
		"network_allowlist": []string{"https://a.example.com", "https://b.example.com"},
	})
	require.Equal(t, http.StatusOK, w3.Code)

	req2 := httptest.NewRequest("GET", "/", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req2)
	require.Equal(t, http.StatusOK, w4.Code)
	page2 := w4.Body.String()

	assert.Contains(t, page2, `<div class="capability-popover-label">Network — 2 origins</div>`)
	assert.Contains(t, page2, `<code>https://a.example.com</code>`)
	assert.Contains(t, page2, `<code>https://b.example.com</code>`)
}

// The Downloads/Clipboard rows appear iff their approval flags are set,
// each with its neutral one-line meaning, independent of the other.
func TestCapabilityPopoverDownloadsAndClipboardRowsPerFlag(t *testing.T) {
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

	assert.Contains(t, page, "Downloads — Can save files to your device")
	assert.NotContains(t, page, "Clipboard — Can read and write your clipboard")

	w3 := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"clipboard_approved": true,
	})
	require.Equal(t, http.StatusOK, w3.Code)

	req2 := httptest.NewRequest("GET", "/", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req2)
	require.Equal(t, http.StatusOK, w4.Code)
	page2 := w4.Body.String()

	assert.Contains(t, page2, "Downloads — Can save files to your device")
	assert.Contains(t, page2, "Clipboard — Can read and write your clipboard")
}

// av-d2xf: the external-links grant appears as its own popover row and cluster
// glyph, and a links-only grant must never collapse to the "Fully contained"
// reassurance row.
func TestCapabilityPopoverLinksRowPerFlag(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Linky")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"links_approved": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	// The glyph must appear in each of its two slots — the card's
	// capability-cluster and the popover row — not merely once anywhere on the
	// page.
	glyph := `<span class="capability-glyph"><i class="ph ph-arrow-square-out"></i></span>`
	assert.Contains(t, page, "External links — Can open links in new tabs")
	assert.Contains(t, page, `<div class="capability-popover-row">`+glyph+`<span>External links — Can open links in new tabs</span></div>`)
	clusterStart := strings.Index(page, "capability-cluster")
	// Note: the cluster's aria-describedby attribute also contains the
	// capability-popover-<id> substring, so anchor the popover boundary on its
	// <div> tag, not the bare id prefix.
	popoverStart := strings.Index(page, `<div class="capability-popover" id="`)
	require.True(t, clusterStart >= 0 && popoverStart > clusterStart, "cluster must precede its popover")
	assert.Contains(t, page[clusterStart:popoverStart], glyph, "capability-cluster must render the links glyph")
	assert.NotContains(t, page, "Fully contained")
}

// av-mv3k, av-f446: camera, microphone and location are three grants, not one
// "media" grant — each gets its own popover row and cluster glyph, and
// approving one must not display the others. A grant of any one of them must
// never collapse to "Fully contained" either.
func TestCapabilityPopoverDeviceAndLocationRowsPerFlag(t *testing.T) {
	cases := []struct {
		field  string
		glyph  string
		row    string
		absent []string
	}{
		{"camera_approved", `<span class="capability-glyph"><i class="ph ph-camera"></i></span>`,
			"Camera — Can capture video from your camera", []string{"Microphone —", "Location —"}},
		{"microphone_approved", `<span class="capability-glyph"><i class="ph ph-microphone"></i></span>`,
			"Microphone — Can capture audio from your microphone", []string{"Camera —", "Location —"}},
		{"geolocation_approved", `<span class="capability-glyph"><i class="ph ph-map-pin"></i></span>`,
			"Location — Can read your location", []string{"Camera —", "Microphone —"}},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			r := newTestRouter(t)
			id := createTestArtifact(t, r, "Recorder")
			w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{tc.field: true})
			require.Equal(t, http.StatusOK, w.Code)

			req := httptest.NewRequest("GET", "/", nil)
			w2 := httptest.NewRecorder()
			r.ServeHTTP(w2, req)
			require.Equal(t, http.StatusOK, w2.Code)
			page := w2.Body.String()

			assert.Contains(t, page, `<div class="capability-popover-row">`+tc.glyph+`<span>`+tc.row+`</span></div>`)
			clusterStart := strings.Index(page, "capability-cluster")
			popoverStart := strings.Index(page, `<div class="capability-popover" id="`)
			require.True(t, clusterStart >= 0 && popoverStart > clusterStart, "cluster must precede its popover")
			assert.Contains(t, page[clusterStart:popoverStart], tc.glyph, "capability-cluster must render the glyph")
			// One grant is one grant: the siblings stay unmentioned.
			for _, absent := range tc.absent {
				assert.NotContains(t, page, absent)
			}
			assert.NotContains(t, page, "Fully contained")
		})
	}
}

// The footer Manage link points at the artifact's Edit page security section
// and is present on both the gallery card and the artifact viewer/detail
// page — the ticket explicitly calls out "card + viewer", not just one.
func TestCapabilityPopoverManageLinkOnGalleryAndDetail(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Managed")
	// A grant, so the card has a posture to report and renders its trigger.
	require.Equal(t, http.StatusOK, doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"downloads_approved": true,
	}).Code)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `<a class="capability-popover-manage" href="/artifacts/`+id+`/edit#security-panel">Manage security settings →</a>`)

	req2 := httptest.NewRequest("GET", "/artifacts/"+id, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	page2 := w2.Body.String()
	assert.Contains(t, page2, `<a class="capability-popover-manage" href="/artifacts/`+id+`/edit#security-panel">Manage security settings →</a>`)
	// The viewer's receipt is the same component, opened by the title.
	assert.Contains(t, page2, `aria-controls="pip-info"`)
}

// Origins are rendered through html/template's contextual auto-escaping, not
// hand-rolled escaping — an origin containing HTML-significant characters must
// come out escaped, never as raw markup an attacker-controlled origin could
// use to break out of the popover.
//
// Since av-i7hd such a value can no longer reach the store at all (the write
// path rejects it, asserted below), so the escaping guarantee is exercised
// against the partial directly — the shape a row written before that
// validation existed still has.
func TestCapabilityPopoverEscapesOrigins(t *testing.T) {
	html, err := renderPartial(t, "capabilityCluster", capabilityView{
		ArtifactID:       "art-escaped",
		NetworkAllowlist: []string{`https://evil.example.com"><script>alert(1)</script>`},
		ShowManage:       true,
	})
	require.NoError(t, err)
	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

// The same hostile value at the single write path: it is not stored and then
// escaped, it is refused, and the 400 names the entry so the edit page can
// point at the row the user typed (av-i7hd).
func TestAllowlistRejectsHostileOrigin(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Escaped")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"network_allowlist": []string{`https://evil.example.com"><script>alert(1)</script>`},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), `evil.example.com`)
}

// The popover's footer Manage link is gated by ShowManage — off in the
// no-owner-session / share case. Nothing under internal/render (which serves
// /s/:shareID) ever composes this gallery partial — the render surface emits
// only the bare artifact document, no gallery chrome at all — so there is no
// live HTTP path to exercise "share view renders the popover without the
// link" end to end. This test instead executes the capabilityCluster
// partial directly with ShowManage: false, the same data shape any future
// no-owner-session caller would pass, and asserts the link is absent while
// the rest of the popover still renders.
func TestCapabilityPopoverManageLinkGatedByShowManage(t *testing.T) {
	shown := capabilityView{
		ArtifactID:        "art-shown",
		NetworkAllowlist:  []string{"https://example.com"},
		DownloadsApproved: true,
		ShowManage:        true,
	}
	hidden := shown
	hidden.ArtifactID = "art-hidden"
	hidden.ShowManage = false

	htmlShown, err := renderPartial(t, "capabilityCluster", shown)
	require.NoError(t, err)
	assert.Contains(t, htmlShown, `Manage security settings`)
	assert.Contains(t, htmlShown, `href="/artifacts/art-shown/edit#security-panel"`)

	htmlHidden, err := renderPartial(t, "capabilityCluster", hidden)
	require.NoError(t, err)
	assert.NotContains(t, htmlHidden, `Manage security settings`)
	assert.NotContains(t, htmlHidden, `capability-popover-manage`)
	// Everything else about the popover still renders — ShowManage gates
	// only the footer link, not the rest of the "receipt".
	assert.Contains(t, htmlHidden, `Sandbox posture`)
	assert.Contains(t, htmlHidden, `Downloads — Can save files to your device`)
	assert.True(t, strings.Contains(htmlHidden, `https://example.com`))
}

// renderPartial executes one of the shared page-template partials directly,
// bypassing a full page/HTTP round trip — useful for a partial whose gating
// (ShowManage) has no live caller yet (see
// TestCapabilityPopoverManageLinkGatedByShowManage).
func renderPartial(t *testing.T, name string, data any) (string, error) {
	t.Helper()
	var b strings.Builder
	err := pageTemplates.ExecuteTemplate(&b, name, data)
	return b.String(), err
}

// av-mf1x: the capability cluster fragment, re-fetched by htmx after a
// runtime origin approval so the toolbar stops describing the allowlist as it
// was at page load. The fragment carries the badge and the popover together —
// capabilityCluster renders both — so one swap refreshes both stale surfaces
// the ticket names.
func TestCapabilityClusterPartialReflectsCurrentAllowlist(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Badge refresh")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"network_allowlist": []string{"https://cdn.example.com"},
	})
	require.Equal(t, http.StatusOK, w.Code)

	fragment := getPage(t, r, "/partials/capability-cluster?artifact="+id)
	assert.Contains(t, fragment, `<span class="capability-count" aria-hidden="true">1</span>`)
	assert.Contains(t, fragment, `<code>https://cdn.example.com</code>`)
	assert.Contains(t, fragment, `Manage security settings`,
		"the owner's fragment keeps the popover's footer link, like the owner's page")

	// The fragment renders the same partial the full page render used — one
	// definition of the badge, which is the whole point of the exercise.
	page := getPage(t, r, "/artifacts/"+id)
	for _, marker := range []string{`<span class="capability-count" aria-hidden="true">1</span>`, `<code>https://cdn.example.com</code>`} {
		assert.Contains(t, fragment, marker)
		assert.Contains(t, page, marker)
	}
}

// An unknown artifact answers 404 with the user-facing fragmentError
// partial, which htmx swaps into the toolbar in place of the stale badge —
// the visitor learns the artifact is gone instead of staring at its old
// posture.
func TestCapabilityClusterPartialUnknownArtifactIsNotFound(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/partials/capability-cluster?artifact=does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `class="frag-error"`)
	assert.Contains(t, w.Body.String(), "Artifact not found. It may have been deleted.")
	assert.NotContains(t, w.Body.String(), "capability-cluster")
}

// The cluster reports what the tool may reach in the visitor's browser, so a
// recipient with a grant reads the same fragment as the owner — minus the
// Manage link, which points at an edit page they cannot reach. A stranger
// with no grant gets the 404 the detail page gives them.
func TestCapabilityClusterPartialRecipientReadsPostureWithoutManageLink(t *testing.T) {
	in := newGrantedInstance(t)

	owner := responseText(in.get(t, "/partials/capability-cluster?artifact="+in.granted, in.cookieOne))
	assert.Contains(t, owner, `Manage security settings`)

	recipient := in.get(t, "/partials/capability-cluster?artifact="+in.granted, in.cookieTwo)
	require.Equal(t, http.StatusOK, recipient.Code)
	assert.Contains(t, responseText(recipient), `Sandbox posture`)
	assert.NotContains(t, responseText(recipient), `Manage security settings`)
	assert.NotContains(t, responseText(recipient), `capability-popover-manage`)
}

// The detail page must carry the wiring that turns an approval into a swap:
// the receipt wrapped in an htmx target fetching the fragment above on the
// event the runtime prompt fires, with htmx itself served from our own
// origin (never a CDN). The posture mark in the title takes its own half of
// the same fragment on the same event (Exh-ghuv), and the receipt's target
// also selects the error fragment, so a deleted artifact says so.
func TestDetailPageWiresCapabilityClusterSwapToHtmx(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Swap wiring")
	page := getPage(t, r, "/artifacts/"+id)

	assert.Contains(t, page, `<script src="`+assetURL("htmx/htmx.min.js")+`"></script>`)
	assert.NotContains(t, page, "unpkg.com")
	assert.Contains(t, page, `id="capability-cluster"`)
	assert.Contains(t, page, `hx-get="/partials/capability-cluster?artifact=`+id+`"`)
	assert.Contains(t, page, `hx-trigger="exhibit:capabilities-changed from:body"`)
	assert.Contains(t, page, `hx-swap="innerHTML"`)
	assert.Contains(t, page, `hx-select=".capability-popover, .frag-error"`)
	assert.Contains(t, page, `id="pip-posture"`)
	assert.Contains(t, page, `hx-select=".pip-posture"`)
	assert.Equal(t, 2, strings.Count(page, `hx-get="/partials/capability-cluster?artifact=`+id+`"`),
		"the mark and the receipt refresh from one fragment")
}

// The fragment carries both halves the page selects from: the posture mark
// and the receipt.
func TestCapabilityClusterPartialCarriesMarkAndReceipt(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Both halves")
	fragment := getPage(t, r, "/partials/capability-cluster?artifact="+id)

	assert.Contains(t, fragment, `<span class="pip-posture" data-posture-glyph>`)
	assert.Contains(t, fragment, `<div class="capability-popover" id="capability-popover-`+id+`">`)
}
