package api

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Neither artwork may declare an id: gradient and filter ids are what clash
// when two inline copies share a page (Exh-bv8l).
func TestLogoArtworkDeclaresNoIDs(t *testing.T) {
	for name, svg := range map[string]string{"flat": exhibitLogoSVG, "hanging": exhibitHangingLogoSVG} {
		assert.NotRegexp(t, regexp.MustCompile(`\sid="`), svg, name)
		assert.NotContains(t, svg, "<defs", name)
		assert.NotContains(t, svg, "url(#", name)
	}
}

// The flat mark is the frame alone: no wire, nail or shadow.
func TestFlatLogoIsFrameOnly(t *testing.T) {
	for _, part := range []string{"logo-wire", "logo-nail", "logo-shadow", "<style", "<filter", "<circle"} {
		assert.NotContains(t, exhibitLogoSVG, part)
	}
}

// The hanging variant hangs the very same frame: every frame element of the
// flat mark appears verbatim in it.
func TestHangingLogoContainsTheFlatFrame(t *testing.T) {
	for _, line := range strings.Split(strings.TrimSpace(exhibitLogoSVG), "\n")[1:] {
		if line == "</svg>" {
			continue
		}
		assert.Contains(t, exhibitHangingLogoSVG, line)
	}
	for _, part := range []string{"logo-wire", "logo-nail", "prefers-color-scheme:dark"} {
		assert.Contains(t, exhibitHangingLogoSVG, part)
	}
}
