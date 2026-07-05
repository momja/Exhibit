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

// The storage shim fetches appOrigin/api/artifacts/:id/state. If the app origin
// is missing from connect-src, the browser blocks the shim's hydrate and
// write-through and state silently never persists. Guard both allowlist paths.
func TestBuildCSPConnectSrcAlwaysIncludesAppOrigin(t *testing.T) {
	const appOrigin = "https://app.example.com"

	t.Run("empty allowlist", func(t *testing.T) {
		cs := connectSrc(t, buildCSP(nil, appOrigin))
		if !strings.Contains(cs, appOrigin) {
			t.Fatalf("connect-src %q missing app origin %q", cs, appOrigin)
		}
		if strings.Contains(cs, "'none'") {
			t.Fatalf("connect-src is 'none' — shim state proxy would be blocked: %q", cs)
		}
	})

	t.Run("populated allowlist", func(t *testing.T) {
		cs := connectSrc(t, buildCSP([]string{"https://api.github.com"}, appOrigin))
		if !strings.Contains(cs, appOrigin) {
			t.Fatalf("connect-src %q missing app origin %q", cs, appOrigin)
		}
		if !strings.Contains(cs, "https://api.github.com") {
			t.Fatalf("connect-src %q dropped the allowlisted origin", cs)
		}
	})
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
	// No async hydrate: write-through uses fetch().catch(), but there must be no
	// .then() chain reading state back — that's the GET hydrate that races.
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
