package api

import (
	_ "embed"
	"encoding/base64"
)

// exhibitLogoSVG is the Exhibit brand mark, embedded from logo.svg so it
// travels with the binary as source; the gallery template injects it as
// trusted markup (template.HTML, see renderGalleryPage) rather than loading
// it as an asset.
//
// logo.svg is the design_files/exhibit_logo.svg artwork with editor-only
// cruft (Inkscape/sodipodi metadata, the XML prolog, export hints) stripped
// and the color.Brand* placeholders resolved to their literal hex values, so
// it renders identically to the design source. The root carries only
// viewBox (no width/height) so CSS can size it, plus role + aria-label for
// an accessible name. It is used two ways: inlined directly in the header,
// and base64-encoded into the favicon data URI below.
//
// It is also the single source the PWA icon build step (web/pwa-icons)
// rasterizes at build time (av-emh4) — one file, two consumers, so the logo
// artwork and the generated icons never drift apart.
//
//go:embed logo.svg
var exhibitLogoSVG string

// exhibitLogoDataURI is the same artwork encoded as an image source, for a
// <link rel="icon"> or an <img>. base64 sidesteps the URL-escaping the SVG's
// many '#' color values would otherwise need in a data URI.
//
// Each use renders in its own image document, so the artwork's element ids
// never collide with the inline header copy — which is why the 404 hero
// (notfound.tmpl) hangs the oversized frame from here instead of inlining a
// second <svg>: two inline copies on one page would duplicate every gradient
// and filter id, and the second copy's url(#…) paint references would resolve
// against the first copy's <defs>.
var exhibitLogoDataURI = "data:image/svg+xml;base64," +
	base64.StdEncoding.EncodeToString([]byte(exhibitLogoSVG))
