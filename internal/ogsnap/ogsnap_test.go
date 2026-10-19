package ogsnap

import (
	"bytes"
	"compress/gzip"
	"context"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
)

// A snapshot is the widget's tile at preview size, and its page reaches the
// render origin and nothing else, even an origin the widget asks for. Every
// way out a page has is covered by chromium's own tests of ReachOnly; this one
// proves the snapshot is taken under it.
func TestSnapshotIsATileThatReachesOnlyTheRenderOrigin(t *testing.T) {
	var outsideHits atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outsideHits.Add(1)
		t.Logf("reached outside: %s", r.URL.Path)
	}))
	defer outside.Close()

	var assetHits, popupHits atomic.Int32
	render := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset.css":
			assetHits.Add(1)
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte("body{background:#123456}"))
		case "/popup":
			popupHits.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>fetch("` + outside.URL + `/popup-fetch").catch(()=>{})</script>`))
		default:
			// Compressed, as the render surface serves it.
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			_, _ = gz.Write([]byte(`<!doctype html><html><head>
<link rel="stylesheet" href="/asset.css">
</head><body>
<img src="` + outside.URL + `/pixel.png">
<script>
fetch("` + outside.URL + `/data").catch(()=>{});
window.open("/popup");
</script>
</body></html>`))
		}
	}))
	defer render.Close()

	browser, err := chromium.New(chromiumtest.Start(t))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := New(browser, render.URL)
	if err != nil {
		t.Fatal(err)
	}
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
	if assetHits.Load() == 0 || popupHits.Load() == 0 {
		t.Fatal("the render origin must stay reachable, from the page and from a popup it opens")
	}
	time.Sleep(300 * time.Millisecond)
	if n := outsideHits.Load(); n != 0 {
		t.Fatalf("the snapshot reached another origin %d time(s)", n)
	}
}

// The page itself must be on the render origin: the snapshotter is handed a
// URL the service built, and refusing anything else keeps a future caller from
// turning it into a general-purpose fetcher.
func TestSnapshotRefusesAPageOffTheRenderOrigin(t *testing.T) {
	browser, err := chromium.New("ws://unused")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := New(browser, "https://render.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{
		"https://elsewhere.test/s/x/widget",
		"https://render.test.elsewhere.test/s/x/widget",
		"https://render.test", // no path: not under the origin's prefix
	} {
		if _, err := snap.Snapshot(context.Background(), u); err == nil || !strings.Contains(err.Error(), "outside the render origin") {
			t.Errorf("Snapshot(%q) must refuse before reaching the browser, got %v", u, err)
		}
	}
}
