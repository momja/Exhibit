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

// geolocation_approved is the location gate's first-use approval (av-f446):
// PATCHed through the single write path, persisted, revocable, and a grant of
// its own. Approving location approves no capture device, and approving a
// device approves no location.
func TestPatchGeolocationApproval(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run tracker")

	patch := func(body map[string]any) store.Artifact {
		t.Helper()
		var resp struct {
			Artifact store.Artifact `json:"artifact"`
		}
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		return resp.Artifact
	}

	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var fresh store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&fresh))
	assert.False(t, fresh.GeolocationApproved, "new artifacts must never be pre-approved for location")

	a := patch(map[string]any{"geolocation_approved": true})
	assert.True(t, a.GeolocationApproved)
	assert.False(t, a.CameraApproved, "a location grant must not carry the camera")
	assert.False(t, a.MicrophoneApproved, "a location grant must not carry the microphone")

	// Revoke location while approving both devices: the devices must not
	// bring location back with them.
	a = patch(map[string]any{"geolocation_approved": false, "camera_approved": true, "microphone_approved": true})
	assert.False(t, a.GeolocationApproved, "a device grant must not carry location")
	assert.True(t, a.CameraApproved)
	assert.True(t, a.MicrophoneApproved)
}

// A non-bool must be a 400, not a stored value that later fails the bool column
// scan and bricks every read of the artifact.
func TestPatchGeolocationApprovalRejectsNonBool(t *testing.T) {
	r := newTestRouter(t)
	id := createTestArtifact(t, r, "Run tracker")

	for _, bad := range []any{"yes", 1, []string{"true"}} {
		w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"geolocation_approved": bad})
		assert.Equal(t, http.StatusBadRequest, w.Code, "geolocation_approved=%#v must be rejected", bad)
	}

	w := doJSON(t, r, "GET", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var a store.Artifact
	require.NoError(t, json.NewDecoder(w.Body).Decode(&a))
	assert.False(t, a.GeolocationApproved)
}

// The frame stays exactly as sandboxed as before. Unlike a camera, a location
// *could* be delegated into it: Chromium honors allow="geolocation" on an
// opaque-origin frame (measured, Chromium 149), but the frame then spends the
// app origin's grant, which would make the library itself the holder of the
// visitor's location. The approval is spent on the top-level render instead,
// so no approval state may ever add the delegation.
func TestDetailPageSandboxUnchangedByGeolocationApproval(t *testing.T) {
	for _, approved := range []bool{false, true} {
		a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Run tracker", Tier: store.Tier1,
			CreatedAt: time.Now(), GeolocationApproved: approved}
		page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
		require.NoError(t, err)

		start := strings.Index(page, "<iframe")
		require.GreaterOrEqual(t, start, 0, "detail page must embed the renderer iframe")
		iframeTag := page[start : start+strings.Index(page[start:], ">")]
		assert.Contains(t, iframeTag, `sandbox="allow-scripts allow-forms"`)
		assert.NotContains(t, iframeTag, "allow-same-origin",
			"approval must never relax the sandbox (approved=%v)", approved)
		assert.NotContains(t, iframeTag, "allow=",
			"a geolocation delegation would spend the app origin's grant (approved=%v)", approved)
	}
}

// The detail page renders the location prompt and the bootstrap flag the gate
// reads. What the prompt does is driven in web/gallery/detail.geolocation.test.mjs;
// this half pins the markup ids that script resolves, which that harness
// creates on demand and so cannot check.
func TestDetailPageRendersGeolocationGate(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Run tracker", Tier: store.Tier1,
		CreatedAt: time.Now()}
	page, err := renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)

	assert.Contains(t, page, `<div id="geo-modal" class="modal-overlay" hidden>`)
	assert.Contains(t, page, `aria-labelledby="geo-title"`)
	assert.Contains(t, page, `<h2 id="geo-title">Allow location access?</h2>`)
	assert.Contains(t, page, `id="geo-allow"`)
	assert.Contains(t, page, `id="geo-block"`)
	// The copy says where the approval is spent, and does not claim browsers
	// are unable to share a location here, which would be false of Chromium.
	assert.Contains(t, page, "The embedded preview does not share your location with artifacts")
	assert.Contains(t, page, "let geolocationApproved = false;")

	a.GeolocationApproved = true
	page, err = renderDetailPage(a, testRenderURLs("https://render.example.com"), testPageCreds, nil)
	require.NoError(t, err)
	assert.Contains(t, page, "let geolocationApproved = true;")
}

// The edit page owns revocation (av-hwx2), so location gets its own control
// there, and like the device selects it ships only when it was touched: the
// bootstrap value goes stale the moment the viewer approves in another tab.
func TestEditPageRendersGeolocationControl(t *testing.T) {
	a := &store.Artifact{ID: "abc123", OwnerID: 1, Title: "Run tracker", Tier: store.Tier1,
		CreatedAt: time.Now(), GeolocationApproved: true}
	page, err := renderEditPage(a, nil, nil, "<html></html>", "", testPageCreds,
		testRenderURLs("https://render.example.com"), false, "", nil)
	require.NoError(t, err)

	assert.Contains(t, page, `<select id="geo-select" class="select">`)
	assert.Contains(t, page, "let geolocationApproved = true;")

	editJS, err := embeddedAssets.ReadFile("assets/gallery/edit.js")
	require.NoError(t, err)
	assert.Contains(t, string(editJS),
		"if (geolocationApprovedDirty) payload.geolocation_approved = geolocationApproved;")
}
