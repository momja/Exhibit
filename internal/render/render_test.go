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

// connect-src's *network* portion is derived purely from the artifact's own
// allowlist. The shim needs no network access of its own (it reads inlined state
// and writes via postMessage), so the app origin must NOT leak into connect-src —
// that would let artifact code talk to the app origin. blob:/data: are local I/O,
// not egress (see TestBuildCSPConnectSrcAlwaysAllowsBlobAndData), so they are
// present in both branches and are excluded from "network" here.
func TestBuildCSPConnectSrcIsAllowlistOnly(t *testing.T) {
	const appOrigin = "https://app.example.com"

	// network returns the connect-src sources that can actually reach the network.
	network := func(cs string) string {
		return strings.TrimSpace(strings.NewReplacer("blob:", "", "data:", "").Replace(cs))
	}

	t.Run("empty allowlist leaves connect-src with no network reach", func(t *testing.T) {
		cs := connectSrc(t, buildCSP(nil, appOrigin, ""))
		if n := network(cs); n != "" {
			t.Fatalf("connect-src %q must reach nothing without an allowlist, got network sources %q", cs, n)
		}
	})

	t.Run("populated allowlist is exactly the allowlist", func(t *testing.T) {
		cs := connectSrc(t, buildCSP([]string{"https://api.github.com"}, appOrigin, ""))
		if network(cs) != "https://api.github.com" {
			t.Fatalf("connect-src %q must reach exactly the allowlisted origin", cs)
		}
		if strings.Contains(cs, appOrigin) {
			t.Fatalf("connect-src %q must not include the app origin", cs)
		}
	})
}

// fetch()ing a blob: object URL the artifact minted itself, or a data: URI it
// built, is local I/O: the bytes are already in the agent and nothing leaves the
// browser. It is nonetheless governed by connect-src, so without these an
// artifact whose library loads its own payload that way (ffmpeg.wasm fetches its
// core .wasm from a blob: URL) fails with a bare "TypeError: Failed to fetch"
// (av-x01o).
func TestBuildCSPConnectSrcAlwaysAllowsBlobAndData(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://api.github.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			cs := connectSrc(t, buildCSP(allowlist, appOrigin, ""))
			for _, src := range []string{"blob:", "data:"} {
				if !strings.Contains(cs, src) {
					t.Fatalf("connect-src %q must allow %s — reading back local bytes is not egress", cs, src)
				}
			}
		})
	}
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
			ss, ok := directive(t, buildCSP(allowlist, appOrigin, ""), "style-src")
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

	ss, _ := directive(t, buildCSP([]string{cdn}, appOrigin, ""), "style-src")
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
			fs, ok := directive(t, buildCSP(allowlist, appOrigin, ""), "font-src")
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

	fs, _ := directive(t, buildCSP([]string{fonts}, appOrigin, ""), "font-src")
	if !strings.Contains(fs, fonts) {
		t.Fatalf("font-src %q dropped the allowlisted font origin %q", fs, fonts)
	}
}

// A locally imported file (<input type=file> -> URL.createObjectURL) played back
// via <video>/<audio src=blob:...> never leaves the browser, so it must render
// regardless of the allowlist. media-src must permit blob: in BOTH branches — in
// the empty branch it would otherwise fall back to default-src 'none' and be
// blocked (this was the reported bug: a blob: media load blocked under
// "default-src 'none'" because no media-src directive existed at all).
func TestBuildCSPMediaSrcAlwaysAllowsBlob(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://cdn.example.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			ms, ok := directive(t, buildCSP(allowlist, appOrigin, ""), "media-src")
			if !ok {
				t.Fatalf("media-src directive missing — a blob: media source falls back to default-src 'none' and is blocked")
			}
			if !strings.Contains(ms, "blob:") {
				t.Fatalf("media-src %q must allow blob: for locally imported files", ms)
			}
		})
	}
}

// A script the artifact builds at runtime (blob:/data: URL) is local execution,
// not egress — and script-src already carries 'unsafe-inline'/'unsafe-eval', so
// these grant nothing new. They must be present in BOTH branches, or the
// empty-allowlist case falls back to default-src 'none' and the populated case
// reduces to a bare origin list.
func TestBuildCSPScriptSrcAlwaysAllowsBlobAndData(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://unpkg.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			ss, ok := directive(t, buildCSP(allowlist, appOrigin, ""), "script-src")
			if !ok {
				t.Fatalf("script-src directive missing")
			}
			for _, src := range []string{"blob:", "data:"} {
				if !strings.Contains(ss, src) {
					t.Fatalf("script-src %q must allow %s for locally constructed scripts", ss, src)
				}
			}
		})
	}
}

