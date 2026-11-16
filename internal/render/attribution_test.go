package render

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShareAttributionOwnerChoiceAndLegacyBytes(t *testing.T) {
	rd, st := newTestRenderer(t, "attribution", `<!doctype html><html><head><title>Tool</title></head><body><button>Run</button></body></html>`)
	link, err := st.CreateAnonymousLink(context.Background(), 1, "attribution")
	require.NoError(t, err)
	baseline := serveShare(t, rd, link.ID)
	require.Equal(t, "779292f5c5d37f06ea83999e7e139db1c8251bc71852a01351bdac22738bcc3a", fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(baseline.Body.String(), link.ID, "public")))))
	require.Equal(t, "31b51fb4673416e2c321dae18b3e0f5e9a456a4df42f02d0ada2e094dc5198f9", fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprint(baseline.Header())))))
	private := serve(t, rd, "attribution")
	for _, instance := range []bool{false, true} {
		rd.cfg.ShareAttributionEnabled = instance
		for _, mode := range []string{"inherit", "show", "hide"} {
			require.NoError(t, st.UpdateArtifact(context.Background(), 1, "attribution", map[string]any{"share_attribution": mode}))
			got := serveShare(t, rd, link.ID)
			show := mode == "show" || mode == "inherit" && instance
			if show {
				require.Contains(t, got.Body.String(), `id="exhibit-attribution" href="`+ExhibitProductOrigin+`"`)
				require.Contains(t, got.Body.String(), `sandbox="allow-scripts allow-forms" allow="fullscreen *; clipboard-write *"`)
				require.Contains(t, got.Header().Get("Content-Security-Policy"), versionSandbox)
			} else {
				require.Equal(t, baseline.Body.String(), got.Body.String())
				require.Equal(t, baseline.Header(), got.Header())
			}
			require.Equal(t, private, serve(t, rd, "attribution"))
		}
	}
}

// Every document route besides the live share ignores both branding controls.
func TestAttributionDoesNotChangeOtherRenderDocuments(t *testing.T) {
	rd, st, link, _ := newShareWidgetFixture(t)
	snapshot := func() []string {
		widget := serveWidget(t, rd, "abc")
		sharedWidget := serveShareWidgetRaw(t, rd, link)
		return []string{serve(t, rd, "abc"), widget.Body.String(), fmt.Sprint(widget.Header()), sharedWidget.Body.String(), fmt.Sprint(sharedWidget.Header())}
	}
	before := snapshot()
	rd.cfg.ShareAttributionEnabled = true
	require.NoError(t, st.UpdateArtifact(context.Background(), 1, "abc", map[string]any{"share_attribution": "show"}))
	require.Equal(t, before, snapshot())
	versioned, versions := newVersionedRenderer(t)
	old := serveVersion(versioned, "abc", 1, versionToken("abc", 1, 1))
	versioned.cfg.ShareAttributionEnabled = true
	require.NoError(t, versions.UpdateArtifact(context.Background(), 1, "abc", map[string]any{"share_attribution": "show"}))
	now := serveVersion(versioned, "abc", 1, versionToken("abc", 1, 1))
	require.Equal(t, old.Body.String(), now.Body.String())
	require.Equal(t, old.Header(), now.Header())
}
