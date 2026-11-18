package tile

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The default tile's hue must be stable per artifact — a card that changes
// face between visits is not recognizable.
func TestHueIsStablePerArtifact(t *testing.T) {
	assert.Equal(t, Hue("abc"), Hue("abc"))
}

// A tile may only wear a palette hue, and over UUID-shaped ids each hue gets
// close to its fair share (100 of 600).
func TestHueIsAlwaysAPaletteHue(t *testing.T) {
	seen := map[int]int{}
	for i := 0; i < 600; i++ {
		seen[Hue(uuid.NewSHA1(uuid.NameSpaceOID, []byte(strconv.Itoa(i))).String())]++
	}
	for hue := range seen {
		assert.Contains(t, Palette, hue)
	}
	for _, hue := range Palette {
		assert.InDelta(t, 100, seen[hue], 35, "hue %d should be spread across ids", hue)
	}
}

// The standalone share tile cannot load tokens.css, so it carries its own
// copy. Every value must equal the stylesheet's, or the two faces drift.
func TestDocumentMirrorsTileTokens(t *testing.T) {
	raw, err := os.ReadFile("../../web/gallery/tokens.css")
	require.NoError(t, err)
	css := string(raw)
	dark := strings.Index(css, "@media (prefers-color-scheme:dark){")
	require.Positive(t, dark)
	decl := func(block, name string) string {
		m := regexp.MustCompile(`(?:^|[\s{;])` + regexp.QuoteMeta(name) + `:\s*([^;}]+?)\s*[;}]`).FindStringSubmatch(block)
		require.NotNil(t, m, "%s missing", name)
		return m[1]
	}
	rootAt := strings.Index(documentSource, ":root{")
	darkAt := strings.Index(documentSource, "@media (prefers-color-scheme:dark){:root{")
	require.Greater(t, darkAt, rootAt)
	docLight, docDark := documentSource[rootAt:darkAt], documentSource[darkAt:]
	pairs := [][2]string{{"--a", "--tile-face-top"}, {"--b", "--tile-face-bottom"}, {"--fill", "--tile-mono-fill"}, {"--ink", "--tile-mono-ink"}, {"--shadow", "--tile-mono-shadow"}, {"--grain-opacity", "--grain-opacity"}}
	for _, p := range pairs {
		assert.Equal(t, decl(css[:dark], p[1]), decl(docLight, p[0]), "light %s", p[0])
		assert.Equal(t, decl(css[dark:], p[1]), decl(docDark, p[0]), "dark %s", p[0])
	}
	// The grain is the same image, byte for byte.
	url := regexp.MustCompile(`--grain-url:url\("([^"]+)"\)`).FindStringSubmatch(css)
	require.NotNil(t, url)
	assert.Contains(t, documentSource, `url("`+url[1]+`")`)
}

func TestMonogram(t *testing.T) {
	cases := map[string]string{
		"Run Log":              "RL",
		"Mortgage Calculator":  "MC",
		"reading-list":         "RL",
		"Budget":               "B",
		"":                     "—",
		"🙂":                    "—",
		"Über Tracker Deluxe":  "ÜT",
		"  leading whitespace": "LW",
	}
	for title, want := range cases {
		assert.Equal(t, want, Monogram(title), "Monogram(%q)", title)
	}
}

func TestDocumentCarriesTheCardsFaceAndNoScript(t *testing.T) {
	doc := string(Document("abc", "Run Log"))
	assert.Contains(t, doc, ">RL</span>")
	assert.Contains(t, doc, "hsl("+strconv.Itoa(Hue("abc"))+" ")
	assert.NotContains(t, strings.ToLower(doc), "<script")
}

// The title is artifact-chosen text; it must not become markup.
func TestDocumentEscapesTheTitle(t *testing.T) {
	doc := string(Document("abc", `</title><script>alert(1)</script>`))
	assert.NotContains(t, doc, "<script>")
}