// A Worker constructed from a blob:/data: URL (the standard way to spawn one —
// e.g. ffmpeg.wasm — from an opaque-origin sandboxed iframe, since a Worker
// cannot load a classic cross-origin script directly) runs the artifact's own
// bytes locally and reaches nothing, so it must execute regardless of the
// allowlist. worker-src is spelled out rather than left to fall back to
// script-src, because its absence fails silently: the Worker constructor
// succeeds, no error is logged and no promise rejects — the worker body simply
// never runs, so an artifact hangs on "Loading..." forever (av-x01o).
func TestBuildCSPWorkerSrcAlwaysAllowsBlobAndData(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://unpkg.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			ws, ok := directive(t, buildCSP(allowlist, appOrigin, ""), "worker-src")
			if !ok {
				t.Fatalf("worker-src directive missing — a blob:/data: worker then fails silently, never running its body")
			}
			for _, src := range []string{"blob:", "data:"} {
				if !strings.Contains(ws, src) {
					t.Fatalf("worker-src %q must allow %s for locally constructed workers", ws, src)
				}
			}
		})
	}
}

// A worker script fetched from a remote origin IS egress, so it stays gated by
// the allowlist like every other network-reaching source: an approved origin
// appears in worker-src, and the app origin never does.
func TestBuildCSPWorkerSrcHonorsAllowlistedOrigin(t *testing.T) {
	const appOrigin = "https://app.example.com"
	const cdn = "https://unpkg.com"

	ws, _ := directive(t, buildCSP([]string{cdn}, appOrigin, ""), "worker-src")
	if !strings.Contains(ws, cdn) {
		t.Fatalf("worker-src %q dropped the allowlisted worker origin %q", ws, cdn)
	}
	if strings.Contains(ws, appOrigin) {
		t.Fatalf("worker-src %q must not include the app origin", ws)
	}
}

// form-action does NOT fall back to default-src, unlike the other directives
// above. So it needs its own explicit value in both branches, built from the
// same allowlist as connect-src (av-jlp8): the sandbox grants allow-forms, and
// without a form-action directive an artifact could submit a <form> to any
// origin, bypassing the network allowlist entirely.
func TestBuildCSPFormActionMirrorsAllowlist(t *testing.T) {
	const appOrigin = "https://app.example.com"

	t.Run("empty allowlist pins form-action to self", func(t *testing.T) {
		fa, ok := directive(t, buildCSP(nil, appOrigin, ""), "form-action")
		if !ok {
			t.Fatalf("form-action directive missing — a form with no explicit action would then be unrestricted, not blocked")
		}
		if fa != "'self'" {
			t.Fatalf("expected form-action 'self', got %q", fa)
		}
	})

	t.Run("populated allowlist includes the allowlisted origin and self", func(t *testing.T) {
		fa, ok := directive(t, buildCSP([]string{"https://api.github.com"}, appOrigin, ""), "form-action")
		if !ok {
			t.Fatalf("form-action directive missing")
		}
		if !strings.Contains(fa, "https://api.github.com") {
			t.Fatalf("form-action %q dropped the allowlisted origin", fa)
		}
		if !strings.Contains(fa, "'self'") {
			t.Fatalf("form-action %q dropped 'self', breaking same-page/empty-action forms", fa)
		}
		if strings.Contains(fa, appOrigin) {
			t.Fatalf("form-action %q must not include the app origin", fa)
		}
	})
}

// Writes must go to the host frame via postMessage (pinned to the app origin),
// not a cross-origin fetch — the sandboxed iframe's opaque origin can't call the
// API, and the fetch approach was what CORS-blocked write-through. The shim may
// mention fetch (it shims data: URL fetches, av-02xs) but must never invoke it
// with a URL — its only network-adjacent call is nativeFetch.apply passthrough.
func TestShimWritesViaPostMessageNotFetch(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)
	if !strings.Contains(doc, "window.parent.postMessage") {
		t.Fatalf("shim should write via postMessage to the host frame: %s", doc)
	}
	// A call with a URL literal would be the CORS-blocked API fetch; the data:
	// shim only ever routes through nativeFetch.apply or constructs Responses.
	if strings.Contains(doc, "fetch('") || strings.Contains(doc, `fetch("`) {
		t.Fatalf("shim must not fetch the API directly (CORS-blocked from the sandbox): %s", doc)
	}
}

