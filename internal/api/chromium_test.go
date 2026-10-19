package api

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/ogsnap"
)

// Unset is every instance that runs no browser: none, and no error, so link
// previews quietly use the generic card.
func TestChromiumFromEnvUnset(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		t.Setenv(envChromiumCDPURL, raw)
		b, err := ChromiumFromEnv()
		if err != nil || b != nil {
			t.Fatalf("%q: got %v, %v; want no browser and no error", raw, b, err)
		}
	}
}

func TestChromiumFromEnvAcceptsDevToolsURLs(t *testing.T) {
	for _, raw := range []string{"ws://chrome:9222", "wss://chrome.internal/", "http://127.0.0.1:9222", "https://chrome.example"} {
		t.Setenv(envChromiumCDPURL, raw)
		if b, err := ChromiumFromEnv(); err != nil || b == nil {
			t.Fatalf("%q: got %v, %v", raw, b, err)
		}
	}
}

// The operator asked for a browser; a value that cannot be one must stop the
// start rather than leave every feature quietly falling back.
func TestChromiumFromEnvRefusesNonsense(t *testing.T) {
	for _, raw := range []string{"chrome:9222", "ftp://chrome:9222", "ws://", "not a url"} {
		t.Setenv(envChromiumCDPURL, raw)
		if _, err := ChromiumFromEnv(); err == nil {
			t.Fatalf("%q must be refused", raw)
		}
	}
}

// With a browser configured, link previews draw in it; without one they use
// the generic card. A nil *ogsnap.Snapshotter inside a non-nil interface would
// look like a browser to the render surface and fail every preview.
func TestPreviewsDrawInTheBrowserWhenThereIsOne(t *testing.T) {
	r := &Router{cfg: Config{RenderOrigin: "https://render.test"}}
	if snap := r.previewSnapshotter(); snap != nil {
		t.Fatalf("no browser: got %T, want nil", snap)
	}
	b, err := chromium.New("ws://chrome:9222")
	if err != nil {
		t.Fatal(err)
	}
	r.cfg.Chromium = b
	if _, ok := r.previewSnapshotter().(*ogsnap.Snapshotter); !ok {
		t.Fatalf("with a browser: got %T, want *ogsnap.Snapshotter", r.previewSnapshotter())
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
