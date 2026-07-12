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

	"github.com/artifact-viewer/artifact-viewer/internal/blob"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/go-chi/chi/v5"
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
// Style/font defaults favor the common self-contained artifact:
//   - style-src 'unsafe-inline' always permits inline <style> blocks and style=""
//     attributes — the default way a single-file artifact carries its CSS, needing
//     no network approval. Allowlisted origins are appended so a <link
//     rel=stylesheet> to an approved origin is honored.
//   - img-src and font-src always permit data: URIs so an artifact that inlines its
//     own images or fonts (e.g. @font-face { src: url(data:...) }) renders with zero
//     network egress. This mirrors the "it's just a file" thesis: inlined assets are
//     not network requests, so blocking them buys no security while breaking a
//     canonical pattern. The network boundary (the allowlist) is unaffected.
func buildCSP(allowlist []string, appOrigin string) string {
	frameAncestors := "frame-ancestors " + appOrigin
	if len(allowlist) == 0 {
		return strings.Join([]string{
			"default-src 'none'",
			"script-src 'unsafe-inline' 'unsafe-eval'",
			"style-src 'unsafe-inline'",
			"img-src data:",
			"font-src data:",
			"connect-src 'none'",
			frameAncestors,
		}, "; ")
	}

	origins := strings.Join(allowlist, " ")
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'unsafe-inline' 'unsafe-eval' " + origins,
		"style-src 'unsafe-inline' " + origins,
		"img-src data: " + origins,
		"font-src data: " + origins,
		"connect-src " + origins,
		frameAncestors,
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