// The framed preamble shims data: URL fetches into local Responses (WebKit
// refuses large data: fetches from an opaque-origin sandbox). Widget renders
// omit the whole bridgeScript, so it doesn't ship there. The canvas-leak
// mitigation trialed in av-02xs was removed as ineffective — assert it stays
// gone so it can't silently degrade artifact rendering again.
func TestShimFramedDataURLFetchWrapper(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)
	if !strings.Contains(doc, "window.fetch = function(input, init)") {
		t.Fatalf("framed shim must wrap fetch for data: URLs: %s", doc)
	}
	if !strings.Contains(doc, "new Response(bytes, {") {
		t.Fatalf("framed shim must construct Responses from data: bytes: %s", doc)
	}
	// The shim must decode payloads byte-by-byte (decodeURIComponent throws on
	// non-UTF-8 sequences and corrupts bytes 0x80+) and must exclude any URL
	// fragment from the body it constructs.
	if !strings.Contains(doc, "function percentDecodeBytes") {
		t.Fatalf("framed shim must byte-level percent-decode non-base64 payloads: %s", doc)
	}
	if !strings.Contains(doc, "data.indexOf('#')") {
		t.Fatalf("framed shim must strip the fragment before decoding the payload: %s", doc)
	}
	if strings.Contains(doc, "willReadFrequently") || strings.Contains(doc, "CANVAS_MITIGATION") {
		t.Fatalf("ineffective canvas mitigation must not ship in the shim: %s", doc)
	}

	widgetDoc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, true, false, nil)
	if strings.Contains(widgetDoc, "window.fetch = function(input, init)") {
		t.Fatalf("widget renders must not carry the fetch shim: %s", widgetDoc)
	}
}

// The shim must inline state so the artifact's synchronous startup reads see it,
// rather than fetching asynchronously (which the artifact's own init would race).
func TestInjectShimInlinesStateWithoutAsyncHydrate(t *testing.T) {
	state := map[string]string{"tkgraph:config:v1": `{"lastSource":"github"}`}
	doc := injectPreamble("<html><head></head><body></body></html>", "abc", "https://app.test", state, nil, false, false, nil)

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
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)
	if !strings.Contains(doc, "var cache = {}") {
		t.Fatalf("nil state should inline an empty object, got: %s", doc)
	}
}

// storageInstall returns the value expression the shim binds to the named
// window property, e.g. "makeStorage(cache, persistState)" for localStorage.
// Comparing the two expressions is how the tests below establish that the two
// Web Storage namespaces are backed by different objects.
func storageInstall(t *testing.T, doc, property string) string {
	t.Helper()
	open := "Object.defineProperty(window, '" + property + "', { value: "
	start := strings.Index(doc, open)
	if start < 0 {
		t.Fatalf("shim does not install window.%s: %s", property, doc)
	}
	rest := doc[start+len(open):]
	end := strings.Index(rest, ", writable:")
	if end < 0 {
		t.Fatalf("could not read the window.%s install expression: %s", property, rest)
	}
	return rest[:end]
}

// localStorage and sessionStorage are two namespaces with two lifetimes, and
// artifacts are written against that: 'draft' in sessionStorage is this
// session's scratch copy, 'draft' in localStorage is the saved one. The shim
// used to install ONE object over ONE cache under both names (av-9jll), so the
// second write clobbered the first and a read from either name returned
// whichever won. They must be built over separate caches, and only the
// localStorage one may reach the server.
func TestShimStorageNamespacesAreIndependent(t *testing.T) {
	// A key inlined into the persisted namespace — the collision case: an
	// artifact writing sessionStorage['draft'] must not see or overwrite it.
	doc := injectPreamble("<head></head>", "abc", "https://app.test",
		map[string]string{"draft": "saved"}, nil, false, false, nil)

	local := storageInstall(t, doc, "localStorage")
	session := storageInstall(t, doc, "sessionStorage")
	if local == session {
		t.Fatalf("both namespaces install the same object (%s) — one cache, colliding keys", local)
	}

	// Each namespace owns its cache: the factory copies its argument into a
	// per-instance binding rather than closing over one shared map.
	if !strings.Contains(doc, "var store = initial;") {
		t.Fatalf("storage objects must be built over their own cache, not a shared one: %s", doc)
	}

	// The persisted namespace gets the inlined state and the write bridge.
	if !strings.Contains(local, "cache") || !strings.Contains(local, "persistState") {
		t.Fatalf("localStorage must be backed by the inlined cache and write through: %s", local)
	}

	// The ephemeral one gets neither, so the inlined 'draft' is unreachable
	// from it and its writes produce no artifact_state rows and no postMessage.
	if strings.Contains(session, "cache") {
		t.Fatalf("sessionStorage must not share the inlined localStorage cache: %s", session)
	}
	if strings.Contains(session, "persistState") {
		t.Fatalf("sessionStorage must not write through to the server: %s", session)
	}
}

