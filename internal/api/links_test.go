package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// links_approved is the link navigation bridge's first-use approval (av-r0dk):
// PATCHed through the single write path, persisted server-side so it survives
// reloads and devices, and revocable the same way.
func TestPatchLinksApproved(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Link Collector")

	// New artifacts must never be pre-approved.
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.LinksApproved)

	// PATCH wraps the artifact alongside the re-scan footprint (updateArtifactResponse).
	var resp struct {
		Artifact store.Artifact `json:"artifact"`
	}

	// Approve.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"links_approved": true})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Artifact.LinksApproved)

	// Revoke.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"links_approved": false})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Artifact.LinksApproved)
}

// A non-bool links_approved must be a 400, not a stored value that later
// fails the bool column scan.
func TestPatchLinksApprovedRejectsNonBool(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Link Collector")

	for _, bad := range []any{"yes", 1, []string{"true"}} {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"links_approved": bad})
		assert.Equal(t, http.StatusBadRequest, w.Code, "value %#v must be rejected", bad)
	}

	// The artifact is still readable and unapproved.
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.LinksApproved)
}

// The whole point of the bridge is that the sandbox stays the wall: the
// detail-page iframe must still omit allow-popups and allow-top-navigation, so
// any vector the shim does not intercept stays browser-blocked regardless of
// approval state.
func TestDetailPageSandboxStillOmitsNavigationTokens(t *testing.T) {
	for _, approved := range []bool{false, true} {
		a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Exporter", Tier: store.Tier1,
			CreatedAt: time.Now(), LinksApproved: approved}
		page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
		require.NoError(t, err)

		start := strings.Index(page, "<iframe")
		require.GreaterOrEqual(t, start, 0, "detail page must embed the renderer iframe")
		iframeTag := page[start : start+strings.Index(page[start:], ">")]
		assert.Contains(t, iframeTag, `sandbox="allow-scripts allow-forms"`)
		assert.NotContains(t, iframeTag, "allow-popups",
			"approval must never relax the sandbox (approved=%v)", approved)
		assert.NotContains(t, iframeTag, "allow-top-navigation",
			"approval must never relax the sandbox (approved=%v)", approved)
	}
}

// The detail page is the bridge's host side: it validates the shim's navigation
// messages, gates them on linksApproved, and exposes the approval state via the
// bootstrap so a reload sees the persisted decision. The first-request modal is
// a later ticket (av-e3sj) — this only ships the gate and the pendingLink hook.
func TestDetailPageRendersLinkBridge(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Link Collector", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
	require.NoError(t, err)

	// Host-side message handler for the shim's navigation messages, plus the
	// gate variable and the pendingLink hook the confirmation modal consumes.
	assert.Contains(t, page, `<script src="/assets/gallery/detail.js"></script>`)
	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	assert.Contains(t, string(detailJS), "d.__avNavigate !== true")
	assert.Contains(t, string(detailJS), "linksApproved")
	assert.Contains(t, string(detailJS), "pendingLink")
	// The approval state is server-rendered, so a reload (or another device)
	// sees the persisted decision.
	assert.Contains(t, page, "let linksApproved = false;")

	// An approved artifact renders with the approval baked in.
	a.LinksApproved = true
	page, err = renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
	require.NoError(t, err)
	assert.Contains(t, page, "let linksApproved = true;")
}

// The first-request confirmation (av-e3sj) renders the approved copy verbatim,
// names the destination host, and the host logic opens the pending URL only
// after the grant persists. Denial drops the destination with nothing persisted
// — mirroring downloads (denial drops, approval persists).
func TestDetailPageRendersLinkConfirmationModal(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Link Collector", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
	require.NoError(t, err)

	// The modal mirrors dl-modal/clip-modal: an accessible dialog, the approved
	// copy verbatim, and a hostname span detail.js fills from the pending link.
	assert.Contains(t, page, `<div id="link-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `role="dialog"`)
	assert.Contains(t, page, `aria-modal="true"`)
	assert.Contains(t, page, `aria-labelledby="link-title"`)
	assert.Contains(t, page, `<h2 id="link-title">Allow opening links?</h2>`)
	assert.Contains(t, page,
		`You are opening a link to <code id="link-host">example.com</code>. Exhibit cannot verify the safety of external sites. Make sure you trust this destination before allowing links. You can revoke this at any time from the toolbar.`)
	assert.Contains(t, page, `id="link-allow"`)
	assert.Contains(t, page, `id="link-block"`)

	// The host logic fills the hostname, shows the modal on an unapproved link,
	// and opens the pending URL only after the grant persists server-side.
	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	assert.Contains(t, string(detailJS), `document.getElementById('link-host').textContent = url.hostname`)
	assert.Contains(t, string(detailJS), `document.getElementById('link-modal').hidden = false`)
	assert.Contains(t, string(detailJS), `setCapabilityApproved('links_approved', approved, 'link')`)
	assert.Contains(t, string(detailJS), `window.open(link.url, '_blank', 'noopener')`)
}
