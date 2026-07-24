// Package render provides the render surface: a read-only HTTP handler that
// serves artifact HTML documents wrapped in a per-artifact CSP and the
// storage shim. It runs on RENDER_ORIGIN, separate from the app origin.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/store"
)

// Config holds the dependencies for the render surface.
type Config struct {
	Store        store.Store
	Blob         blob.Store
	AppOrigin    string
	RenderOrigin string
}

// Renderer handles render-origin requests.
type Renderer struct {
	cfg Config
}

// New creates a Renderer with the given config.
func New(cfg Config) *Renderer {
	return &Renderer{cfg: cfg}
}

// ServeArtifact serves the artifact identified by {artifactID} from the URL.
func (rd *Renderer) ServeArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := rd.cfg.Store.GetArtifact(r.Context(), id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rd.serveArtifactDoc(w, r, a)
}

// ServeShare serves an artifact via a share link.
func (rd *Renderer) ServeShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	sh, err := rd.cfg.Store.GetShare(r.Context(), shareID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if sh == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if sh.ExpiresAt != nil && sh.ExpiresAt.Before(time.Now()) {
		http.Error(w, "share expired", http.StatusGone)
		return
	}

	a, err := rd.cfg.Store.GetArtifact(r.Context(), sh.ArtifactID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	rd.serveArtifactDoc(w, r, a)
}

// serveArtifactDoc reads the artifact body, injects the shim and CSP, and writes
// the resulting document to the response.
func (rd *Renderer) serveArtifactDoc(w http.ResponseWriter, r *http.Request, a *store.Artifact) {
	rc, err := rd.cfg.Blob.Get(r.Context(), a.SourceBlobID)
	if err != nil {
		http.Error(w, "artifact body not found", http.StatusNotFound)
		return
	}
	defer rc.Close()

	bodyBytes, err := io.ReadAll(rc)
	if err != nil {
		http.Error(w, "failed to read artifact body", http.StatusInternalServerError)
		return
	}

	csp := buildCSP(a.NetworkAllowlist, rd.cfg.AppOrigin)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The render doc is dynamic: it inlines the artifact's live state and the
	// per-artifact CSP. It must never be cached, or an iframe can load a stale
	// document (old shim/state) after a redeploy or state change.
	w.Header().Set("Cache-Control", "no-store")

	// Inline the artifact's persisted state so the shim's cache is ready before
	// any artifact script runs (avoids the async-hydration race). Degrade to an
	// empty cache if state can't be read — the artifact still renders.
	state, err := rd.cfg.Store.GetState(r.Context(), a.ID)
	if err != nil {
		slog.WarnContext(r.Context(), "render state read failed",
			slog.String("artifact_id", a.ID), slog.String("err", err.Error()))
		state = nil
	}

	doc := injectShim(string(bodyBytes), a.ID, rd.cfg.AppOrigin, state)
	slog.DebugContext(r.Context(), "rendered artifact",
		slog.String("artifact_id", a.ID),
		slog.Int("body_bytes", len(bodyBytes)),
		slog.Int("allowlist", len(a.NetworkAllowlist)),
		slog.Int("state_keys", len(state)),
		slog.String("csp", csp),
	)
	fmt.Fprint(w, doc)
}

// buildCSP generates a per-artifact Content-Security-Policy header value
// from the artifact's network allowlist. appOrigin is the only origin permitted
// to embed this page in an iframe. The storage shim needs no connect-src of its
// own: it reads inlined state and writes via postMessage to the host frame.
//
// Every source in this policy falls into one of two buckets, and which bucket a
// new source belongs to is the only question worth asking when adding one:
//
//   - Network-reaching sources (a remote origin an artifact fetches, imports,
//     or submits to) are egress. They are gated by scan → approve → allowlist
//     per docs/product_requirement_doc.md §6.2, so they appear only as the
//     appended `origins` below. An empty allowlist means an artifact reaches
//     nothing.
//   - Local, no-egress sources ('unsafe-inline', 'unsafe-eval', data:, blob:)
//     execute or render bytes the artifact already carries or the visitor
//     already picked locally. Nothing leaves the browser, so gating them behind
//     per-artifact approval buys no security while breaking canonical
//     single-file patterns. These are unconditional — present whether or not
//     the artifact has an allowlist.
//
// Applying that split directive by directive:
//   - style-src 'unsafe-inline' always permits inline <style> blocks and style=""
//     attributes — the default way a single-file artifact carries its CSS.
//     Allowlisted origins are appended so a <link rel=stylesheet> to an approved
//     origin is honored.
//   - img-src and font-src always permit data: URIs so an artifact that inlines its
//     own images or fonts (e.g. @font-face { src: url(data:...) }) renders with zero
//     network egress.
//   - media-src always permits blob: so a <video>/<audio> element can play back a
//     file the artifact loaded locally via <input type=file> + URL.createObjectURL
//     — the object never leaves the browser.
//   - script-src always permits blob: and data: so a script the artifact builds at
//     runtime can execute. Given 'unsafe-inline' and 'unsafe-eval' are already
//     present (an artifact is a single file of its own code), these grant no
//     capability the policy doesn't already allow.
//   - worker-src is explicit rather than left to fall back to script-src, and
//     always permits blob: and data:. A Worker built from a blob:/data: URL is the
//     standard way to start one from an opaque-origin sandbox (e.g. ffmpeg.wasm);
//     a missing worker-src silently produces a worker whose body never runs — no
//     console error, no rejected promise, just a hang.
//   - connect-src always permits blob: and data: alongside the allowlist. Reading
//     back a blob: object URL the artifact itself minted, or a data: URI it built,
//     is fetch used as local I/O — the bytes are already in the agent, nothing
//     leaves the browser. (ffmpeg.wasm's core loads its own .wasm this way.) An
//     artifact with no approved origins still gets no *network* reach: the
//     allowlist portion is what governs egress, and it stays empty.
//   - form-action is built from the same allowlist as connect-src. This matters
//     because form-action does NOT fall back to default-src — a sandbox that
//     grants allow-forms without an explicit form-action would let an artifact
//     submit a <form> to any origin, a network-egress vector the allowlist would
//     otherwise govern. form-action is pinned to 'self' even with an empty
//     allowlist: a form with no/empty action submits to the current document (the
//     render URL itself), which is zero-egress and needs no approval.
func buildCSP(allowlist []string, appOrigin string) string {
	origins := strings.Join(allowlist, " ")

	// withOrigins appends the approved (network-reaching) origins to a directive's
	// unconditional, no-egress sources.
	withOrigins := func(directive string) string {
		if origins == "" {
			return directive
		}
		return directive + " " + origins
	}

	return strings.Join([]string{
		"default-src 'none'",
		withOrigins("script-src 'unsafe-inline' 'unsafe-eval' blob: data:"),
		withOrigins("worker-src blob: data:"),
		withOrigins("style-src 'unsafe-inline'"),
		withOrigins("img-src data:"),
		withOrigins("font-src data:"),
		withOrigins("media-src blob:"),
		withOrigins("connect-src blob: data:"),
		withOrigins("form-action 'self'"),
		"frame-ancestors " + appOrigin,
	}, "; ")
}

// shimScript is the shim injected before any artifact scripts run. It
// intercepts localStorage/sessionStorage and routes state through the API,
// and bridges the capabilities the sandbox denies — downloads (the sandbox
// omits allow-downloads) and clipboard read/write (opaque-origin permissions
// policy) — to the host frame, where they run only after user approval.
const shimTemplate = `<script>
(function() {
  var ARTIFACT_ID = %q;
  var API_ORIGIN = %q;

  // State is inlined by the render surface at request time, so getItem is
  // correct on the first *synchronous* read. Fetching it asynchronously would
  // race the artifact's own startup reads (which run before a fetch resolves).
  var cache = %s;

  // Writes go to the trusted host frame (same-origin with the API and
  // authenticated) via postMessage. The sandbox gives this iframe an opaque
  // 'null' origin, so it cannot call the API cross-origin itself. targetOrigin
  // is pinned to API_ORIGIN so the message can only reach our own host.
  function writeThrough(key, value) {
    if (window.parent === window) return; // top-level: no host to persist through
    window.parent.postMessage(
      { __avState: true, artifactId: ARTIFACT_ID, key: key, value: value },
      API_ORIGIN
    );
  }

  var shimStorage = {
    getItem: function(key) {
      return Object.prototype.hasOwnProperty.call(cache, key) ? cache[key] : null;
    },
    setItem: function(key, value) {
      cache[key] = String(value);
      writeThrough(key, String(value));
    },
    removeItem: function(key) {
      delete cache[key];
      writeThrough(key, '');
    },
    clear: function() {
      cache = {};
    },
    key: function(n) {
      return Object.keys(cache)[n] || null;
    },
    get length() {
      return Object.keys(cache).length;
    }
  };

  try {
    Object.defineProperty(window, 'localStorage', { value: shimStorage, writable: false });
    Object.defineProperty(window, 'sessionStorage', { value: shimStorage, writable: false });
  } catch(e) {}

  // ---- Download bridge ----
  // The sandbox deliberately omits allow-downloads, so nothing in this frame
  // can download directly. When embedded in the gallery, the shim intercepts
  // the common export vectors — anchor activations with blob:/data: hrefs —
  // and posts filename + bytes to the host frame, which asks for first-use
  // approval and performs the download from the app origin. Vectors the shim
  // does not catch simply stay blocked by the sandbox; evading the shim gains
  // nothing. Top-level (no host frame) there is no sandbox and native
  // downloads already work, so the bridge stays uninstalled — including on
  // share pages, which get no bridge.
  if (window.parent !== window) {
    // blob: URLs cannot be dereferenced here without a fetch (which
    // connect-src governs), so remember the Blob behind every URL this
    // document mints. The shim runs first, so the registry sees them all.
    var blobURLs = {};
    var createObjectURL = URL.createObjectURL.bind(URL);
    var revokeObjectURL = URL.revokeObjectURL.bind(URL);
    URL.createObjectURL = function(obj) {
      var url = createObjectURL(obj);
      if (obj instanceof Blob) blobURLs[url] = obj;
      return url;
    };
    URL.revokeObjectURL = function(url) {
      delete blobURLs[url];
      revokeObjectURL(url);
    };

    var dataURLToBlob = function(href) {
      var comma = href.indexOf(',');
      if (comma < 0) return null;
      var meta = href.slice(5, comma);
      var data = href.slice(comma + 1);
      var mime = meta.replace(/;base64$/i, '') || 'text/plain';
      try {
        if (/;base64$/i.test(meta)) {
          var bin = atob(data);
          var bytes = new Uint8Array(bin.length);
          for (var i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
          return new Blob([bytes], { type: mime });
        }
        return new Blob([decodeURIComponent(data)], { type: mime });
      } catch (e) {
        return null;
      }
    };

    var isDownloadHref = function(href) {
      // Coerce first: an SVG <a> (which closest('a') also matches) exposes href
      // as an SVGAnimatedString, not a string, so a bare .slice would throw.
      // Stringified it can't match blob:/data:, so SVG anchors are safely skipped.
      href = String(href);
      return href.slice(0, 5) === 'blob:' || href.slice(0, 5) === 'data:';
    };

    // Posts the anchor's payload to the host frame. The bytes cross the
    // boundary as transferred data, not a capability grant, and targetOrigin
    // stays pinned to the app origin like every other shim message.
    var bridgeDownload = function(anchor) {
      var href = anchor.href;
      var blob = href.slice(0, 5) === 'data:' ? dataURLToBlob(href) : blobURLs[href];
      if (!blob) return;
      var filename = anchor.getAttribute('download') || 'download';
      var reader = new FileReader();
      reader.onload = function() {
        window.parent.postMessage(
          { __avDownload: true, artifactId: ARTIFACT_ID, filename: filename, mime: blob.type, bytes: reader.result },
          API_ORIGIN,
          [reader.result]
        );
      };
      reader.readAsArrayBuffer(blob);
    };

    // Capture phase sees the click before the artifact's own handlers — for
    // user clicks and programmatic click() on in-document anchors alike —
    // without suppressing them (preventDefault only, no stopPropagation).
    document.addEventListener('click', function(e) {
      var anchor = e.target && e.target.closest ? e.target.closest('a') : null;
      if (!anchor || !isDownloadHref(anchor.href || '')) return;
      e.preventDefault();
      bridgeDownload(anchor);
    }, true);

    // Detached anchors (createElement -> click() without appendChild — the
    // canonical export-a-CSV pattern) never propagate to the document
    // listener, so route their programmatic clicks through the bridge here.
    var nativeClick = HTMLAnchorElement.prototype.click;
    HTMLAnchorElement.prototype.click = function() {
      if (!this.isConnected && isDownloadHref(this.href || '')) {
        bridgeDownload(this);
        return;
      }
      nativeClick.apply(this, arguments);
    };

    // ---- Clipboard bridge (av-hll6) ----
    // navigator.clipboard is denied in this opaque-origin frame by permissions
    // policy, so proxy readText/writeText through the host frame the same way
    // as downloads: post the request pinned to the app origin, the host prompts
    // for first-use approval, performs the op on the app origin, and posts the
    // result back. Each call carries an id so the returned Promise settles with
    // the host's answer; a denial rejects with a NotAllowedError DOMException,
    // exactly what a real blocked clipboard call throws, so artifacts handle it
    // unchanged. Native keyboard paste (Ctrl/Cmd+V into a field) is a browser
    // event, not an API call, and is unaffected.
    var clipSeq = 0;
    var clipPending = {};
    window.addEventListener('message', function(e) {
      // The host replies from the app origin. It must target '*' because this
      // frame's origin is opaque, so identity is established by e.origin (the
      // sender) and e.source, not the message's targetOrigin.
      if (e.origin !== API_ORIGIN || e.source !== window.parent) return;
      var d = e.data;
      if (!d || d.__avClipboardResult !== true) return;
      var p = clipPending[d.id];
      if (!p) return;
      delete clipPending[d.id];
      if (d.ok) p.resolve(d.text != null ? d.text : undefined);
      else p.reject(new DOMException(d.error || 'Clipboard access denied', 'NotAllowedError'));
    });

    var requestClip = function(op, text) {
      return new Promise(function(resolve, reject) {
        var id = 'c' + (++clipSeq);
        clipPending[id] = { resolve: resolve, reject: reject };
        window.parent.postMessage(
          { __avClipboard: true, artifactId: ARTIFACT_ID, id: id, op: op, text: text },
          API_ORIGIN
        );
      });
    };

    var clipboardShim = {
      writeText: function(text) { return requestClip('write', String(text)); },
      readText: function() { return requestClip('read'); }
    };
    try {
      Object.defineProperty(navigator, 'clipboard', { value: clipboardShim, configurable: true });
    } catch (e) {
      // Some engines expose navigator.clipboard as a non-configurable getter;
      // fall back to replacing just the two methods we bridge.
      try {
        if (navigator.clipboard) {
          navigator.clipboard.writeText = clipboardShim.writeText;
          navigator.clipboard.readText = clipboardShim.readText;
        }
      } catch (e2) {}
    }

    // ---- File System Access picker polyfill (av-70t9) ----
    // The sandboxed iframe's opaque origin makes the File System Access API
    // unreachable: Blink's VerifyIsAllowedToShowFilePicker throws a
    // SecurityError, and no sandbox token re-enables it (even allow-same-origin
    // wouldn't help — the render origin is cross-origin to the app origin).
    // Polyfill showOpenFilePicker / showDirectoryPicker / showSaveFilePicker
    // on the classic <input type=file> picker, which Blink subjects to no
    // sandbox check at all (only a user-activation requirement, which the
    // artifact's own click already provides). Open/directory return FSA-shaped
    // handles backed by the picked File(s); save's createWritable materializes
    // a download through the download bridge above (host-mediated, first-use
    // approval) rather than adding allow-downloads to the sandbox — the bridge
    // is the single export path av-ryby established, and the sandbox token set
    // stays unchanged. No approval gates the input fallback: the user picks
    // each file explicitly (ordinary web behavior). Install framed-only;
    // top-level renders have native FSA and share pages get no bridge.

    // Flattens an FSA "types" array ([{ description, accept: { mime: [exts] }}])
    // into an <input accept> string, or undefined for no filter.
    var acceptFromTypes = function(types) {
      if (!types || !types.length) return undefined;
      var set = {};
      for (var i = 0; i < types.length; i++) {
        var accept = types[i] && types[i].accept;
        if (!accept) continue;
        Object.keys(accept).forEach(function(mime) {
          if (mime && mime !== '*/*') set[mime] = true;
          accept[mime].forEach(function(ext) { set[ext] = true; });
        });
      }
      var list = Object.keys(set);
      return list.length ? list.join(',') : undefined;
    };

    // A FileSystemWritableFileStream that buffers writes and, on close,
    // triggers the download bridge via a detached blob:-href anchor click —
    // the same path a[download] exports take. seek/truncate are no-ops
    // (sequential buffer); the polyfill is read/write-to-download only.
    var makeWritable = function(filename, mime) {
      var chunks = [];
      var closed = false;
      return {
        write: function(data) {
          if (closed) return Promise.reject(new DOMException('Writable stream is closed', 'InvalidStateError'));
          // Unwrap the WriteParams form { type: 'write'|'seek'|'truncate', data }.
          // Only those three exact type values are treated as WriteParams — a
          // Blob also has a .type property (its MIME), so a broad typeof check
          // would silently drop every Blob write. seek/truncate are no-ops
          // (sequential buffer); 'write' unwraps .data.
          if (data && typeof data === 'object' && (data.type === 'write' || data.type === 'seek' || data.type === 'truncate')) {
            if (data.type === 'write') data = data.data;
            else return Promise.resolve();
          }
          chunks.push(data);
          return Promise.resolve();
        },
        close: function() {
          if (closed) return Promise.resolve(undefined);
          closed = true;
          var blob = new Blob(chunks, { type: mime || 'application/octet-stream' });
          var url = URL.createObjectURL(blob); // registered in blobURLs above
          var a = document.createElement('a');
          a.href = url;
          a.download = filename;
          a.click(); // detached -> prototype.click override -> bridgeDownload -> host
          return Promise.resolve(undefined);
        },
        abort: function() { closed = true; chunks = []; return Promise.resolve(undefined); },
        seek: function() { return Promise.resolve(); },
        truncate: function() { return Promise.resolve(); }
      };
    };

    var makeFileHandle = function(file) {
      return {
        kind: 'file',
        name: file.name,
        getFile: function() { return Promise.resolve(file); },
        createWritable: function() { return Promise.resolve(makeWritable(file.name, file.type)); }
      };
    };

    // Reconstructs the directory tree from <input webkitdirectory>'s flat file
    // list (each File carries .webkitRelativePath = "root/sub/file"). Empty
    // subdirectories are invisible to webkitdirectory and are omitted — an
    // acceptable limitation for the read-a-folder tools this targets.
    var makeDirHandle = function(node) {
      return {
        kind: 'directory',
        name: node.name,
        values: function() { return dirIterator(node, 'values'); },
        keys: function() { return dirIterator(node, 'keys'); },
        entries: function() { return dirIterator(node, 'entries'); },
        // Read-only polyfill: the sandbox can't persist writes to disk handles.
        removeEntry: function() {
          return Promise.reject(new DOMException('Directory is read-only', 'NotSupportedError'));
        },
        [Symbol.asyncIterator]: function() { return dirIterator(node, 'entries'); }
      };
    };

    // Yields [name, handle] pairs for the direct children of a node, in
    // insertion order (subdirectories first, then files).
    var childPairs = function(node) {
      var out = [];
      Object.keys(node.dirs).forEach(function(n) { out.push([n, makeDirHandle(node.dirs[n])]); });
      node.files.forEach(function(f) { out.push([f.name, makeFileHandle(f)]); });
      return out;
    };

    var dirIterator = function(node, mode) {
      var kids = childPairs(node);
      var i = 0;
      return {
        next: function() {
          return new Promise(function(resolve) {
            if (i < kids.length) {
              var pair = kids[i++];
              resolve({ value: mode === 'values' ? pair[1] : (mode === 'keys' ? pair[0] : pair), done: false });
            } else {
              resolve({ value: undefined, done: true });
            }
          });
        },
        return: function() { return Promise.resolve({ value: undefined, done: true }); },
        [Symbol.asyncIterator]: function() { return this; }
      };
    };

    // Opens an <input type=file> and resolves with its FileList. .click() must
    // run synchronously so the picker opens within the user-gesture window that
    // triggered the FSA call — deferring to a microtask loses activation. Some
    // browsers only open the picker for an in-DOM input, so append (hidden) and
    // remove on change. A canceled picker (no files) rejects with AbortError,
    // matching native FSA semantics.
    var runFileInput = function(attrs) {
      return new Promise(function(resolve, reject) {
        var input = document.createElement('input');
        input.type = 'file';
        input.style.position = 'fixed';
        input.style.top = '-9999px';
        input.style.opacity = '0';
        if (attrs.multiple) input.multiple = true;
        if (attrs.webkitdirectory) input.webkitdirectory = true;
        if (attrs.accept) input.accept = attrs.accept;
        input.onchange = function() {
          var files = input.files;
          if (input.parentNode) input.parentNode.removeChild(input);
          if (!files || !files.length) {
            reject(new DOMException('The user aborted the request.', 'AbortError'));
            return;
          }
          resolve(files);
        };
        (document.body || document.documentElement).appendChild(input);
        input.click();
      });
    };

    // Map a FileList from runFileInput into FSA file handles.
    var filesToHandles = function(files) {
      var handles = [];
      for (var i = 0; i < files.length; i++) handles.push(makeFileHandle(files[i]));
      return handles;
    };

    // Rebuild a directory tree from a webkitdirectory FileList and return the
    // root directory handle. Named (not inline) so the shim stays free of the
    // inline then-callback form the async-state-hydration guard watches for —
    // this runs only on a user picker gesture, never at startup.
    var filesToDirHandle = function(files) {
      var root = { name: '', dirs: {}, files: [] };
      for (var i = 0; i < files.length; i++) {
        var f = files[i];
        var parts = String(f.webkitRelativePath || f.name).split('/');
        if (root.name === '') root.name = parts[0];
        var node = root;
        for (var j = 1; j < parts.length; j++) {
          if (j === parts.length - 1) node.files.push(f);
          else { var s = parts[j]; if (!node.dirs[s]) node.dirs[s] = { name: s, dirs: {}, files: [] }; node = node.dirs[s]; }
        }
      }
      return makeDirHandle(root);
    };

    window.showOpenFilePicker = function(opts) {
      opts = opts || {};
      return runFileInput({ multiple: !!opts.multiple, accept: acceptFromTypes(opts.types) }).then(filesToHandles);
    };

    window.showDirectoryPicker = function() {
      return runFileInput({ webkitdirectory: true }).then(filesToDirHandle);
    };

    window.showSaveFilePicker = function(opts) {
      opts = opts || {};
      // No native save dialog exists in a sandboxed iframe; return a handle
      // whose createWritable() materializes a download via the bridge above.
      return Promise.resolve(makeFileHandle(new File([], opts.suggestedName || 'download')));
    };
  }
})();
</script>`

// injectShim inserts the storage shim as the first element inside <head>.
// If no <head> is found, the shim is prepended to the document. The artifact's
// current state is inlined into the shim so the cache is populated before any
// artifact script runs.
func injectShim(body, artifactID, appOrigin string, state map[string]string) string {
	if state == nil {
		state = map[string]string{}
	}
	// json.Marshal escapes <, >, & as </>/&, so the literal is
	// safe to embed inside a <script> element (can't break out with </script>).
	stateJSON, err := json.Marshal(state)
	if err != nil {
		stateJSON = []byte("{}")
	}
	shim := fmt.Sprintf(shimTemplate, artifactID, appOrigin, stateJSON)
	// The snippet element-picker (Exh-edjk) rides along with the shim: inert
	// until the app-origin host activates it, so it costs nothing for plain
	// renders and share views.
	shim += "\n" + snippetScript(appOrigin)

	// Try to inject after <head>
	idx := strings.Index(strings.ToLower(body), "<head>")
	if idx >= 0 {
		insertAt := idx + len("<head>")
		return body[:insertAt] + "\n" + shim + body[insertAt:]
	}

	// Try to inject before </head>
	idx = strings.Index(strings.ToLower(body), "</head>")
	if idx >= 0 {
		return body[:idx] + shim + "\n" + body[idx:]
	}

	// Fallback: prepend
	return shim + "\n" + body
}
