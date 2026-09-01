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

// camera_approved / microphone_approved are the media gate's first-use
// approvals (av-mv3k): PATCHed through the single write path, persisted so they
// survive reloads and devices, and revocable the same way.
func TestPatchMediaApprovals(t *testing.T) {
	for _, field := range []string{"camera_approved", "microphone_approved"} {
		t.Run(field, func(t *testing.T) {
			r := newTestRouter(t)
			id := createTestArtifact(t, r, "Recorder")

			read := func() store.Artifact {
				w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
				require.Equal(t, http.StatusOK, w.Code)
				var a store.Artifact
				require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
				return a
			}

			// New artifacts must never be pre-approved.
			a := read()
			assert.False(t, a.CameraApproved)
			assert.False(t, a.MicrophoneApproved)

			var resp struct {
				Artifact store.Artifact `json:"artifact"`
			}
			w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{field: true})
			require.Equal(t, http.StatusOK, w.Code)
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

			// One grant is one grant: approving a camera must never hand over a
			// microphone, which is why these are two columns and not one.
			if field == "camera_approved" {
				assert.True(t, resp.Artifact.CameraApproved)
				assert.False(t, resp.Artifact.MicrophoneApproved)
			} else {
				assert.True(t, resp.Artifact.MicrophoneApproved)
				assert.False(t, resp.Artifact.CameraApproved)
			}

			// Revoke.
			w = doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{field: false})
			require.Equal(t, http.StatusOK, w.Code)
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.False(t, resp.Artifact.CameraApproved)
			assert.False(t, resp.Artifact.MicrophoneApproved)
		})
	}
}

// A non-bool must be a 400, not a stored value that later fails the bool column
// scan and bricks every read of the artifact.
func TestPatchMediaApprovalsRejectNonBool(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Recorder")

	for _, field := range []string{"camera_approved", "microphone_approved"} {
		for _, bad := range []any{"yes", 1, []string{"true"}} {
			w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{field: bad})
			assert.Equal(t, http.StatusBadRequest, w.Code, "%s=%#v must be rejected", field, bad)
		}
	}

	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.CameraApproved)
	assert.False(t, a.MicrophoneApproved)
}

// The sandbox stays the wall. A device grant is honored by the render
// document's Permissions-Policy header — never by loosening the frame, and
// never by an allow= delegation, whose allowlist keys on the frame's opaque src
// origin and so matches nothing (measured: Chrome refuses it even with the
// auto-accept flag set — the same no-op the clipboard delegation turned out to
// be, av-hll6).
func TestDetailPageSandboxUnchangedByMediaApproval(t *testing.T) {
	for _, approved := range []bool{false, true} {
		a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Recorder", Tier: store.Tier1,
			CreatedAt: time.Now(), CameraApproved: approved, MicrophoneApproved: approved}
		page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
		require.NoError(t, err)

		start := strings.Index(page, "<iframe")
		require.GreaterOrEqual(t, start, 0, "detail page must embed the renderer iframe")
		iframeTag := page[start : start+strings.Index(page[start:], ">")]
		assert.Contains(t, iframeTag, `sandbox="allow-scripts allow-forms"`)
		assert.NotContains(t, iframeTag, "allow-same-origin",
			"approval must never relax the sandbox (approved=%v)", approved)
		assert.NotContains(t, iframeTag, "allow=",
			"a Permissions-Policy delegation onto an opaque origin is a no-op (approved=%v)", approved)
	}
}

// The detail page is the gate's host side: it validates the shim's media
// messages, prompts for the devices the request named, persists only those, and
// exposes the approval state through the bootstrap so a reload (or another
// device) sees the persisted decision. Approval opens the top-level render,
// which is the only place a capture device is reachable at all.
func TestDetailPageRendersMediaGate(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Recorder", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
	require.NoError(t, err)

	// The first-use prompt: an accessible dialog naming the devices asked for.
	assert.Contains(t, page, `<div id="media-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `aria-labelledby="media-title"`)
	assert.Contains(t, page, `<h2 id="media-title">Allow device access?</h2>`)
	assert.Contains(t, page, `<span id="media-devices">`)
	assert.Contains(t, page, `id="media-allow"`)
	assert.Contains(t, page, `id="media-block"`)
	// The copy must say where the grant is actually spent. An "Allow" that
	// appeared to enable the preview would be the worst available answer.
	assert.Contains(t, page, "Browsers cannot give a capture device to this embedded preview")

	assert.Contains(t, page, "let cameraApproved = false;")
	assert.Contains(t, page, "let microphoneApproved = false;")
	// The top-level render URL the gate opens, server-rendered rather than
	// rebuilt in page JS.
	assert.Contains(t, page, "const OPEN_URL =")

	detailJS, err := embeddedAssets.ReadFile("assets/gallery/detail.js")
	require.NoError(t, err)
	js := string(detailJS)
	assert.Contains(t, js, "d.__avMedia !== true")
	assert.Contains(t, js, "setCapabilityApproved('camera_approved', true, 'camera')")
	assert.Contains(t, js, "setCapabilityApproved('microphone_approved', true, 'microphone')")
	assert.Contains(t, js, "window.open(OPEN_URL, '_blank', 'noopener')")
	// Every path settles the artifact's promise — a getUserMedia left pending is
	// a hang, which is the failure this gate exists to remove. The three that
	// are easy to forget: an already-approved request (no prompt to show), a
	// request displaced by a newer one, and an outright denial.
	assert.Contains(t, js, "if (pendingMedia) replyMedia(pendingMedia.id, false, 'Permission denied', 'NotAllowedError');")
	assert.Contains(t, js, "if (deny && pendingMedia) replyMedia(pendingMedia.id, false, 'Permission denied', 'NotAllowedError');")
	assert.Contains(t, js, "replyMedia(req.id, true, 'Capture devices are unavailable in the embedded preview")

	// An approved artifact renders with the approvals baked in.
	a.CameraApproved, a.MicrophoneApproved = true, true
	page, err = renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds)
	require.NoError(t, err)
	assert.Contains(t, page, "let cameraApproved = true;")
	assert.Contains(t, page, "let microphoneApproved = true;")
}

// The edit page owns revocation (av-hwx2), so both devices need their own
// control there — and, like the link grant, each ships only when its select was
// actually touched: the bootstrap value goes stale the moment the viewer
// approves in another tab, and an unconditional write would revoke a newer
// grant on an unrelated save.
func TestEditPageRendersMediaControls(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Recorder", Tier: store.Tier1,
		CreatedAt: time.Now(), CameraApproved: true}
	page, err := renderEditPage(a, nil, "<html></html>", "", testPageCreds,
		testRenderURLs("https://render.example.com"), false, "")
	require.NoError(t, err)

	assert.Contains(t, page, `<select id="cam-select" class="select">`)
	assert.Contains(t, page, `<select id="mic-select" class="select">`)
	assert.Contains(t, page, "let cameraApproved = true;")
	assert.Contains(t, page, "let microphoneApproved = false;")

	editJS, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	js := string(editJS)
	assert.Contains(t, js, "if (cameraApprovedDirty) payload.camera_approved = cameraApproved;")
	assert.Contains(t, js, "if (microphoneApprovedDirty) payload.microphone_approved = microphoneApproved;")
}
