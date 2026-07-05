package api

import (
	"strconv"
	"strings"

	"github.com/artifact-viewer/artifact-viewer/internal/store"
)

// Brand palette. Single source of truth for the Exhibit logo artwork
// (logo.go) and the gallery UI's accent color (gallery.go).
const (
	brandBlue      = "#23559e"
	brandBlueHover = "#1a4076" // hand-picked darker shade; brandBlue has no existing hover pair
	brandYellow    = "#fae317"
	brandRed       = "#de281d"
)

// pillTextLight/Dark are the two text colors tag pills choose between,
// matching the body/inverse text tones already used elsewhere in the
// gallery stylesheet (body text is #111, not pure black).
const (
	pillTextDark  = "#111111"
	pillTextLight = "#ffffff"
)

// tagColorPresets is the preset swatch palette offered in the tag edit/add
// modals, alongside a custom hex input. A small, distinct, colorblind-
// friendlyish spread rather than an exhaustive picker.
var tagColorPresets = []string{
	"#6B7280", // gray (store.DefaultTagColor)
	"#EF4444", // red
	"#F59E0B", // amber
	"#10B981", // green
	"#3B82F6", // blue
	"#8B5CF6", // purple
	"#EC4899", // pink
}

// normalizeHexColor validates a tag color for safe use inside an HTML style
// attribute, expanding #RGB to #RRGGBB. Tag colors are user-authored free
// text (see store.DefaultTagColor and createTag), so anything that isn't a
// well-formed hex color falls back to the default rather than being
// interpolated as-is.
func normalizeHexColor(c string) string {
	if !isHexColor(c) {
		return strings.ToLower(store.DefaultTagColor)
	}
	if len(c) == 4 {
		c = "#" + string(c[1]) + string(c[1]) + string(c[2]) + string(c[2]) + string(c[3]) + string(c[3])
	}
	return strings.ToLower(c)
}

func isHexColor(c string) bool {
	if len(c) != 4 && len(c) != 7 {
		return false
	}
	if c[0] != '#' {
		return false
	}
	for i := 1; i < len(c); i++ {
		ch := c[i]
		isHexDigit := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
		if !isHexDigit {
			return false
		}
	}
	return true
}

// pillTextColor picks black or white text for readable contrast against a
// tag's background color, using the standard YIQ perceived-brightness
// formula (threshold 128, the common cutoff for this heuristic).
func pillTextColor(bgHex string) string {
	hex := normalizeHexColor(bgHex)
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	yiq := (float64(r)*299 + float64(g)*587 + float64(b)*114) / 1000
	if yiq >= 128 {
		return pillTextDark
	}
	return pillTextLight
}
