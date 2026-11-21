// Package color holds the logo's pigments and the hex utilities shared by the
// logo, artifact favicon and tag-pill rendering. The gallery's UI accent is
// ink and lives in web/gallery/tokens.css, not here.
// It lives outside internal/api because neither concern is endpoint logic —
// api's gallery and logo rendering just consume it.
package color

// Brand palette: the pigments of the hero painting (Exh-d5ku). The logo
// artwork (internal/api/logo*.svg) spells the same literals, and a test pins
// them together.
const (
	BrandBlue   = "#2f6fc4"
	BrandYellow = "#f2b63c"
	BrandRed    = "#d9432b"
	BrandWhite  = "#f6efdc" // the frame's white cell
	BrandInk    = "#1f2a1c" // frame and strokes
)
