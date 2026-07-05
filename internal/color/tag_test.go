package color

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContrastText(t *testing.T) {
	assert.Equal(t, textDark, ContrastText("#FFFFFF"))
	assert.Equal(t, textLight, ContrastText("#000000"))
	assert.Equal(t, textLight, ContrastText("#111111"))
	assert.Equal(t, textDark, ContrastText("#FAE317")) // BrandYellow
}

func TestNormalizeFallback(t *testing.T) {
	assert.Equal(t, "#ff0000", Normalize("#f00"))
	assert.Equal(t, "#abcdef", Normalize("#ABCDEF"))
	assert.Equal(t, "#6b7280", Normalize("not-a-color"))
	assert.Equal(t, "#6b7280", Normalize(""))
}

func TestIsValidHex(t *testing.T) {
	assert.True(t, isValidHex("#fff"))
	assert.True(t, isValidHex("#FFFFFF"))
	assert.False(t, isValidHex("fff"))     // missing leading '#'
	assert.False(t, isValidHex("#ff"))     // wrong length
	assert.False(t, isValidHex("#gggggg")) // non-hex digits
}
