// Package render provides the render surface: a read-only HTTP handler that
// serves artifact HTML documents wrapped in a per-artifact CSP and the
// storage shim. It runs on RENDER_ORIGIN, separate from the app origin.
package render

import (
	"fmt"
	"io"
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

	csp := buildCSP(a.NetworkAllowlist)
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Prevent the render origin from being framed by arbitrary pages
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")

	doc := injectShim(string(bodyBytes), a.ID, rd.cfg.AppOrigin)
	fmt.Fprint(w, doc)
}

// buildCSP generates a per-artifact Content-Security-Policy header value
// from the artifact's network allowlist.
func buildCSP(allowlist []string) string {
	if len(allowlist) == 0 {
		// No network access permitted
		return strings.Join([]string{
			"default-src 'none'",
			"script-src 'unsafe-inline' 'unsafe-eval'",
			"style-src 'unsafe-inline'",
			"img-src data:",
			"connect-src 'none'",
		}, "; ")
	}

	origins := strings.Join(allowlist, " ")
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'unsafe-inline' 'unsafe-eval' " + origins,
		"style-src 'unsafe-inline' " + origins,
		"img-src data: " + origins,
		"connect-src " + origins,
		"font-src " + origins,
	}, "; ")
}

// shimScript is the storage shim injected before any artifact scripts run.
// It intercepts localStorage/sessionStorage and routes state through the API.
const shimTemplate = `<script>
(function() {
  var ARTIFACT_ID = %q;
  var API_ORIGIN = %q;

  // In-memory cache hydrated from the server
  var cache = {};
  var hydrated = false;

  // Hydrate state from API on load
  fetch(API_ORIGIN + '/api/artifacts/' + ARTIFACT_ID + '/state', {
    headers: { 'Content-Type': 'application/json' }
  }).then(function(r) {
    return r.ok ? r.json() : {};
  }).then(function(state) {
    cache = state || {};
    hydrated = true;
  }).catch(function() {
    hydrated = true;
  });

  function writeThrough(key, value) {
    fetch(API_ORIGIN + '/api/artifacts/' + ARTIFACT_ID + '/state', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: key, value: value })
    }).catch(function() {});
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
})();
</script>`

// injectShim inserts the storage shim as the first element inside <head>.
// If no <head> is found, the shim is prepended to the document.
func injectShim(body, artifactID, appOrigin string) string {
	shim := fmt.Sprintf(shimTemplate, artifactID, appOrigin)

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
