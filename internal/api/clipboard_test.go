package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clipboard_approved is the clipboard bridge's first-use approval (av-hll6),
// the sibling of downloads_approved: PATCHed through the single write path,
// persisted server-side so it survives reloads and devices, and revocable.
func TestPatchClipboardApproved(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Clipboard Tool")

	// New artifacts must never be pre-approved.
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.ClipboardApproved)

	// PATCH wraps the artifact alongside the re-scan footprint (updateArtifactResponse).
	var resp struct {
		Artifact store.Artifact `json:"artifact"`
	}

	// Approve.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"clipboard_approved": true})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Artifact.ClipboardApproved)

	// Revoke.
	w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"clipboard_approved": false})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Artifact.ClipboardApproved)
}

// A non-bool clipboard_approved must be a 400, not a stored value that later
// fails the bool column scan.
func TestPatchClipboardApprovedRejectsNonBool(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Clipboard Tool")

	for _, bad := range []any{"yes", 1, []string{"true"}} {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"clipboard_approved": bad})
		assert.Equal(t, http.StatusBadRequest, w.Code, "value %#v must be rejected", bad)
	}

	// The artifact is still readable and unapproved.
	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.ClipboardApproved)
}

// The detail page is the clipboard bridge's host side: it validates the shim's
// clipboard messages, prompts on first use (naming the artifact), persists the
// decision via PATCH, and exposes a revoke control in the toolbar.
func TestDetailPageRendersClipboardBridge(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Copy <Tool>", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page := renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")

	// Host-side handler for the shim's clipboard messages, and the result reply.
	assert.Contains(t, page, "d.__avClipboard !== true")
	assert.Contains(t, page, "__avClipboardResult")
	// The approval state is server-rendered, so a reload (or another device)
	// sees the persisted decision.
	assert.Contains(t, page, "let clipboardApproved = false;")
	// First-use prompt names the artifact (HTML-escaped).
	assert.Contains(t, page, `<div id="clip-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `<strong>Copy &lt;Tool&gt;</strong> wants to`)
	assert.Contains(t, page, `id="clip-allow"`)
	assert.Contains(t, page, `id="clip-block"`)
	// Toolbar shows the state and offers revocation.
	assert.Contains(t, page, `id="clip-state"`)
	assert.Contains(t, page, `id="clip-revoke"`)
	assert.Contains(t, page, "clipboard_approved")

	// An approved artifact renders with the approval baked in.
	a.ClipboardApproved = true
	page = renderDetailPage(a, "<p>src</p>", "https://render.example.com", "tok")
	assert.Contains(t, page, "let clipboardApproved = true;")
}
