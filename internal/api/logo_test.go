package api

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// The wire and nail are lightened by the SVG's own stylesheet in dark mode. An
// inline style="" outranks that rule, so their paint must be presentation
// attributes, in both artworks.
func TestLogoWireAndNailPaintIsNotInlineStyle(t *testing.T) {
	tag := regexp.MustCompile(`<[^>]*class="logo-(?:wire|nail)"[^>]*>`)
	for name, svg := range map[string]string{"flat": exhibitLogoSVG, "hanging": exhibitHangingLogoSVG} {
		for _, el := range tag.FindAllString(svg, -1) {
			assert.NotContains(t, el, "style=", name+": "+el)
		}
	}
	assert.Len(t, tag.FindAllString(exhibitHangingLogoSVG, -1), 2)
	assert.Contains(t, exhibitHangingLogoSVG, ".logo-wire{stroke:#b4bac4}")
	assert.Contains(t, exhibitHangingLogoSVG, ".logo-nail{fill:#b4bac4}")
}

// Both artworks are painted in the pigments of internal/color and no others
// (Exh-d5ku); the hanging variant's dark-mode wire lightening and the black
// shadow are the only other colors.
func TestLogoArtworkUsesBrandPigments(t *testing.T) {
	hex := regexp.MustCompile(`#[0-9a-fA-F]{6}`)
	pigments := []string{color.BrandBlue, color.BrandRed, color.BrandYellow, color.BrandWhite, color.BrandInk}
	// Only the hanging mark has a wire (lightened in dark mode) and a shadow.
	extras := map[string][]string{"flat": nil, "hanging": {"#b4bac4", "#000000"}}
	for name, svg := range map[string]string{"flat": exhibitLogoSVG, "hanging": exhibitHangingLogoSVG} {
		allowed := map[string]bool{}
		for _, c := range append(append([]string{}, pigments...), extras[name]...) {
			allowed[c] = true
		}
		seen := map[string]bool{}
		for _, h := range hex.FindAllString(svg, -1) {
			assert.True(t, allowed[h], "%s: unexpected colour %s", name, h)
			seen[h] = true
		}
		for _, want := range pigments {
			assert.True(t, seen[want], "%s: missing pigment %s", name, want)
		}
	}
}

func svgNumber(t *testing.T, re, svg string) float64 {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(svg)
	if !assert.NotNil(t, m, re) {
		t.FailNow()
	}
	f, err := strconv.ParseFloat(m[1], 64)
	require.NoError(t, err)
	return f
}

// Nail, wire apex and frame centre share one axis; the legs land symmetrically
// on the frame's outer top edge; the wire stroke matches the frame's; the nail
// shadow is offset exactly as the frame's is (one light direction).
func TestHangingLogoGeometry(t *testing.T) {
	svg := exhibitHangingLogoSVG
	const eps = 1e-3
	frame := regexp.MustCompile(`<rect style="fill:#f6efdc;stroke:#1f2a1c;stroke-width:([\d.]+)[^"]*" width="([\d.]+)" height="[\d.]+" x="([\d.]+)" y="([\d.]+)"`).FindStringSubmatch(svg)
	require.NotNil(t, frame)
	f := make([]float64, 4)
	for i := range f {
		f[i], _ = strconv.ParseFloat(frame[i+1], 64)
	}
	stroke, w, x, y := f[0], f[1], f[2], f[3]
	centre, outerTop := x+w/2, y-stroke/2

	wire := regexp.MustCompile(`class="logo-wire" d="M ([\d.]+),([\d.]+) ([\d.]+),([\d.]+) ([\d.]+),([\d.]+)"`).FindStringSubmatch(svg)
	require.NotNil(t, wire)
	v := make([]float64, 6)
	for i := range v {
		v[i], _ = strconv.ParseFloat(wire[i+1], 64)
	}
	nailX := svgNumber(t, `class="logo-nail" cx="([\d.]+)"`, svg)
	assert.InDelta(t, centre, nailX, eps, "nail on the frame's axis")
	assert.InDelta(t, centre, v[2], eps, "wire apex on the frame's axis")
	assert.InDelta(t, centre-v[0], v[4]-centre, eps, "legs symmetric about the axis")
	assert.InDelta(t, outerTop, v[1], eps, "left leg lands on the outer top edge")
	assert.InDelta(t, outerTop, v[5], eps, "right leg lands on the outer top edge")
	assert.InDelta(t, stroke, svgNumber(t, `class="logo-wire"[^>]*stroke-width="([\d.]+)"`, svg), eps, "wire stroke matches the frame's")

	// Shadows: the frame's copy is the first rect of logo-shadow, the nail's the circle.
	shadowRectX := svgNumber(t, `class="logo-shadow"[^>]*>\s*<rect[^>]*x="([\d.]+)"`, svg)
	shadowRectY := svgNumber(t, `class="logo-shadow"[^>]*>\s*<rect[^>]*y="([\d.]+)"`, svg)
	shadowNailX := svgNumber(t, `<circle cx="([\d.]+)"`, svg)
	shadowNailY := svgNumber(t, `<circle cx="[\d.]+" cy="([\d.]+)"`, svg)
	nailY := svgNumber(t, `class="logo-nail" cx="[\d.]+" cy="([\d.]+)"`, svg)
	assert.InDelta(t, shadowRectX-x, shadowNailX-nailX, eps, "one shadow direction (x)")
	assert.InDelta(t, shadowRectY-y, shadowNailY-nailY, eps, "one shadow direction (y)")
}
