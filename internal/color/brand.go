// Package color holds the color constants and hex utilities shared by the
// gallery's brand theming (logo, header accent) and its tag-pill rendering.
// It lives outside internal/api because neither concern is endpoint logic —
// api's gallery and logo rendering just consume it.
package color

// Brand palette. Single source of truth for the Exhibit logo artwork and the
// gallery UI's accent color.
const (
	BrandBlue      = "#23559e"
	BrandBlueHover = "#1a4076" // hand-picked darker shade; BrandBlue has no existing hover pair
	BrandYellow    = "#fae317"
	BrandRed       = "#de281d"
)
