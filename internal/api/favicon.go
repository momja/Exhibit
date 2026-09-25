package api

import (
	"net/http"
)

// faviconCacheControl is how long a browser may keep the brand mark without
// re-requesting it. The artwork is compiled into the binary (logo.go), so a
// new deploy is a new URL fetch away — a day of caching costs nothing on
// redeploy and spares every later tab its own request.
const faviconCacheControl = "public, max-age=86400"

// favicon serves GET /favicon.ico on the app origin (nw-6184). It is the
// same artwork the pages inline as their data-URI <link rel="icon"> —
// exhibitLogoSVG, the compiled-in brand mark (logo.go) — served as a file
// for the requests no link tag covers: a browser's automatic /favicon.ico
// probe, a bookmark, a tab restored from history. Static and public, the
// same trust level as the manifest, so it needs no auth and no per-request
// store lookup. The bytes are SVG despite the .ico path: every modern
// browser decodes an SVG favicon from whatever URL served it, and keeping
// the conventional path is what catches the automatic probe.
func (ro *Router) favicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", faviconCacheControl)
	_, _ = w.Write([]byte(exhibitLogoSVG))
}
