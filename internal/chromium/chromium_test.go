package chromium

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
)

func TestNewTakesDevToolsEndpointsOnly(t *testing.T) {
	for _, ok := range []string{"ws://chrome:9222", "wss://chrome.internal/", "http://127.0.0.1:9222", "https://chrome.example"} {
		if _, err := New(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"chrome:9222", "ftp://chrome:9222", "ws://", "not a url", ""} {
		if _, err := New(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

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
// which behind a proxy is not where the service reaches it.
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
		b, err := New(tc.endpoint)
		if err != nil {
			t.Fatal(err)
		}
		b.discovery = &http.Client{Transport: rt}
		got, err := b.webSocketURL(context.Background())
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

	b, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	b.discovery = srv.Client()
	got, err := b.webSocketURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := "wss://" + strings.TrimPrefix(srv.URL, "https://") + "/devtools/browser/abc"; got != want {
		t.Fatalf("connects to %q, want %q", got, want)
	}
}

func TestReachOnlyTakesOriginsOnly(t *testing.T) {
	for _, bad := range []string{"render.test", "ftp://render.test", "https://render.test/path", "https://render.test/?q", "https://user@render.test", "https://"} {
		if _, err := ReachOnly(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if _, err := ReachOnly("https://render.test/"); err != nil {
		t.Errorf("a trailing slash is still an origin: %v", err)
	}
}

// outsideServer counts every request that reaches it, WebSocket handshakes
// included, and returns its http and ws base URLs.
func outsideServer(t *testing.T) (*atomic.Int32, string, string) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Logf("reached outside: %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return &hits, srv.URL, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func newBrowser(t *testing.T, flags ...string) *Browser {
	t.Helper()
	b, err := New(chromiumtest.Start(t, flags...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A confined context reaches its origins and nothing else, by every way a page
// has of reaching out: subresources and fetch; a WebSocket, which request
// interception would never see; a top-level navigation; and the targets a page
// can start — a popup on the allowed origin, an about:blank popup it writes
// into, a srcdoc frame, blob: and data: workers, and a service worker. The
// outside server is on loopback too, so this also proves loopback is not
// exempt: Chromium's default exemption would hand a page the browser's own
// DevTools port.
func TestReachOnlyConfinesEverythingTheContextStarts(t *testing.T) {
	b := newBrowser(t)
	outsideHits, out, wsOut := outsideServer(t)

	var assetHits, popupHits, workerScriptHits atomic.Int32
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head>
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
	defer allowed.Close()

	reach, err := ReachOnly(allowed.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tab, closeTab, err := b.Tab(ctx, reach)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chromedp.Run(tab, chromedp.Navigate(allowed.URL+"/page")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	// A top-level navigation off the allowed origin fails at the network.
	_, _ = chromedp.Run(tab, chromedp.Navigate(out+"/navigation"))
	closeTab()

	if assetHits.Load() == 0 {
		t.Fatal("a subresource on the allowed origin must load, or the confinement is refusing everything")
	}
	if popupHits.Load() == 0 || workerScriptHits.Load() == 0 {
		t.Fatal("the page's script must have started its popup and service worker, or this test proves nothing")
	}
	// Anything that got out may still be in flight.
	time.Sleep(300 * time.Millisecond)
	if n := outsideHits.Load(); n != 0 {
		t.Fatalf("the confined context reached another origin %d time(s)", n)
	}
}

// A restriction belongs to the context it was given to. Another feature's tab
// in the same browser, open at the same time, reaches the network as usual.
func TestReachOnlyLeavesOtherTabsAlone(t *testing.T) {
	b := newBrowser(t)
	outsideHits, out, _ := outsideServer(t)
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><p>confined</p>`))
	}))
	defer allowed.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reach, err := ReachOnly(allowed.URL)
	if err != nil {
		t.Fatal(err)
	}
	confined, closeConfined, err := b.Tab(ctx, reach)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConfined()
	if _, err := chromedp.Run(confined, chromedp.Navigate(allowed.URL)); err != nil {
		t.Fatal(err)
	}

	plain, closePlain, err := b.Tab(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closePlain()
	if _, err := chromedp.Run(plain, chromedp.Navigate(out+"/unconfined")); err != nil {
		t.Fatal(err)
	}
	if outsideHits.Load() == 0 {
		t.Fatal("a tab with no restriction was confined by another tab's")
	}
}

// An origin on its scheme's default port is allowed on that port only: the
// rule names the port, so another port on the same host stays unreachable.
// The host resolver sends render.test, on any port, to the test server.
func TestReachOnlyNamesTheDefaultPort(t *testing.T) {
	var pageHits, otherPortHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "render.test:8081" {
			otherPortHits.Add(1)
			return
		}
		pageHits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<script>fetch("http://render.test:8081/other-port", {mode: "no-cors"}).catch(()=>{})</script>`))
	}))
	defer srv.Close()
	b := newBrowser(t, "--host-resolver-rules=MAP render.test "+strings.TrimPrefix(srv.URL, "http://"))

	reach, err := ReachOnly("http://render.test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tab, closeTab, err := b.Tab(ctx, reach)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTab()
	if _, err := chromedp.Run(tab, chromedp.Navigate("http://render.test/")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if pageHits.Load() == 0 {
		t.Fatal("http://render.test, on its default port, must be reachable")
	}
	if otherPortHits.Load() != 0 {
		t.Fatal("another port on the allowed host was reachable")
	}
}
