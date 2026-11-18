// Package color holds the logo's pigments and the hex utilities shared by the
// logo, artifact favicon and tag-pill rendering. The gallery's UI accent is
// ink and lives in web/gallery/tokens.css, not here.
// It lives outside internal/api because neither concern is endpoint logic —
// api's gallery and logo rendering just consume it.
package color

// Brand palette: the Exhibit logo artwork's pigments.
const (
	BrandBlue   = "#23559e"
	BrandYellow = "#fae317"
	BrandRed    = "#de281d"
)
