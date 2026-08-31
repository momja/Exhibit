package render

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
)

// newWidgetRenderer builds a Renderer holding one artifact that has both a body
// and a widget, plus one state key so inlining is observable.
func newWidgetRenderer(t *testing.T, id, body, widget string) *Renderer {
	t.Helper()

	dbf, err := os.CreateTemp(t.TempDir(), "widget-*.db")
	if err != nil {
		t.Fatal(err)
	}
	dbf.Close()
	st, err := store.OpenSQLite(dbf.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	bl, err := blob.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := bl.Put(ctx, id+"-body", strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	widgetBlob := ""
	if widget != "" {
		widgetBlob = id + "-widget"
		if err := bl.Put(ctx, widgetBlob, strings.NewReader(widget)); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.PutArtifact(ctx, &store.Artifact{
		ID: id, OwnerID: 1, Title: "t", SourceBlobID: id + "-body", Tier: 1,
		WidgetBlobID:     widgetBlob,
		NetworkAllowlist: []string{"https://api.example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetState(ctx, 1, id, 1, "runs", `[{"km":5}]`); err != nil {
		t.Fatal(err)
	}

	return New(Config{
		Store: st, Blob: bl,
		AppOrigin: "https://app.test", RenderOrigin: "https://render.test",
		Tokens: testTokens,
	})
}

func serveWidget(t *testing.T, rd *Renderer, id string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	rd.ServeWidget(w, renderRequest("/w/"+id, id, 1))
	return w
}

// The widget is a *view* of the artifact's state, so the render must inline the
// same state the artifact gets. This is the whole point of the feature: a
// gallery tile that shows the tool's real, cross-device data without the tool.
func TestServeWidgetInlinesArtifactState(t *testing.T) {
	rd := newWidgetRenderer(t, "abc", "<html><head></head><body>ARTIFACT-BODY</body></html>",
		"<html><head></head><body>WIDGET-BODY</body></html>")

	w := serveWidget(t, rd, "abc")
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"runs"`) {
		t.Fatalf("widget render must inline the artifact's state: %s", body)
	}
	if !strings.Contains(body, "WIDGET-BODY") || strings.Contains(body, "ARTIFACT-BODY") {
		t.Fatalf("widget render must serve the widget body, not the artifact body: %s", body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("widget doc must be Cache-Control: no-store, got %q", cc)
	}
}

// The widget's network reach is exactly the artifact's — one allowlist, one
// CSP, no second policy to keep in sync.
func TestServeWidgetUsesArtifactCSP(t *testing.T) {
	rd := newWidgetRenderer(t, "abc", "<html><head></head><body>ARTIFACT-BODY</body></html>",
		"<html><head></head><body>WIDGET-BODY</body></html>")

	w := serveWidget(t, rd, "abc")
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src blob: data: https://api.example.com") {
		t.Fatalf("widget CSP must be built from the artifact's allowlist, got %q", csp)
	}
	if !strings.Contains(csp, "frame-ancestors https://app.test") {
		t.Fatalf("widget must stay embeddable only by the app origin, got %q", csp)
	}
}

// A widget renders state; it never writes it. The write-through bridge must be
// short-circuited so no tile can mutate the library it is displayed in.
func TestWidgetPreambleCannotWriteState(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test",
		map[string]string{"k": "v"}, originPolicy{}, true, false, nil)

	if !strings.Contains(doc, "var WIDGET = true;") {
		t.Fatalf("widget preamble must declare widget mode: %s", doc)
	}
	if !strings.Contains(doc, "if (WIDGET) return;") {
		t.Fatalf("widget preamble must short-circuit the state write-through: %s", doc)
	}
	// Reads still work — the cache is inlined exactly as for the artifact.
	if !strings.Contains(doc, `{"k":"v"}`) {
		t.Fatalf("widget preamble must inline state for reads: %s", doc)
	}
}

// A widget's authority is a strict subset of its artifact's: no download
// bridge, no clipboard bridge, no file-picker polyfill, no element picker.
// These are capabilities the *user* approved for a tool they opened, not for a
// tile that renders unattended behind pointer-events:none.
func TestWidgetPreambleInstallsNoCapabilityBridges(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, originPolicy{}, true, false, nil)

	for _, marker := range []string{"__avDownload", "__avNavigate", "__avClipboard", "__avMedia", "showOpenFilePicker", "__avSnippet"} {
		if strings.Contains(doc, marker) {
			t.Fatalf("widget preamble must not install %s: %s", marker, doc)
		}
	}
	// The artifact preamble still has them — this is a widget-only subtraction,
	// not a removal.
	full := injectPreamble("<head></head>", "abc", "https://app.test", nil, originPolicy{}, false, false, nil)
	for _, marker := range []string{"__avDownload", "__avNavigate", "__avClipboard", "__avMedia", "showOpenFilePicker"} {
		if !strings.Contains(full, marker) {
			t.Fatalf("artifact preamble lost %s: %s", marker, full)
		}
	}
}

// Widgets are drawn into a fixed-size well with no page of their own, so the
// preamble establishes the viewport floor (no body margin, full height,
// transparent) before the widget's markup, which can still override it.
func TestWidgetPreambleAddsBaseStylesheet(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, originPolicy{}, true, false, nil)
	if !strings.Contains(doc, "background:transparent") {
		t.Fatalf("widget preamble must set a transparent base surface: %s", doc)
	}
	if strings.Contains(injectPreamble("<head></head>", "abc", "https://app.test", nil, originPolicy{}, false, false, nil), "background:transparent") {
		t.Fatal("artifact preamble must not inject the widget base stylesheet")
	}
}

// An artifact with no widget has no widget document to serve. The gallery
// renders its default tile instead and never points a frame here, so a 404 is
// the honest answer rather than an empty 200.
func TestServeWidgetNotFoundWithoutWidget(t *testing.T) {
	rd := newWidgetRenderer(t, "abc", "<html><head></head><body>ARTIFACT-BODY</body></html>", "")
	if w := serveWidget(t, rd, "abc"); w.Code != 404 {
		t.Fatalf("expected 404 for an artifact with no widget, got %d", w.Code)
	}
}

// A widget frame is cross-origin and opaque, so the host cannot tell a working
// widget from a 404 page or one whose script threw — every case fires the same
// `load`. The preamble therefore has the widget report on itself, and the host
// falls back to the monogram tile on an error or on silence.
func TestWidgetPreambleReportsHealthToHost(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, originPolicy{}, true, false, nil)

	if !strings.Contains(doc, "__avWidget") {
		t.Fatalf("widget preamble must post a health report to the host: %s", doc)
	}
	// Both outcomes must exist: 'ready' is what suppresses the host's deadline,
	// 'error' is what trips the fallback immediately.
	for _, marker := range []string{"post('ready')", "post('error'"} {
		if !strings.Contains(doc, marker) {
			t.Fatalf("widget preamble missing %s: %s", marker, doc)
		}
	}
	// Rendering nothing is a failure by contract — a widget must always draw
	// something, an empty state included.
	if !strings.Contains(doc, "widget rendered nothing") {
		t.Fatalf("widget preamble must treat an empty render as a failure: %s", doc)
	}
	// Uncaught errors and rejected promises both count.
	if !strings.Contains(doc, "'unhandledrejection'") {
		t.Fatalf("widget preamble must report rejected promises: %s", doc)
	}
	// Pinned to the app origin like every other message the preamble sends.
	if !strings.Contains(doc, "window.parent.postMessage({ __avWidget: true, status: type, detail: detail || null }, API_ORIGIN)") {
		t.Fatalf("health report must be pinned to the app origin: %s", doc)
	}
	// Top-level renders have no host to report to.
	if !strings.Contains(doc, "if (window.parent === window) return;") {
		t.Fatalf("health report must be framed-only: %s", doc)
	}

	// The artifact preamble has no such reporter: an artifact that fails is
	// visible to the person looking at it, and has the capability-warning
	// banner besides.
	if strings.Contains(injectPreamble("<head></head>", "abc", "https://app.test", nil, originPolicy{}, false, false, nil), "__avWidget") {
		t.Fatal("artifact preamble must not carry the widget health reporter")
	}
}

// av-kmwj: a widget has no prompt to raise — the capability bridges the
// reporter lives among are omitted wholesale for a tile — so it neither listens
// for violations nor carries the list of origins it would have suppressed. A
// gallery page renders one widget document per card, and none of them should
// ship machinery none of them can use.
func TestWidgetPreambleHasNoNetworkReporter(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil,
		originPolicy{Blocked: []string{"https://tracker.example.com"}}, true, false, nil)

	if strings.Contains(doc, "securitypolicyviolation") || strings.Contains(doc, "__avNetwork") {
		t.Fatalf("a widget must not report CSP violations: %s", doc)
	}
	if strings.Contains(doc, "tracker.example.com") {
		t.Fatalf("a widget carries no suppression list, having nothing to suppress: %s", doc)
	}
	if full := injectPreamble("<head></head>", "abc", "https://app.test", nil,
		originPolicy{Blocked: []string{"https://tracker.example.com"}}, false, false, nil); !strings.Contains(full, "__avNetwork") {
		t.Fatalf("the artifact preamble must still carry the reporter: %s", full)
	}
}
