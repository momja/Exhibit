package ogsnap

import (
	"bufio"
	"bytes"
	"context"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// startChromium launches a headless Chromium with a DevTools port and returns
// its websocket URL. The binary comes from OGSNAP_CHROMIUM, else a Playwright
// headless shell if one is installed; with neither the test is skipped, since
// the browser is an optional sidecar and not a build dependency.
func startChromium(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("OGSNAP_CHROMIUM")
	if bin == "" {
		home, _ := os.UserHomeDir()
		matches, _ := filepath.Glob(filepath.Join(home, ".cache/ms-playwright/chromium_headless_shell-*/*/chrome-headless-shell"))
		if len(matches) > 0 {
			bin = matches[len(matches)-1]
		}
	}
	if bin == "" {
		t.Skip("no Chromium: set OGSNAP_CHROMIUM to run the snapshot tests")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "--headless", "--no-sandbox", "--disable-gpu",
		"--remote-debugging-port=0", "--user-data-dir="+t.TempDir())
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })

	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if _, ws, ok := strings.Cut(sc.Text(), "DevTools listening on "); ok {
				found <- strings.TrimSpace(ws)
			}
		}
	}()
	select {
	case ws := <-found:
		return ws
	case <-time.After(15 * time.Second):
		t.Fatal("Chromium did not report a DevTools endpoint")
		return ""
	}
}

// A snapshot loads the page and what the render origin serves, and nothing
// else: the widget's attempts at another origin — a fetch, an image, a
// stylesheet — never arrive there, even though nothing in the page's own
// policy forbids them. That is the property that keeps a server-side browser
// from being an SSRF whose answer is published as a picture.
func TestSnapshotReachesTheRenderOriginAndNothingElse(t *testing.T) {
	ws := startChromium(t)

	var outsideHits atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outsideHits.Add(1)
	}))
	defer outside.Close()

	var assetHits atomic.Int32
	render := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/asset.css" {
			assetHits.Add(1)
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte("body{background:#123456}"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
<link rel="stylesheet" href="/asset.css">
<link rel="stylesheet" href="` + outside.URL + `/style.css">
</head><body>
<img src="` + outside.URL + `/pixel.png">
<script>fetch("` + outside.URL + `/data").catch(function(){})</script>
</body></html>`))
	}))
	defer render.Close()

	snap := NewChromium(ws, render.URL)
	out, err := snap.Snapshot(context.Background(), render.URL+"/s/x/widget")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != ImageWidth || b.Dy() != ImageHeight {
		t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), ImageWidth, ImageHeight)
	}
	if assetHits.Load() == 0 {
		t.Fatal("a render-origin subresource must load, or the confinement is refusing everything")
	}
	if n := outsideHits.Load(); n != 0 {
		t.Fatalf("the snapshot reached another origin %d time(s)", n)
	}
}

// The page itself must be on the render origin: the snapshotter is handed a
// URL the service built, and refusing anything else keeps a future caller from
// turning it into a general-purpose fetcher.
func TestSnapshotRefusesAPageOffTheRenderOrigin(t *testing.T) {
	snap := NewChromium("ws://unused", "https://render.test")
	for _, u := range []string{
		"https://elsewhere.test/s/x/widget",
		"https://render.test.elsewhere.test/s/x/widget",
		"https://render.test", // no path: not under the origin's prefix
	} {
		if _, err := snap.Snapshot(context.Background(), u); err == nil {
			t.Errorf("Snapshot(%q) must refuse", u)
		}
	}
}
