package color

import (
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

var tagPalette = []struct{ hex, name string }{
	{"#6B7280", "Gray"}, // store.DefaultTagColor
	{"#B86562", "Red"},
	{"#AB7B3E", "Amber"},
	{"#377A64", "Green"},
	{"#42838C", "Teal"},
	{"#527CB0", "Blue"},
	{"#8470AC", "Violet"},
	{"#AC6F92", "Pink"},
}

// Presets is the ordered swatch palette offered alongside a custom hex input.
var Presets = func() []string {
	hexes := make([]string, len(tagPalette))
	for i, preset := range tagPalette {
		hexes[i] = preset.hex
	}
	return hexes
}()

// PresetName names the palette for people choosing a color, while the hex value
// remains available for precise custom colors.
func PresetName(hex string) string {
	for _, preset := range tagPalette {
		if strings.EqualFold(hex, preset.hex) {
			return preset.name
		}
	}
	return "Custom"
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
