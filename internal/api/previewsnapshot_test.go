package api

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/ogsnap"
)

// Unset is every instance that runs no snapshot browser: no snapshotter, and
// no error, so previews quietly use the generic card.
func TestPreviewSnapshotterFromEnvUnset(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		t.Setenv(envPreviewSnapshotCDPURL, raw)
		snap, err := PreviewSnapshotterFromEnv("https://render.test")
		if err != nil || snap != nil {
			t.Fatalf("%q: got %v, %v; want no snapshotter and no error", raw, snap, err)
		}
	}
}

func TestPreviewSnapshotterFromEnvAcceptsDevToolsURLs(t *testing.T) {
	for _, raw := range []string{"ws://chrome:9222", "wss://chrome.internal/", "http://127.0.0.1:9222", "https://chrome.example"} {
		t.Setenv(envPreviewSnapshotCDPURL, raw)
		snap, err := PreviewSnapshotterFromEnv("https://render.test")
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if _, ok := snap.(*ogsnap.Chromium); !ok {
			t.Fatalf("%q: got %T, want *ogsnap.Chromium", raw, snap)
		}
	}
}

// The operator asked for previews; a value that cannot be a browser endpoint
// must stop the start rather than leave them wondering why every preview is
// the logo.
func TestPreviewSnapshotterFromEnvRefusesNonsense(t *testing.T) {
	for _, raw := range []string{"chrome:9222", "ftp://chrome:9222", "ws://", "not a url"} {
		t.Setenv(envPreviewSnapshotCDPURL, raw)
		if _, err := PreviewSnapshotterFromEnv("https://render.test"); err == nil {
			t.Fatalf("%q must be refused", raw)
		}
	}
}

// The render origin serves both preview routes, and the generic card is the
// one the asset build rasterized.
func TestRenderHandlerServesThePreviewCard(t *testing.T) {
	if len(previewImagePNG()) == 0 {
		t.Fatal("the asset build did not produce assets/icons/og-default.png")
	}
	r := newTestRouter(t)
	w := renderGet(t, r, "/og.png")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("/og.png: got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	w = renderGet(t, r, "/s/never-issued/og.png")
	if w.Code != http.StatusNotFound {
		t.Fatalf("/s/never-issued/og.png: got %d, want 404", w.Code)
	}

	// A live link with no snapshot browser configured shows the card too.
	id := createArtifact(t, r, map[string]any{
		"title": "Run Log", "body": "<html><head></head><body></body></html>", "network_allowlist": []string{},
	})
	w = renderGet(t, r, "/s/"+createShare(t, r, id)+"/og.png")
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), previewImagePNG()) {
		t.Fatalf("a live link's preview image: got %d, %d bytes", w.Code, w.Body.Len())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("a live link's preview image: Cache-Control = %q, want no-store", cc)
	}
}
