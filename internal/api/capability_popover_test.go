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

// The trigger is a focusable control independent of whatever page embeds it
// (card or toolbar): tabindex=0, role=button, aria-haspopup, and
// aria-describedby pointing at this artifact's popover id.
func TestCapabilityPopoverTriggerIsFocusable(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Focusable")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, `tabindex="0" role="button" aria-haspopup="true" aria-expanded="false" aria-describedby="capability-popover-`+id+`" data-capability-trigger`)
	assert.Contains(t, page, `<div class="capability-popover" id="capability-popover-`+id+`">`)
	assert.Contains(t, page, `<div class="capability-popover-header">Sandbox posture</div>`)
}

// No grants at all collapses the popover body to a single reassurance row
// instead of per-capability rows.
func TestCapabilityPopoverSandboxedShowsFullyContained(t *testing.T) {
	r := newTestRouter(t)
	createTestArtifact(t, r, "Plain")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()

	assert.Contains(t, page, "Fully contained — no network, download, or clipboard access")
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

// The footer Manage link points at the artifact's Edit page security section
// and is present on both the gallery card and the artifact viewer/detail
// page — the ticket explicitly calls out "card + viewer", not just one.
func TestCapabilityPopoverManageLinkOnGalleryAndDetail(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Managed")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `<a class="capability-popover-manage" href="/artifacts/`+id+`/edit#security-panel">Manage in allowlist settings →</a>`)

	req2 := httptest.NewRequest("GET", "/artifacts/"+id, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	page2 := w2.Body.String()
	assert.Contains(t, page2, `<a class="capability-popover-manage" href="/artifacts/`+id+`/edit#security-panel">Manage in allowlist settings →</a>`)
	// The viewer's cluster/popover is the same component, wired the same way.
	assert.Contains(t, page2, `aria-describedby="capability-popover-`+id+`" data-capability-trigger`)
}

// Origins are rendered through html/template's contextual auto-escaping, not
// hand-rolled escaping — an origin containing HTML-significant characters
// (as the scanner can produce, per scanner-origins-need-html-escaping) must
// come out escaped, never as raw markup an attacker-controlled origin could
// use to break out of the popover.
func TestCapabilityPopoverEscapesOrigins(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Escaped")
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{
		"network_allowlist": []string{`https://evil.example.com"><script>alert(1)</script>`},
	})
	require.Equal(t, http.StatusOK, w.Code)

	req := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	require.Equal(t, http.StatusOK, w2.Code)
	page := w2.Body.String()

	assert.NotContains(t, page, "<script>alert(1)</script>")
	assert.Contains(t, page, "&lt;script&gt;alert(1)&lt;/script&gt;")
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
	assert.Contains(t, htmlShown, `Manage in allowlist settings`)
	assert.Contains(t, htmlShown, `href="/artifacts/art-shown/edit#security-panel"`)

	htmlHidden, err := renderPartial(t, "capabilityCluster", hidden)
	require.NoError(t, err)
	assert.NotContains(t, htmlHidden, `Manage in allowlist settings`)
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
