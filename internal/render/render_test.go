package render

import (
	"strings"
	"testing"
)

// directive returns the value of the named CSP directive (e.g. "style-src"),
// and whether it was present at all. Absence is meaningful in CSP: a missing
// directive falls back to default-src, so tests distinguish "absent" from "empty".
func directive(t *testing.T, csp, name string) (string, bool) {
	t.Helper()
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if v, ok := strings.CutPrefix(d, name+" "); ok {
			return v, true
		}
	}
	return "", false
}

// connectSrc extracts the connect-src directive value from a CSP string.
func connectSrc(t *testing.T, csp string) string {
	t.Helper()
	v, ok := directive(t, csp, "connect-src")
	if !ok {
		t.Fatalf("no connect-src directive in CSP: %q", csp)
	}
	return v
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

// Inline CSS is the default way a single-file artifact carries its styling, so
// it must always render without any network approval: style-src must permit
// 'unsafe-inline' (which covers both <style> blocks and style="" attributes) in
// both the empty and populated allowlist branches.
func TestBuildCSPStyleSrcAlwaysAllowsInline(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://cdn.example.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			ss, ok := directive(t, buildCSP(allowlist, appOrigin), "style-src")
			if !ok {
				t.Fatalf("style-src directive missing")
			}
			if !strings.Contains(ss, "'unsafe-inline'") {
				t.Fatalf("style-src %q must allow 'unsafe-inline' for inline CSS", ss)
			}
		})
	}
}

// A <link rel=stylesheet href="https://approved/..."> to an allowlisted origin
// must be honored: once an origin is on the network allowlist, style-src includes
// it so the stylesheet is not blocked. This is the "accessible via the network
// policy" case from the ticket.
func TestBuildCSPStyleSrcHonorsAllowlistedOrigin(t *testing.T) {
	const appOrigin = "https://app.example.com"
	const cdn = "https://cdn.example.com"

	ss, _ := directive(t, buildCSP([]string{cdn}, appOrigin), "style-src")
	if !strings.Contains(ss, cdn) {
		t.Fatalf("style-src %q dropped the allowlisted stylesheet origin %q", ss, cdn)
	}
}

// Self-contained artifacts commonly inline fonts as data: URIs, e.g.
// @font-face { src: url(data:font/woff2;base64,...) }. That is zero network
// egress, so it must render regardless of the allowlist. font-src must permit
// data: in BOTH branches — in the empty branch it would otherwise fall back to
// default-src 'none' and be blocked; in the populated branch a bare origin list
// omits data:. (img-src already carries data:; this closes the same gap for fonts.)
func TestBuildCSPFontSrcAlwaysAllowsDataURI(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://fonts.example.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			fs, ok := directive(t, buildCSP(allowlist, appOrigin), "font-src")
			if !ok {
				t.Fatalf("font-src directive missing — a data: font falls back to default-src 'none' and is blocked")
			}
			if !strings.Contains(fs, "data:") {
				t.Fatalf("font-src %q must allow data: for inlined @font-face URIs", fs)
			}
		})
	}
}

// A web font from an allowlisted origin (@font-face { src: url(https://approved/..) })
// must be honored: font-src includes the allowlisted origins alongside data:.
func TestBuildCSPFontSrcHonorsAllowlistedOrigin(t *testing.T) {
	const appOrigin = "https://app.example.com"
	const fonts = "https://fonts.example.com"

	fs, _ := directive(t, buildCSP([]string{fonts}, appOrigin), "font-src")
	if !strings.Contains(fs, fonts) {
		t.Fatalf("font-src %q dropped the allowlisted font origin %q", fs, fonts)
	}
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