// The sessionStorage shim installs ONLY when framed. In the sandbox it is
// forced (the opaque origin has no storage key, so the native getter throws on
// property access), and purely in-memory is exactly native behavior there —
// each navigation gets a fresh opaque origin. Top-level at RENDER_ORIGIN/a/:id
// the document has a real origin where native sessionStorage genuinely works,
// is tab-scoped, and survives a reload, so replacing it is a strict downgrade.
// localStorage keeps its unconditional install: it serves the inlined reads
// top-level too (its own top-level write problem is av-blzu).
func TestShimSessionStorageInstallIsFramedOnly(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	guard := strings.Index(doc, "if (window.parent !== window) {")
	if guard < 0 {
		t.Fatalf("framed guard missing from the shim: %s", doc)
	}
	if strings.Index(doc, "Object.defineProperty(window, 'sessionStorage'") < guard {
		t.Fatalf("sessionStorage is installed outside the framed guard — a top-level render would lose native sessionStorage: %s", doc)
	}
	if local := strings.Index(doc, "Object.defineProperty(window, 'localStorage'"); local < 0 || local > guard {
		t.Fatalf("localStorage must install unconditionally so inlined reads work top-level too: %s", doc)
	}
}

// removeItem and clear (av-ms3r, av-st7c) must not fall back to the old
// key/value sentinel (deleting by writing ”), because ” is a legitimate
// stored value that must remain distinguishable from an actual delete. Each
// operation is tagged with an explicit op so the host bridge — and the two
// listeners that consume it — can tell them apart with no ambiguity.
func TestShimRemoveItemAndClearUseExplicitOp(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	if !strings.Contains(doc, "persist('delete', key)") {
		t.Fatalf("removeItem must post an explicit 'delete' op, not a '' sentinel: %s", doc)
	}
	if !strings.Contains(doc, "persist('clear')") {
		t.Fatalf("clear must post an explicit 'clear' op with no key: %s", doc)
	}
	if !strings.Contains(doc, "persist('set', key, String(value))") {
		t.Fatalf("setItem must still post its value under the 'set' op: %s", doc)
	}
	// The old bug: removeItem persisting '' as if it were a stored value.
	if strings.Contains(doc, "persist(key, '')") {
		t.Fatalf("removeItem must not persist an empty-string tombstone: %s", doc)
	}
}

// clear() must write through — the original bug (av-st7c) dropped the cache
// with no persist call at all, so the wipe looked successful until the next
// render re-inlined every original key.
func TestShimClearWritesThrough(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	clearIdx := strings.Index(doc, "clear: function() {")
	if clearIdx < 0 {
		t.Fatalf("shim missing clear(): %s", doc)
	}
	nextMethod := strings.Index(doc[clearIdx+1:], "key: function(n) {")
	if nextMethod < 0 {
		t.Fatalf("could not bound the clear() body: %s", doc)
	}
	body := doc[clearIdx : clearIdx+1+nextMethod]
	if !strings.Contains(body, "if (persist) persist('clear')") {
		t.Fatalf("clear() does not write through to the host: %s", body)
	}
}

// The top-level guard is centralized in persistState rather than duplicated
// per operation, so clear() and removeItem() called with no host frame stay
// cache-only and never throw — matching every other bridge's guard.
func TestShimPersistStateGuardsTopLevelForEveryOp(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	fn := "function persistState(op, key, value) {"
	start := strings.Index(doc, fn)
	if start < 0 {
		t.Fatalf("shim missing persistState: %s", doc)
	}
	body := doc[start : start+300]
	if !strings.Contains(body, "if (window.parent === window) return;") {
		t.Fatalf("persistState must guard every op (set/delete/clear) behind one top-level check: %s", body)
	}
}

