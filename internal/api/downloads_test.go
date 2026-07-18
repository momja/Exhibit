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

// downloads_approved is the download bridge's first-use approval (av-ryby):
// PATCHed through the single write path, persisted server-side so it survives
// reloads and devices, and revocable the same way.
func TestPatchDownloadsApproved(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "CSV Exporter")

	// New artifacts must never be pre-approved.
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.DownloadsApproved)

	// PATCH wraps the artifact alongside the re-scan footprint (updateArtifactResponse).
	var resp struct {
		Artifact store.Artifact `json:"artifact"`
	}

	// Approve.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"downloads_approved": true})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Artifact.DownloadsApproved)

	// Revoke.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"downloads_approved": false})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Artifact.DownloadsApproved)
}

// A non-bool downloads_approved must be a 400, not a stored value that later
// fails the bool column scan.
func TestPatchDownloadsApprovedRejectsNonBool(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "CSV Exporter")

	for _, bad := range []any{"yes", 1, []string{"true"}} {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"downloads_approved": bad})
		assert.Equal(t, http.StatusBadRequest, w.Code, "value %#v must be rejected", bad)
	}

	// The artifact is still readable and unapproved.
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.DownloadsApproved)
}

// The whole point of the bridge is that the sandbox stays the wall: the
// detail-page iframe must still omit allow-downloads, so any vector the shim
// does not intercept stays browser-blocked regardless of approval state.
func TestDetailPageSandboxStillOmitsAllowDownloads(t *testing.T) {
	for _, approved := range []bool{false, true} {
		a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Exporter", Tier: store.Tier1,
			CreatedAt: time.Now(), DownloadsApproved: approved}
		page, err := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
		require.NoError(t, err)

		start := strings.Index(page, "<iframe")
		require.GreaterOrEqual(t, start, 0, "detail page must embed the renderer iframe")
		iframeTag := page[start : start+strings.Index(page[start:], ">")]
		assert.Contains(t, iframeTag, `sandbox="allow-scripts"`)
		assert.NotContains(t, iframeTag, "allow-downloads",
			"approval must never relax the sandbox (approved=%v)", approved)
	}
}

// The detail page is the bridge's host side: it validates the shim's download
// messages, prompts on first use (naming the artifact and filename), persists
// the decision via PATCH, and exposes a revoke control in the toolbar.
func TestDetailPageRendersDownloadBridge(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "CSV <Exporter>", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
	require.NoError(t, err)

	// Host-side message handler for the shim's download messages, and the
	// PATCH that persists the decision — both live in the static page script
	// the detail page loads.
	assert.Contains(t, page, `<script src="/assets/gallery/detail.js"></script>`)
	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	assert.Contains(t, string(detailJS), "d.__avDownload !== true")
	assert.Contains(t, string(detailJS), "downloads_approved")
	// The approval state is server-rendered, so a reload (or another device)
	// sees the persisted decision.
	assert.Contains(t, page, "let downloadsApproved = false;")
	// First-use prompt names the artifact (HTML-escaped) and the filename.
	assert.Contains(t, page, `<div id="dl-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `<strong>CSV &lt;Exporter&gt;</strong> wants to download <code id="dl-filename"></code>`)
	assert.Contains(t, page, `id="dl-allow"`)
	assert.Contains(t, page, `id="dl-block"`)
	// Toolbar shows the state and offers revocation.
	assert.Contains(t, page, `id="dl-state"`)
	assert.Contains(t, page, `id="dl-revoke"`)

	// An approved artifact renders with the approval baked in.
	a.DownloadsApproved = true
	page, err = renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
	require.NoError(t, err)
	assert.Contains(t, page, "let downloadsApproved = true;")
}
