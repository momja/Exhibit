package ogsnap

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	// Local Network Access would refuse a stamped document's requests to the
	// test servers, which are all on loopback. That protection is Chromium's,
	// not ours, and an operator's browser may not apply it, so it is off here
	// and the confinement has to stand without it.
	cmd := exec.CommandContext(ctx, bin, "--headless", "--no-sandbox", "--disable-gpu",
		"--disable-features=LocalNetworkAccessChecks",
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
// else, even though nothing in the page's own policy forbids the rest. That is
// the property that keeps a server-side browser from being an SSRF whose answer
// is published as a picture, so the widget here tries every way out the
// confinement has to cover: subresources and fetch from the page; a WebSocket,
// which request interception never sees; and the targets a page can start —
// a popup on the render origin, an about:blank popup it writes into, a srcdoc
// frame, blob: and data: workers, and a service worker.
func TestSnapshotReachesTheRenderOriginAndNothingElse(t *testing.T) {
	ws := startChromium(t)

	var outsideHits atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outsideHits.Add(1)
		t.Logf("reached outside: %s", r.URL.Path)
	}))
	defer outside.Close()
	out, wsOut := outside.URL, "ws"+strings.TrimPrefix(outside.URL, "http")

	var assetHits, popupHits, workerScriptHits atomic.Int32
	render := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset.css":
			assetHits.Add(1)
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte("body{background:#123456}"))
		case "/popup":
			popupHits.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>fetch("` + out + `/popup-fetch").catch(()=>{});new WebSocket("` + wsOut + `/popup-ws")</script>`))
		case "/sw.js":
			workerScriptHits.Add(1)
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte(`self.addEventListener("install", e => e.waitUntil(new Promise(done => {
  fetch("` + out + `/sw-fetch").catch(()=>{}); new WebSocket("` + wsOut + `/sw-ws"); setTimeout(done, 300);
})));`))
		default:
			// Compressed, as the render surface serves it: the stamped copy
			// is re-sent decoded and must still run.
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			_, _ = gz.Write([]byte(`<!doctype html><html><head>
<link rel="stylesheet" href="/asset.css">
<link rel="stylesheet" href="` + out + `/style.css">
</head><body>
<img src="` + out + `/pixel.png">
<script>
fetch("` + out + `/data").catch(()=>{});
new WebSocket("` + wsOut + `/page-ws");
window.open("/popup");
var blank = window.open("about:blank");
if (blank) blank.document.write('<scr' + 'ipt>fetch("` + out + `/blank-fetch").catch(()=>{});new WebSocket("` + wsOut + `/blank-ws")</scr' + 'ipt>');
var frame = document.createElement("iframe");
frame.srcdoc = '<scr' + 'ipt>new WebSocket("` + wsOut + `/srcdoc-ws")</scr' + 'ipt>';
document.body.appendChild(frame);
new Worker(URL.createObjectURL(new Blob(['fetch("` + out + `/blob-worker").catch(()=>{});new WebSocket("` + wsOut + `/blob-worker-ws")'])));
new Worker("data:text/javascript,new WebSocket('` + wsOut + `/data-worker-ws')");
navigator.serviceWorker.register("/sw.js").catch(()=>{});
</script>
</body></html>`))
		}
	}))
	defer render.Close()

	// The http form of the endpoint, so the snapshot also discovers the
	// browser's address the way a configured sidecar is reached.
	endpoint, err := url.Parse(ws)
	if err != nil {
		t.Fatal(err)
	}
	snap := NewChromium("http://"+endpoint.Host, render.URL)
	png, err := snap.Snapshot(context.Background(), render.URL+"/s/x/widget")
	if err != nil {
		t.Fatal(err)
	}
	img, err := pngDecode(png)
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != ImageWidth || b.Dy() != ImageHeight {
		t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), ImageWidth, ImageHeight)
	}
	if assetHits.Load() == 0 {
		t.Fatal("a render-origin subresource must load, or the confinement is refusing everything")
	}
	if popupHits.Load() == 0 || workerScriptHits.Load() == 0 {
		t.Fatal("the widget's script must have run and started its popup and service worker, or this test proves nothing")
	}
	// Anything that got out may still be in flight when the snapshot returns.
	time.Sleep(500 * time.Millisecond)
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

func pngDecode(b []byte) (image.Image, error) { return png.Decode(bytes.NewReader(b)) }

// recordingTransport answers /json/version the way Chromium does, with an
// address built from where the browser thinks it is, and records each request.
type recordingTransport struct{ requested []string }

func (rt *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.requested = append(rt.requested, r.URL.String())
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"webSocketDebuggerUrl":"ws://10.9.9.9:9222/devtools/browser/abc"}`)),
		Request:    r,
	}, nil
}

// The endpoint's scheme decides TLS for discovery and for the connection, a
// missing port is the scheme's default, and the address Chromium reports is
// trusted for its path only: its host is wherever the browser thinks it is,
// which behind a proxy is not where Exhibit reaches it.
func TestDiscoveryKeepsTheEndpointsSchemeAndHost(t *testing.T) {
	for _, tc := range []struct{ endpoint, discovery, want string }{
		{"https://chrome.example", "https://chrome.example/json/version", "wss://chrome.example/devtools/browser/abc"},
		{"wss://chrome.example:9443/", "https://chrome.example:9443/json/version", "wss://chrome.example:9443/devtools/browser/abc"},
		{"http://localhost:9222", "http://localhost:9222/json/version", "ws://localhost:9222/devtools/browser/abc"},
		{"ws://127.0.0.1", "http://127.0.0.1/json/version", "ws://127.0.0.1/devtools/browser/abc"},
		{"wss://chrome.example/devtools/browser/xyz", "", "wss://chrome.example/devtools/browser/xyz"},
		{"http://[::1]:9222/devtools/browser/xyz", "", "ws://[::1]:9222/devtools/browser/xyz"},
	} {
		rt := &recordingTransport{}
		c := NewChromium(tc.endpoint, "https://render.test")
		c.discovery = &http.Client{Transport: rt}
		got, err := c.browserWebSocketURL(context.Background())
		if err != nil {
			t.Errorf("%s: %v", tc.endpoint, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: connects to %q, want %q", tc.endpoint, got, tc.want)
		}
		if asked := strings.Join(rt.requested, " "); asked != tc.discovery {
			t.Errorf("%s: asked %q, want %q", tc.endpoint, asked, tc.discovery)
		}
	}
}

// Against a real TLS listener: discovery over https completes, which it could
// not if it spoke plaintext to the port.
func TestDiscoveryOverTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`))
	}))
	defer srv.Close()

	c := NewChromium(srv.URL, "https://render.test")
	c.discovery = srv.Client()
	got, err := c.browserWebSocketURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "wss://" + strings.TrimPrefix(srv.URL, "https://") + "/devtools/browser/abc"; got != want {
		t.Fatalf("connects to %q, want %q", got, want)
	}
}