// The download bridge (av-ryby): the sandbox omits allow-downloads, so the
// shim intercepts the common export vectors and posts filename + bytes to the
// host frame, which owns approval and performs the download. The shim itself
// must never gain a network path for this (blob payloads come from a
// createObjectURL registry, not a connect-src-governed fetch).
func TestShimInstallsDownloadBridge(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	// The message shape the host's download listener validates.
	if !strings.Contains(doc, "__avDownload") {
		t.Fatalf("shim missing the download bridge message: %s", doc)
	}
	// Capture-phase click interception: the shim must see the activation
	// before the artifact's own handlers and before the (blocked) default.
	if !strings.Contains(doc, "document.addEventListener('click'") || !strings.Contains(doc, "}, true);") {
		t.Fatalf("shim missing capture-phase click interception: %s", doc)
	}
	// blob: payloads are recovered from the createObjectURL registry rather
	// than re-fetched, so the bridge stays independent of connect-src.
	if !strings.Contains(doc, "URL.createObjectURL") {
		t.Fatalf("shim missing the createObjectURL registry: %s", doc)
	}
	// Detached anchors (createElement -> click() without appendChild) never
	// reach the document listener; their programmatic clicks must be routed.
	if !strings.Contains(doc, "HTMLAnchorElement.prototype.click") {
		t.Fatalf("shim missing the programmatic-click route: %s", doc)
	}
}

// The bridge must only install when a host frame exists. Opened top-level
// (direct render-origin visit or a share) there is no sandbox and native
// downloads already work — intercepting there would break them, and share
// pages get no bridge in v1. Widget renders never carry this block at all
// (av-fafu) — see widget_test.go.
func TestShimDownloadBridgeIsFramedOnly(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)
	if !strings.Contains(doc, "if (window.parent !== window) {") {
		t.Fatalf("download bridge must be guarded to framed (gallery-embedded) contexts: %s", doc)
	}
}

// The link navigation bridge (av-r0dk): the sandbox omits allow-popups, so a
// target=_blank anchor is dropped and a plain anchor would navigate the iframe
// itself. The shim intercepts external http(s) anchor activations and posts the
// URL to the host, which owns approval and opens it in a new tab. Downloads
// (blob:/data:) still win over navigation, and same-origin/hash/mailto/
// javascript: links are left to their native behavior.
func TestShimInstallsLinkNavigationBridge(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	if !strings.Contains(doc, "__avNavigate") {
		t.Fatalf("shim missing the link navigation bridge message: %s", doc)
	}
	// The interception must distinguish navigation hrefs from download hrefs;
	// downloads keep their own message and still win on click.
	if !strings.Contains(doc, "isExternalLinkHref") {
		t.Fatalf("shim missing the external-link predicate: %s", doc)
	}
	// The resolved URL is compared against location.origin so hash-only,
	// relative, javascript:, and mailto: links are untouched.
	if !strings.Contains(doc, "location.origin") {
		t.Fatalf("shim must resolve against the document origin: %s", doc)
	}
	// The download bridge survives the addition.
	if !strings.Contains(doc, "__avDownload") {
		t.Fatalf("shim lost the download bridge: %s", doc)
	}
}

