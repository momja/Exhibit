package color

import (
	"strconv"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

// Presets is the preset swatch palette offered in the tag edit/add modals,
// alongside a custom hex input. A small, distinct, colorblind-friendlyish
// spread rather than an exhaustive picker.
var Presets = []string{
	"#6B7280", // gray (store.DefaultTagColor)
	"#EF4444", // red
	"#F59E0B", // amber
	"#10B981", // green
	"#3B82F6", // blue
	"#8B5CF6", // purple
	"#EC4899", // pink
}

// textDark/textLight are the two text colors tag pills choose between,
// matching the body/inverse text tones already used elsewhere in the
// gallery stylesheet (body text is #111, not pure black).
const (
	textDark  = "#111111"
	textLight = "#ffffff"
)

// Normalize validates a tag color for safe use inside an HTML style
// attribute, expanding the 3-digit CSS shorthand hex form (e.g. "#f00",
// same shorthand CSS itself accepts) to the full 6-digit form ("#ff0000").
// Tag colors are user-authored free text (see store.DefaultTagColor and the
// tag create/update handlers), so anything that isn't a well-formed hex
// color falls back to the default rather than being interpolated as-is.
func Normalize(hex string) string {
	if !isValidHex(hex) {
		return strings.ToLower(store.DefaultTagColor)
	}
	if len(hex) == 4 {
		hex = "#" + string(hex[1]) + string(hex[1]) + string(hex[2]) + string(hex[2]) + string(hex[3]) + string(hex[3])
	}
	return strings.ToLower(hex)
}

func isValidHex(hex string) bool {
	if len(hex) != 4 && len(hex) != 7 {
		return false
	}
	if hex[0] != '#' {
		return false
	}
	for i := 1; i < len(hex); i++ {
		ch := hex[i]
		isHexDigit := (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
		if !isHexDigit {
			return false
		}
	}
	return true
}

// ContrastText picks black or white text for readable contrast against a
// tag's background color, using the standard YIQ perceived-brightness
// formula (threshold 128, the common cutoff for this heuristic).
func ContrastText(bgHex string) string {
	hex := Normalize(bgHex)
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	yiq := (float64(r)*299 + float64(g)*587 + float64(b)*114) / 1000
	if yiq >= 128 {
		return textDark
	}
	return textLight
}
