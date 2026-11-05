package color

import (
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

// Presets is the preset swatch palette offered in the tag edit/add modals,
// alongside a custom hex input. A small, distinct, colorblind-friendlyish
// spread rather than an exhaustive picker.
var Presets = []string{
	"#6B7280", // gray (store.DefaultTagColor)
	"#B86562", // rose red
	"#AB7B3E", // amber
	"#377A64", // green
	"#42838C", // teal
	"#527CB0", // blue
	"#8470AC", // violet
	"#AC6F92", // rose
}

// PresetName names the palette for people choosing a color, while the hex value
// remains available for precise custom colors.
func PresetName(hex string) string {
	switch strings.ToUpper(hex) {
	case "#6B7280":
		return "Gray"
	case "#B86562":
		return "Red"
	case "#AB7B3E":
		return "Amber"
	case "#377A64":
		return "Green"
	case "#42838C":
		return "Teal"
	case "#527CB0":
		return "Blue"
	case "#8470AC":
		return "Violet"
	case "#AC6F92":
		return "Pink"
	default:
		return "Custom"
	}
}

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
