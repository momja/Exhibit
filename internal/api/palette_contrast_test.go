package api

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every text/surface token pair holds WCAG AA in both palettes (Exh-plvh). It
// reads web/gallery/tokens.css itself, so it needs no browser and fails the
// moment a value drifts; TestReadabilityBrowser covers the computed colors on
// the shipped pages. Large text and non-text pairs use the 3:1 threshold.

type tokenPair struct {
	ink, surface string
	min          float64
}

var paletteTextPairs = func() []tokenPair {
	var pairs []tokenPair
	for _, ink := range []string{"color-ink", "color-ink-soft", "color-ink-mid", "color-muted", "color-label", "color-faint", "color-placeholder", "color-code-muted"} {
		for _, surface := range []string{"color-panel", "color-ground", "color-subtle", "color-field", "color-hover-fill"} {
			pairs = append(pairs, tokenPair{ink, surface, 4.5})
		}
	}
	for _, ink := range []string{"color-slate-ink", "color-slate-body", "color-slate-medium", "color-slate-muted", "color-slate-placeholder"} {
		for _, surface := range []string{"color-panel", "color-ground"} {
			pairs = append(pairs, tokenPair{ink, surface, 4.5})
		}
	}
	for _, surface := range []string{"color-panel", "color-ground", "color-subtle", "color-hover-fill", "color-accent-tint", "color-warn-tint"} {
		pairs = append(pairs, tokenPair{"color-link", surface, 4.5}, tokenPair{"color-link-hover", surface, 4.5})
	}
	for _, surface := range []string{"color-panel", "color-ground", "color-subtle", "color-hover-fill", "color-accent-tint"} {
		pairs = append(pairs, tokenPair{"brand-blue", surface, 4.5})
	}
	pairs = append(pairs,
		tokenPair{"color-on-accent", "brand-blue", 4.5},
		tokenPair{"color-on-accent", "brand-blue-hover", 4.5},
		tokenPair{"color-accent-tint-ink", "color-accent-tint", 4.5},
		tokenPair{"color-accent-tint-ink", "color-accent-tint-strong", 4.5},
		tokenPair{"color-ink", "color-code-selection", 4.5},
		tokenPair{"color-danger", "color-panel", 4.5},
		tokenPair{"color-danger", "color-ground", 4.5},
		tokenPair{"color-on-danger", "color-danger", 4.5},
		tokenPair{"color-on-danger", "color-danger-hover", 4.5},
		tokenPair{"color-danger-ink", "color-danger-tint", 4.5},
		tokenPair{"color-danger-ink", "color-danger-soft", 4.5},
		tokenPair{"color-warn-ink", "color-warn-tint", 4.5},
		tokenPair{"color-warn-ink-soft", "color-warn-tint", 4.5},
		tokenPair{"color-warn-ink-soft", "color-panel", 4.5},
		tokenPair{"color-info-ink", "color-info-tint", 4.5},
		tokenPair{"color-success", "color-panel", 4.5},
		tokenPair{"color-success", "color-success-tint", 4.5},
		tokenPair{"color-note-ink", "color-note-tint", 4.5},
		// Non-text: warning icon and field boundaries need 3:1.
		tokenPair{"color-warn-strong", "color-panel", 3},
		tokenPair{"color-warn-strong", "color-warn-tint", 3},
		tokenPair{"color-field-border", "color-field", 3},
		tokenPair{"color-field-border", "color-panel", 3},
	)
	return pairs
}()

var tokenDecl = regexp.MustCompile(`(--[a-z0-9-]+):\s*([^;]+);`)

// parseTokens returns the light and dark value of every token. Dark inherits
// light for a token its single block does not redefine.
func parseTokens(t *testing.T) (light, dark map[string]string) {
	t.Helper()
	raw, err := os.ReadFile("../../web/gallery/tokens.css")
	require.NoError(t, err)
	css := string(raw)
	at := strings.Index(css, "@media (prefers-color-scheme:dark){")
	require.Positive(t, at, "tokens.css must keep its single dark block")
	collect := func(s string, into map[string]string) {
		for _, m := range tokenDecl.FindAllStringSubmatch(s, -1) {
			into[m[1]] = strings.TrimSpace(m[2])
		}
	}
	light, dark = map[string]string{}, map[string]string{}
	collect(css[:at], light)
	for k, v := range light {
		dark[k] = v
	}
	collect(css[at:], dark)
	return light, dark
}

func resolveToken(t *testing.T, tokens map[string]string, name string) [3]float64 {
	t.Helper()
	v, ok := tokens["--"+name]
	require.True(t, ok, "token --%s is not defined", name)
	if ref := regexp.MustCompile(`^var\(--([a-z0-9-]+)\)$`).FindStringSubmatch(v); ref != nil {
		return resolveToken(t, tokens, ref[1])
	}
	hex := strings.TrimPrefix(v, "#")
	require.Regexp(t, `^[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$`, hex, "--%s: %q is not an opaque hex color", name, v)
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	var rgb [3]float64
	for i := range rgb {
		n, err := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
		require.NoError(t, err)
		c := float64(n) / 255
		if c <= .04045 {
			rgb[i] = c / 12.92
		} else {
			rgb[i] = math.Pow((c+.055)/1.055, 2.4)
		}
	}
	return rgb
}

func luminance(rgb [3]float64) float64 { return .2126*rgb[0] + .7152*rgb[1] + .0722*rgb[2] }

func contrastRatio(a, b [3]float64) float64 {
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + .05) / (math.Min(la, lb) + .05)
}

func TestPaletteContrastAA(t *testing.T) {
	light, dark := parseTokens(t)
	for mode, tokens := range map[string]map[string]string{"light": light, "dark": dark} {
		for _, p := range paletteTextPairs {
			ratio := contrastRatio(resolveToken(t, tokens, p.ink), resolveToken(t, tokens, p.surface))
			t.Log(fmt.Sprintf("%s %s on %s %.2f", mode, p.ink, p.surface, ratio))
			if ratio < p.min {
				t.Errorf("%s: %s on %s is %.2f:1, needs %.1f:1", mode, p.ink, p.surface, ratio, p.min)
			}
		}
	}
}
