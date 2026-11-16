package api

import (
	_ "embed"
	"encoding/base64"
)

// exhibitLogoSVG is the flat Exhibit mark: the frame alone, in a tight
// viewBox, with no wire, nail or shadow (Exh-bv8l). It is the brand mark
// everywhere except the 404 hero: the header, sign-in and Clerk pages, admin,
// the app and render-origin favicons, and the PWA icons and logo OG card
// rasterized from it (web/pwa-icons).
//
// The artwork descends from design_files/exhibit_logo.svg with editor-only
// cruft stripped and the color.Brand* placeholders resolved to literal hex
// values. The root carries only viewBox (no width/height) so CSS can size it,
// plus role + aria-label for an accessible name. It is inlined in page
// headers and base64-encoded into the favicon data URI below. It declares no
// ids, so any number of inline copies can share a page.
//
//go:embed logo.svg
var exhibitLogoSVG string

// exhibitHangingLogoSVG is the same frame hung on a nail by a wire, for the
// places where the picture-on-a-wall metaphor helps: today only the 404 hero.
// Nail, wire apex and frame centre share one vertical axis, the wire's legs
// land symmetrically on the frame's outer top edge, and the wire matches the
// frame's stroke width. One shadow direction (down and right) falls under the
// frame and the nail; the wire casts none.
//
// The wire and the nail are drawn in plain black on the page itself rather
// than on the white frame, so they are what a dark ground would swallow
// (av-xie4). They carry their color as attributes, which CSS outranks, and the
// SVG's own <style> lightens them under prefers-color-scheme: dark. As an
// image that follows the browser's scheme. The artwork declares no ids (no
// gradients or filters), so it cannot clash with another copy on the page.
//
//go:embed logo-hanging.svg
var exhibitHangingLogoSVG string

// exhibitLogoDataURI is the flat mark encoded as an image source, for a
// <link rel="icon"> or an <img>. base64 sidesteps the URL-escaping the SVG's
// many '#' color values would otherwise need in a data URI.
var exhibitLogoDataURI = svgDataURI(exhibitLogoSVG)

// exhibitHangingLogoDataURI is the hanging variant as an image source. The
// 404 hero uses it as an <img>, so the artwork lives in its own image document.
var exhibitHangingLogoDataURI = svgDataURI(exhibitHangingLogoSVG)

func svgDataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
