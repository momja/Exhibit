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
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/store"
)

// Config holds the dependencies for the render surface.
type Config struct {
	Store        store.Store
	Blob         blob.Store
	AppOrigin    string
	RenderOrigin string
	// Tokens verifies the short-lived (artifact, owner) credential that /a/:id
	// and /w/:id require (av-c5aq). It is how this surface learns who it is
	// serving without holding a session — see internal/rendertoken for why a
	// cookie here would be readable by the artifact itself. A nil Signer fails
	// those two routes closed; /s/:shareID never consults it.
	Tokens *rendertoken.Signer
}

// Renderer handles render-origin requests.
type Renderer struct {
	cfg Config
}

// New creates a Renderer with the given config.
func New(cfg Config) *Renderer {
	return &Renderer{cfg: cfg}
}

// ServeArtifact serves the artifact identified by {artifactID} from the URL,
// to the principal named by the request's render token.
func (rd *Renderer) ServeArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, principal, ok := rd.authorize(w, r, id)
	if !ok {
		return
	}
	rd.serveArtifactDoc(w, r, a, principal)
}

// authorize is the front door for the two token-gated routes. It verifies the
// request's token against the artifact id in the URL, loads the artifact, and
// confirms the token's owner is the artifact's owner.
//
// Order matters: the token is checked before the store is touched, so an
// unauthenticated caller cannot use response timing or status to learn whether
// an id exists. And every failure past that point answers 404, not 403 — "your
// token is wrong" and "there is no such artifact" must look identical from
// outside, or the surface becomes an id oracle for other tenants' libraries.
func (rd *Renderer) authorize(w http.ResponseWriter, r *http.Request, id string) (*store.Artifact, int64, bool) {
	if rd.cfg.Tokens == nil {
		// No signer configured: nothing can present a valid token, so nothing
		// may render. Failing closed is the point — an open render surface is
		// the vulnerability this gate exists to close.
		http.Error(w, "not found", http.StatusNotFound)
		return nil, 0, false
	}
	principal, err := rd.cfg.Tokens.Verify(r.URL.Query().Get(rendertoken.Param), id)
	if err != nil {
		slog.InfoContext(r.Context(), "render token rejected",
			slog.String("artifact_id", id), slog.String("err", err.Error()))
		http.Error(w, "not found", http.StatusNotFound)
		return nil, 0, false
	}
	// Scoped by the token's principal (av-ep8k): the render surface now has an
	// owner to read as, so this is an ordinary owner-scoped read rather than
	// one of the explicitly-named unscoped accessors. A cross-tenant id is
	// therefore already indistinguishable from a nonexistent one here.
	a, err := rd.cfg.Store.GetArtifact(r.Context(), principal, id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil, 0, false
	}
	// The owner check is belt to the signature's braces: the token already
	// names one artifact, so this can only fire if something minted a token for
	// an artifact its owner does not own. Cheap, and it keeps the cross-tenant
	// guarantee true even if a future minting call site gets the owner wrong.
	if a == nil || a.OwnerID != principal {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, 0, false
	}
	return a, principal, true
}

// ServeShare serves an artifact via a share link.
func (rd *Renderer) ServeShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	// The share row is the authorization here (architecture §7), so this
	// path is owner-independent by design — not an oversight.
	sh, err := rd.cfg.Store.GetShareUnscoped(r.Context(), shareID)
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

	a, err := rd.cfg.Store.GetArtifactUnscoped(r.Context(), sh.ArtifactID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	// The share row is the authorization (architecture.md §7), so this route
	// carries no token and has no principal of its own. State is inlined for
	// the artifact's owner: a share publishes the artifact *as its owner sees
	// it*, which is what a link recipient with no account can be shown.
	rd.serveArtifactDoc(w, r, a, a.OwnerID)
}

// ServeWidget serves an artifact's widget (av-fafu) — the small, informative
// document its gallery card renders. It is the same read path as the artifact
// itself, differing only in which blob it reads and which preamble it injects
// (widget mode: state readable, writes and capability bridges absent). An
// artifact with no widget 404s; the gallery renders its default tile instead
// and never points a frame here.
func (rd *Renderer) ServeWidget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, principal, ok := rd.authorize(w, r, id)
	if !ok {
		return
	}
	if a.WidgetBlobID == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rd.serveDoc(w, r, a, a.WidgetBlobID, true, principal)
}