// The module-worker interceptor (av-yvtb): Chrome refuses module-worker script
// fetches for an opaque origin, so Worker({type:'module'}) silently hangs in the
// sandbox. The preamble wraps the Worker constructor and, on the module-worker +
// opaque-origin case, posts a generic __avCapabilityWarning diagnostic to the
// host frame (naming the 'module-worker' capability), pinned to the app origin
// like every other bridge — so the host can warn.
func TestShimInstallsModuleWorkerInterceptor(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	// The generic message shape the host's banner listener validates.
	if !strings.Contains(doc, "__avCapabilityWarning") {
		t.Fatalf("shim missing the capability-warning diagnostic message: %s", doc)
	}
	// The module-worker detection names its capability slug so the host can map
	// it to support copy.
	if !strings.Contains(doc, "'module-worker'") {
		t.Fatalf("shim must name the module-worker capability: %s", doc)
	}
	// It must gate on the module worker type and the opaque origin, not fire for
	// classic workers or top-level (real-origin) renders.
	if !strings.Contains(doc, "options.type === 'module'") {
		t.Fatalf("interceptor must gate on the module worker type: %s", doc)
	}
	// Gate on self.origin (the effective/opaque origin), not location.origin —
	// the latter reports the URL's tuple origin for an http-loaded opaque
	// document and would never match 'null'.
	if !strings.Contains(doc, "self.origin === 'null'") {
		t.Fatalf("interceptor must gate on the opaque ('null') effective origin: %s", doc)
	}
	// The diagnostic is pinned to the app origin like every other shim message.
	if !strings.Contains(doc, "API_ORIGIN") {
		t.Fatalf("module-worker messages must be pinned to the app origin: %s", doc)
	}
	// Runtime behavior is unchanged: the real Worker is still constructed.
	if !strings.Contains(doc, "NativeWorker") {
		t.Fatalf("interceptor must still construct the real Worker: %s", doc)
	}
	// The diagnostic fires at load and can race the host's listener, so it is
	// buffered and replayed on the host's readiness ping — delivery must not
	// depend on the host already listening when the worker is constructed.
	if !strings.Contains(doc, "__avHostReady") {
		t.Fatalf("interceptor must replay the diagnostic on the host-ready handshake: %s", doc)
	}
}

// Like the other bridges, the module-worker interceptor installs framed-only —
// guarded by the same window.parent check — so top-level and share renders
// (which have a real origin and run module workers fine) neither install it nor
// warn.
func TestShimModuleWorkerInterceptorIsFramedOnly(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)
	// The interceptor block sits inside the framed guard; the diagnostic marker
	// must appear after the guard opens.
	guard := strings.Index(doc, "if (window.parent !== window) {")
	marker := strings.Index(doc, "__avCapabilityWarning")
	if guard < 0 || marker < 0 || marker < guard {
		t.Fatalf("module-worker interceptor must be guarded to framed contexts: %s", doc)
	}
}

// The clipboard bridge (av-hll6) proxies navigator.clipboard read/write through
// the host frame: it replaces the API, posts the host-validated message shape,
// and pins the request to the app origin like every other shim message. Like
// the download bridge it installs framed-only (guarded by the same
// window.parent check), so top-level/share renders are unaffected.
func TestShimInstallsClipboardBridge(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	// The message shape the host's clipboard listener validates.
	if !strings.Contains(doc, "__avClipboard") {
		t.Fatalf("shim missing the clipboard bridge message: %s", doc)
	}
	// The Clipboard API surface is actually replaced, not just referenced.
	if !strings.Contains(doc, "writeText:") || !strings.Contains(doc, "readText:") {
		t.Fatalf("shim must replace navigator.clipboard read/write: %s", doc)
	}
	if !strings.Contains(doc, "navigator") || !strings.Contains(doc, "'clipboard'") {
		t.Fatalf("shim must install onto navigator.clipboard: %s", doc)
	}
	// Requests are pinned to the app origin, never broadcast.
	if !strings.Contains(doc, "API_ORIGIN") {
		t.Fatalf("clipboard messages must be pinned to the app origin: %s", doc)
	}
}

