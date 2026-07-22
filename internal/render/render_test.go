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
			ms, ok := directive(t, buildCSP(allowlist, appOrigin), "media-src")
			if !ok {
				t.Fatalf("media-src directive missing — a blob: media source falls back to default-src 'none' and is blocked")
			}
			if !strings.Contains(ms, "blob:") {
				t.Fatalf("media-src %q must allow blob: for locally imported files", ms)
			}
		})
	}
}

// A Worker constructed from a blob: URL (the standard workaround for spawning a
// cross-origin worker script — e.g. ffmpeg.wasm — from an opaque-origin sandboxed
// iframe, since a Worker cannot load a classic cross-origin script directly) must
// execute regardless of the allowlist. There is no worker-src directive, so the
// browser falls back to script-src for worker scripts; script-src must permit
// blob: in BOTH branches, or the fallback leaves it out of the empty-allowlist
// case entirely (default-src 'none') and drops it from the origin list otherwise.
func TestBuildCSPScriptSrcAlwaysAllowsBlob(t *testing.T) {
	const appOrigin = "https://app.example.com"

	cases := map[string][]string{
		"empty allowlist":     nil,
		"populated allowlist": {"https://unpkg.com"},
	}
	for name, allowlist := range cases {
		t.Run(name, func(t *testing.T) {
			ss, ok := directive(t, buildCSP(allowlist, appOrigin), "script-src")
			if !ok {
				t.Fatalf("script-src directive missing")
			}
			if !strings.Contains(ss, "blob:") {
				t.Fatalf("script-src %q must allow blob: for worker scripts loaded from a blob: URL", ss)
			}
		})
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

// The download bridge (av-ryby): the sandbox omits allow-downloads, so the
// shim intercepts the common export vectors and posts filename + bytes to the
// host frame, which owns approval and performs the download. The shim itself
// must never gain a network path for this (blob payloads come from a
// createObjectURL registry, not a connect-src-governed fetch).
func TestShimInstallsDownloadBridge(t *testing.T) {
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)

	// The message shape the host's download listener validates.
	if !strings.Contains(doc, "__avDownload") {
		t.Fatalf("shim missing the download bridge message: %s", doc)
	}
	// Capture-phase click interception: the shim must see the activation
	// before the artifact's own handlers and before the (blocked) default.
	if !strings.Contains(doc, "document.addEventListener('click'") || !strings.Contains(doc, "}, true);") {
		t.Fatalf("shim missing capture-phase click interception: %s", doc)
	}
	// blob: payloads are recovered from the createObjectURL registry — a
	// fetch(blobURL) would be governed by connect-src and blocked at 'none'.
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
// pages get no bridge in v1.
func TestShimDownloadBridgeIsFramedOnly(t *testing.T) {
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)
	if !strings.Contains(doc, "if (window.parent !== window) {") {
		t.Fatalf("download bridge must be guarded to framed (gallery-embedded) contexts: %s", doc)
	}
}

// The clipboard bridge (av-hll6) proxies navigator.clipboard read/write through
// the host frame: it replaces the API, posts the host-validated message shape,
// and pins the request to the app origin like every other shim message. Like
// the download bridge it installs framed-only (guarded by the same
// window.parent check), so top-level/share renders are unaffected.
func TestShimInstallsClipboardBridge(t *testing.T) {
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)

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
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)

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
	doc := injectShim("<head></head>", "abc", "https://app.test", nil)

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