// serveArtifactDoc serves the artifact's own body — the full, interactive tool.
func (rd *Renderer) serveArtifactDoc(w http.ResponseWriter, r *http.Request, a *store.Artifact, principal int64) {
	rd.serveDoc(w, r, a, a.SourceBlobID, false, principal)
}

// serveDoc reads blobID, wraps it in the artifact's security envelope (CSP from
// the artifact's allowlist, render preamble with the artifact's state inlined),
// and writes the resulting document.
//
// The artifact and its widget deliberately share this one path. The security
// envelope is a property of the *artifact*, not of which document is being
// served: same allowlist, same CSP, same opaque-origin sandbox. widget only
// selects the narrower preamble, so a widget's authority can only ever be a
// subset of its artifact's — there is no second policy to keep in sync.
// principal is the user this document is being rendered *for*: the owner named
// by a verified render token, or (on a share) the artifact's own owner. Today
// it selects nothing, because artifact_state is keyed by artifact alone — but
// it is the answer to "whose state should be inlined here", which is the
// question av-q0ub makes load-bearing when state gains a principal column. It
// is threaded and logged now so that ticket changes one call, not a call chain.
func (rd *Renderer) serveDoc(w http.ResponseWriter, r *http.Request, a *store.Artifact, blobID string, widget bool, principal int64) {
	rc, err := rd.cfg.Blob.Get(r.Context(), blobID)
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
	// The owner comes from the artifact this handler already resolved, not
	// from a request that has none — so the state read stays owner-scoped
	// without a third unscoped accessor.
	state, err := rd.cfg.Store.GetState(r.Context(), a.OwnerID, a.ID)
	if err != nil {
		slog.WarnContext(r.Context(), "render state read failed",
			slog.String("artifact_id", a.ID), slog.String("err", err.Error()))
		state = nil
	}

	doc := injectPreamble(string(bodyBytes), a.ID, rd.cfg.AppOrigin, state, widget)
	slog.DebugContext(r.Context(), "rendered artifact",
		slog.String("artifact_id", a.ID),
		slog.Int64("principal", principal),
		slog.Bool("widget", widget),
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
// intercepts the two Web Storage namespaces — localStorage, whose backing is
// swapped to the server, and (framed only) sessionStorage, which stays in
// memory for the life of the frame — and bridges the capabilities the sandbox
// denies — downloads (the sandbox omits allow-downloads) and clipboard
// read/write (opaque-origin permissions policy) — to the host frame, where
// they run only after user approval.
//
// WIDGET (av-fafu) narrows the same shim for a widget render. A widget is a
// *view* of an artifact: it reads the artifact's state and shows one fact from
// it. So in widget mode writes stop at the in-memory cache — the write-through
// to the host is short-circuited — and bridgeScript below is not spliced in at
// all. Both are subtractions from the one preamble rather than a second shim,
// which is what makes "a widget's authority is a strict subset of its
// artifact's" a property you can read off this file instead of a claim two
// files have to keep agreeing on. A widget that calls setItem still behaves
// like Storage within its own frame — it just cannot outlive the render.
const shimTemplate = `<script>
(function() {
  var ARTIFACT_ID = %q;
  var API_ORIGIN = %q;
  var WIDGET = %t;

  // State is inlined by the render surface at request time, so getItem is
  // correct on the first *synchronous* read. Fetching it asynchronously would
  // race the artifact's own startup reads (which run before a fetch resolves).
  var cache = %s;

  // Writes go to the trusted host frame (same-origin with the API and
  // authenticated) via postMessage. The sandbox gives this iframe an opaque
  // 'null' origin, so it cannot call the API cross-origin itself. targetOrigin
  // is pinned to API_ORIGIN so the message can only reach our own host.
  //
  // op is explicit ('set' | 'delete' | 'clear') rather than inferred from
  // key/value, because a delete must be unambiguously distinct from setting a
  // key to '' — storing an empty string is legitimate and must stay possible.
  // clear has no key at all, so it gets its own op rather than a sentinel key.
  function persistState(op, key, value) {
    if (WIDGET) return;                    // a widget renders state, never edits it
    if (window.parent === window) return; // top-level: no host to persist through
    var msg = { __avState: true, artifactId: ARTIFACT_ID, op: op };
    if (key !== undefined) msg.key = key;
    if (value !== undefined) msg.value = value;
    window.parent.postMessage(msg, API_ORIGIN);
  }

  // makeStorage builds one Storage-shaped object over its OWN cache. Each Web
  // Storage namespace gets its own call, so a key written to one is invisible
  // to the other — they are distinct namespaces with distinct lifetimes, and
  // artifacts are written against that ('draft' in sessionStorage is this
  // session's scratch copy; 'draft' in localStorage is the saved one).
  // persist, when given, bridges a mutation onward; the namespace that passes
  // null keeps every write inside this frame.
  function makeStorage(initial, persist) {
    var store = initial;
    return {
      getItem: function(key) {
        return Object.prototype.hasOwnProperty.call(store, key) ? store[key] : null;
      },
      setItem: function(key, value) {
        store[key] = String(value);
        if (persist) persist('set', key, String(value));
      },
      removeItem: function(key) {
        delete store[key];
        if (persist) persist('delete', key);
      },
      clear: function() {
        store = {};
        if (persist) persist('clear');
      },
      key: function(n) {
        // Index the array rather than testing the value for truthiness: ""
        // is a legitimate stored key, and '|| null' would report it missing.
        var keys = Object.keys(store);
        return n >= 0 && n < keys.length ? keys[n] : null;
      },
      get length() {
        return Object.keys(store).length;
      }
    };
  }

  // localStorage is the persisted namespace: its cache is the state inlined
  // above and every mutation writes through to the host frame. Installed
  // unconditionally — the native getter throws on the sandbox's opaque origin,
  // and top-level it still serves the inlined reads.
  try {
    Object.defineProperty(window, 'localStorage', { value: makeStorage(cache, persistState), writable: false });
  } catch(e) {}
%s
})();
</script>`

// bridgeScript is the capability half of the render preamble: the bridges and
// polyfills that give an artifact back what the opaque-origin sandbox takes
// away (downloads, clipboard, file pickers) and the diagnostics for what it
// cannot give back at all.
//
// It is a separate string, spliced into shimTemplate's trailing %s, because a
// widget render omits it entirely rather than shipping it disabled (av-fafu).
// Omission is both the honest encoding of "a widget has none of these
// capabilities" and the cheap one: a gallery page renders one widget document
// per card, and none of them should carry a download bridge they can't use.
//
// ---- Framed-only installs ----
// Everything below belongs to the sandboxed, opaque-origin frame the gallery
// embeds: the sessionStorage namespace, the capability diagnostic, and the
// bridges and polyfills that stand in for what that sandbox denies. Opened
// top-level (a direct render-origin visit or a share) there is no host frame
// and no sandbox — the document has a real origin where all of this works
// natively — so none of it installs.
const bridgeScript = `
  if (window.parent !== window) {
    // ---- sessionStorage ----
    // Its own, purely in-memory Storage object: separate cache, no persist
    // callback, nothing server-side. sessionStorage means 'dies with the
    // session' and artifacts pick it precisely for what must not survive — a
    // dismissed banner, a wizard's in-progress step — so persisting it
    // cross-device would invert the lifetime the author chose.
    //
    // In-memory is not an approximation here, it is exactly the native
    // behavior: a sandboxed browsing context is assigned a FRESH opaque origin
    // on every navigation and storage is keyed by origin, so native
    // sessionStorage would likewise start empty after each (re)load and would
    // be shared with no other frame. Installing something is forced — an
    // opaque origin gets no storage key at all, so the native getter throws a
    // SecurityError on property *access*, killing any artifact that reads
    // storage at the top of its script.
    try {
      Object.defineProperty(window, 'sessionStorage', { value: makeStorage({}, null), writable: false });
    } catch(e) {}

    // ---- Unsupported-capability diagnostic (av-yvtb) ----
    // Some browser capabilities cannot work inside this opaque-origin sandbox no
    // matter the CSP, and fail silently rather than throwing something the
    // artifact surfaces. We detect those cases and post a GENERIC
    // '__avCapabilityWarning' to the host frame, which shows a banner offering
    // the top-level render (a real origin, where the capability works). The
    // payload names the capability (so the host can describe it) and an optional
    // resource string; the channel is intentionally capability-agnostic so
    // future detections (SharedWorker, service-worker registration — both also
    // fail on an opaque origin) reuse it without a host-side rewrite.
    //
    // Phase 1 detects exactly one capability: a module Worker. Chrome refuses to
    // fetch a module worker's script for an opaque origin, so a Worker
    // constructed with { type: 'module' } here (origin 'null') fires onerror
    // with an empty message and never runs — no securitypolicyviolation, so it
    // is not a CSP fault and cannot be relaxed with CSP. Classic blob:/data:
    // workers run fine here (av-x01o). The result is a silent, indefinite hang
    // (ffmpeg.wasm 0.12 always spawns a module worker). We do NOT change runtime
    // behavior: the real Worker still constructs and returns; it just fails on
    // its own as before. Debounced to the first warning so a library spawning
    // many workers warns once.
    //
    // Gate on self.origin (the document's *effective*, security-relevant
    // origin), NOT location.origin: for an http-loaded opaque-sandbox document
    // location.origin still reports the URL's tuple origin (e.g.
    // 'http://render.example'), while self.origin / window.origin serialize the
    // opaque origin to the string 'null' — the same value the host sees as
    // e.origin. That is the condition the module-worker fetch actually fails on.
    if (self.origin === 'null') {
      // Buffer the diagnostic (also the first-occurrence debounce): the trigger
      // (e.g. a module worker) is typically constructed at load, which can race
      // the host page's listener attachment — a one-shot postMessage would be
      // lost if it fires first. So keep the payload and (a) post it immediately
      // for the case the host is already listening, and (b) replay it whenever
      // the host announces itself with an __avHostReady ping (the host sends one
      // on iframe load). Between the two, delivery is guaranteed regardless of
      // load ordering.
      var pendingCapabilityWarning = null;
      var postCapabilityWarning = function() {
        if (pendingCapabilityWarning) window.parent.postMessage(pendingCapabilityWarning, API_ORIGIN);
      };
      // warnCapability records the first unsupported capability seen (capability
      // is a stable slug the host maps to copy; resource is an optional detail,
      // e.g. the worker script URL) and posts it.
      var warnCapability = function(capability, resource) {
        if (pendingCapabilityWarning) return;
        pendingCapabilityWarning = { __avCapabilityWarning: true, artifactId: ARTIFACT_ID, capability: capability, resource: resource || null };
        postCapabilityWarning();
      };
      window.addEventListener('message', function(e) {
        // Only our own host (app origin) may trigger a replay, like every other
        // shim message; the frame's origin is opaque so identity is e.source.
        if (e.origin !== API_ORIGIN || e.source !== window.parent) return;
        if (e.data && e.data.__avHostReady === true) postCapabilityWarning();
      });
    }

    // Module-worker detection (phase 1) — wraps the Worker constructor to spot
    // the { type: 'module' } case above and warn via warnCapability.
    if (self.origin === 'null' && typeof Worker === 'function') {
      var NativeWorker = Worker;
      var WorkerShim = function(scriptURL, options) {
        if (options && options.type === 'module') {
          var url = null;
          try { url = scriptURL != null ? String(scriptURL) : null; } catch (e) { url = null; }
          warnCapability('module-worker', url);
        }
        // Construct the real Worker transparently: forward all args with 'new'
        // so 'this', the prototype chain, and the return value are unchanged.
        return new (Function.prototype.bind.apply(NativeWorker, [null].concat([].slice.call(arguments))))();
      };
      WorkerShim.prototype = NativeWorker.prototype;
      try {
        Object.defineProperty(window, 'Worker', { value: WorkerShim, writable: true, configurable: true });
      } catch (e) {}
    }

    // ---- Download bridge (av-ryby) ----
    // The sandbox deliberately omits allow-downloads, so nothing in this frame
    // can download directly. The bridge intercepts the common export vectors —
    // anchor activations with blob:/data: hrefs — and posts filename + bytes to
    // the host frame, which asks for first-use approval and performs the
    // download from the app origin. Vectors it does not catch simply stay
    // blocked by the sandbox; evading the bridge gains nothing.
    //
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
  }`

// widgetBaseCSS is one of two things a widget render adds that an artifact
// render does not: a floor for the card tile a widget is drawn into. A widget
// has no page of its own to establish a viewport — it fills a fixed-size well —
// so the default 8px body margin and `height:auto` would leave every widget
// author writing the same four rules. Transparent by default so the card
// surface shows through; a widget that wants its own background paints one. It
// is emitted before the widget's own markup, so anything the widget declares
// wins.
const widgetBaseCSS = `<style>
html,body{margin:0;padding:0;height:100%;background:transparent;overflow:hidden}
body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;color:#111;-webkit-font-smoothing:antialiased}
*{box-sizing:border-box}
</style>`

// widgetHealthScript is the other: a widget vouching for itself to the host.
//
// The host cannot see into this frame — it is cross-origin and opaque, so an
// iframe `load` event fires just the same for a 404 page, for a widget whose
// script threw on line one, and for one that rendered perfectly. From outside,
// all three look identical, and the failure mode a card shows is a blank
// rectangle where a number should be. For a surface whose whole job is to be
// trustworthy at a glance, blank-with-no-explanation is the worst answer
// available.
//
// So the report comes from inside, via the one script in the frame that is
// ours and runs first. On load (plus a frame, so a widget that paints in a
// rAF or a load handler still counts) it checks that something was actually
// rendered and posts __avWidgetReady; an uncaught error, a rejected promise,
// or an empty body posts __avWidgetError instead. The host falls back to the
// default monogram tile on an error — or on hearing nothing at all, which
// covers the cases no in-frame script can report: a document that never
// loaded, a parse failure, a script that hung the thread.
//
// This is diagnosis, not enforcement: a widget that suppresses the report just
// gets the monogram, which is the same outcome as failing. Nothing here can
// grant it anything.
const widgetHealthScript = `<script>
(function() {
  if (window.parent === window) return; // top-level: no host to report to
  var API_ORIGIN = %q;
  var sent = false;
  function post(type, detail) {
    if (sent) return;
    sent = true;
    window.parent.postMessage({ __avWidget: true, status: type, detail: detail || null }, API_ORIGIN);
  }
  window.addEventListener('error', function(e) {
    post('error', e && e.message ? String(e.message) : 'script error');
  });
  window.addEventListener('unhandledrejection', function() { post('error', 'unhandled rejection'); });
  window.addEventListener('load', function() {
    requestAnimationFrame(function() {
      // "Rendered nothing" is a failure by contract: a widget must always draw
      // something, an empty state included. Element children or any
      // non-whitespace text is enough to count as drawn — this is a liveness
      // check, not a design review.
      var body = document.body;
      var drew = body && (body.children.length > 0 || (body.textContent || '').trim() !== '');
      if (drew) post('ready');
      else post('error', 'widget rendered nothing');
    });
  });
})();
</script>`

// injectPreamble inserts the render preamble as the first element inside <head>
// (prepended to the document if it has no <head>). The artifact's current state
// is inlined into the shim so the cache is populated before any script runs.
//
// widget selects the narrower preamble for a widget render (av-fafu): the same
// storage shim with writes stopping at the cache, no capability bridges, no
// element picker, plus the widget base stylesheet.
func injectPreamble(body, artifactID, appOrigin string, state map[string]string, widget bool) string {
	if state == nil {
		state = map[string]string{}
	}
	// json.Marshal escapes <, >, & as </>/&, so the literal is
	// safe to embed inside a <script> element (can't break out with </script>).
	stateJSON, err := json.Marshal(state)
	if err != nil {
		stateJSON = []byte("{}")
	}
	bridges := bridgeScript
	if widget {
		bridges = ""
	}
	shim := fmt.Sprintf(shimTemplate, artifactID, appOrigin, widget, stateJSON, bridges)
	if widget {
		// No snippet picker: it exists so the user can point at an element in
		// the *artifact* preview and hand it to the agent. A widget frame is
		// pointer-events:none, so there is nothing to point at.
		shim += "\n" + widgetBaseCSS
		shim += "\n" + fmt.Sprintf(widgetHealthScript, appOrigin)
	} else {
		// The snippet element-picker (Exh-edjk) rides along with the shim: inert
		// until the app-origin host activates it, so it costs nothing for plain
		// renders and share views.
		shim += "\n" + snippetScript(appOrigin)
	}

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