// The File System Access picker polyfill (av-70t9): the sandboxed iframe's
// opaque origin makes showOpenFilePicker / showDirectoryPicker / showSaveFilePicker
// unreachable (Blink throws a SecurityError that no sandbox token re-enables),
// so the shim polyfills them on the classic <input type=file> picker, which
// Blink subjects to no sandbox check. Open/directory return FSA-shaped handles;
// save's createWritable routes through the download bridge. Like the other
// bridges, framed-only (co-located inside the window.parent guard).
func TestShimInstallsFSAPickerPolyfill(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	// All three FSA entry points are replaced on window.
	for _, name := range []string{"showOpenFilePicker", "showDirectoryPicker", "showSaveFilePicker"} {
		if !strings.Contains(doc, "window."+name) {
			t.Fatalf("shim must polyfill %s: %s", name, doc)
		}
	}
	// The polyfill backs onto <input type=file>, the one picker Blink allows
	// inside a sandboxed frame.
	if !strings.Contains(doc, "input.type = 'file'") {
		t.Fatalf("FSA polyfill must use an <input type=file> fallback: %s", doc)
	}
	// Directories use the webkitdirectory attribute so one pick yields a folder.
	if !strings.Contains(doc, "webkitdirectory") {
		t.Fatalf("showDirectoryPicker must fall back to webkitdirectory: %s", doc)
	}
	// A canceled picker rejects with AbortError, matching native FSA semantics.
	if !strings.Contains(doc, "AbortError") {
		t.Fatalf("canceled picker must reject with AbortError: %s", doc)
	}
	// File handles carry the FSA surface artifacts actually call.
	if !strings.Contains(doc, "getFile") || !strings.Contains(doc, "createWritable") {
		t.Fatalf("file handles must expose getFile/createWritable: %s", doc)
	}
	// Directory handles are async-iterable (values/entries/keys + the default
	// async iterator), the shape `for await (const h of dir.values())` needs.
	if !strings.Contains(doc, "Symbol.asyncIterator") {
		t.Fatalf("directory handles must be async-iterable: %s", doc)
	}
	// The directory tree is reconstructed from webkitRelativePath so nested
	// walks (for await ... of subdirHandle.values()) work, not just a flat list.
	if !strings.Contains(doc, "webkitRelativePath") {
		t.Fatalf("showDirectoryPicker must rebuild the tree from webkitRelativePath: %s", doc)
	}
}

// The save picker has no native save dialog in a sandboxed iframe, so
// createWritable materializes a download by reusing the av-ryby download
// bridge (a detached blob:-href anchor click) — NOT by adding allow-downloads
// to the sandbox or introducing a new host message type. This keeps the export
// surface single-path and the sandbox token set unchanged (downloads_test.go
// still asserts sandbox="allow-scripts" with no allow-downloads).
func TestShimFSASaveReusesDownloadBridge(t *testing.T) {
	doc := injectPreamble("<head></head>", "abc", "https://app.test", nil, nil, false, false, nil)

	// The save writable triggers a download via a detached anchor click, the
	// same vector the download bridge intercepts.
	if !strings.Contains(doc, "a.download = filename") || !strings.Contains(doc, "a.click()") {
		t.Fatalf("createWritable.close() must trigger the download bridge via an anchor click: %s", doc)
	}
	// No separate host message for saves — the bytes ride __avDownload like
	// every other export, so the host's single download listener handles them.
	if strings.Contains(doc, "__avSave") {
		t.Fatalf("save must reuse the __avDownload bridge, not a new message type: %s", doc)
	}
	// The writable stream honors the WriteParams form ({type:'write', data})
	// that spec-conformant artifacts pass, not just bare Blob/ArrayBuffer.
	if !strings.Contains(doc, "data.type === 'write'") {
		t.Fatalf("createWritable.write must accept the WriteParams form: %s", doc)
	}
}

// The asset source is a system source, not an approved one: it is added by the
// render surface and never appears in the allowlist editor. What it must be is
// an absolute, path-scoped URL. A path-only source would be resolved against
// whatever the browser is comparing it to, and the artifact's own frame has an
// opaque origin — so a vendored payload's fetch would be blocked by the very
// policy that is supposed to permit it. The trailing slash keeps it a prefix
// over one artifact's assets rather than a grant over the whole origin.
func TestBuildCSPCarriesTheAbsoluteAssetSource(t *testing.T) {
	const appOrigin = "https://app.example.com"
	rd := &Renderer{cfg: Config{RenderOrigin: "https://render.example.com"}}

	base := rd.assetBaseURL("art-1")
	if base != "https://render.example.com/a/art-1/assets/" {
		t.Fatalf("asset base %q must be absolute and path-scoped to the artifact", base)
	}

	csp := buildCSP(nil, appOrigin, base)
	// Every directive a vendored payload can load under: the runtime pass's
	// arrive by fetch, markup assets (av-oz40) through the element that names
	// them.
	for _, name := range []string{"connect-src", "img-src", "font-src", "media-src", "style-src", "script-src"} {
		d, ok := directive(t, csp, name)
		if !ok {
			t.Fatalf("%s directive missing", name)
		}
		if !strings.Contains(d, base) {
			t.Fatalf("%s %q must carry the asset source %q", name, d, base)
		}
	}

	// And it costs an artifact without assets nothing.
	if strings.Contains(buildCSP(nil, appOrigin, ""), "/assets/") {
		t.Fatalf("an artifact with no assets must keep the policy it had before assets existed")
	}
}
