package render

import (
	"strings"
	"testing"
)

// connectSrc extracts the connect-src directive value from a CSP string.
func connectSrc(t *testing.T, csp string) string {
	t.Helper()
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if v, ok := strings.CutPrefix(d, "connect-src "); ok {
			return v
		}
	}
	t.Fatalf("no connect-src directive in CSP: %q", csp)
	return ""
}

// connect-src is derived purely from the artifact's own allowlist. The shim
// needs no network access of its own (it reads inlined state and writes via
// postMessage), so the app origin must NOT leak into connect-src — that would
// let artifact code talk to the app origin.
func TestBuildCSPConnectSrcIsAllowlistOnly(t *testing.T) {
	const appOrigin = "https://app.example.com"

	t.Run("empty allowlist locks connect-src to none", func(t *testing.T) {
		cs := connectSrc(t, buildCSP(nil, appOrigin))
		if cs != "'none'" {
			t.Fatalf("expected connect-src 'none', got %q", cs)
		}
	})

	t.Run("populated allowlist is exactly the allowlist", func(t *testing.T) {
		cs := connectSrc(t, buildCSP([]string{"https://api.github.com"}, appOrigin))
		if !strings.Contains(cs, "https://api.github.com") {
			t.Fatalf("connect-src %q dropped the allowlisted origin", cs)
		}
		if strings.Contains(cs, appOrigin) {
			t.Fatalf("connect-src %q must not include the app origin", cs)
		}
	})
}

// Writes must go to the host frame via postMessage (pinned to the app origin),
// not a cross-origin fetch — the sandboxed iframe's opaque origin can't call the
// API, and the fetch approach was what CORS-blocked write-through.
func TestShimWritesViaPostMessageNotFetch(t *testing.T) {
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)
	if !strings.Contains(doc, "window.parent.postMessage") {
		t.Fatalf("shim should write via postMessage to the host frame: %s", doc)
	}
	if strings.Contains(doc, "fetch(") {
		t.Fatalf("shim must not fetch the API directly (CORS-blocked from the sandbox): %s", doc)
	}
}

// The shim must inline state so the artifact's synchronous startup reads see it,
// rather than fetching asynchronously (which the artifact's own init would race).
func TestInjectShimInlinesStateWithoutAsyncHydrate(t *testing.T) {
	state := map[string]string{"tkgraph:config:v1": `{"lastSource":"github"}`}
	doc := injectShim("<html><head></head><body></body></html>", "abc", "https://app.test", state)

	// The state value is embedded directly in the shim's cache.
	if !strings.Contains(doc, "lastSource") || !strings.Contains(doc, "github") {
		t.Fatalf("state not inlined into shim: %s", doc)
	}
	// No async hydrate at all — a .then() chain reading state back would be the
	// GET hydrate that races the artifact's synchronous startup reads.
	if strings.Contains(doc, ".then(function") {
		t.Fatalf("shim still hydrates asynchronously — reintroduces the race: %s", doc)
	}
	// The closing tag must not be breakable out of the <script>.
	if strings.Contains(doc, "</script>{") {
		t.Fatalf("state JSON not HTML-escaped for <script> context")
	}
}

// A nil/empty state must produce a valid empty-object cache, never `null`.
func TestInjectShimNilStateIsEmptyObject(t *testing.T) {
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)
	if !strings.Contains(doc, "var cache = {}") {
		t.Fatalf("nil state should inline an empty object, got: %s", doc)
	}
}
