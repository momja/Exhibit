package tile

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The default tile's hue must be stable per artifact — a card that changes
// face between visits is not recognizable.
func TestHueIsStablePerArtifact(t *testing.T) {
	assert.Equal(t, Hue("abc"), Hue("abc"))
	assert.NotEqual(t, Hue("abc"), Hue("abd"))
	assert.Less(t, Hue("abc"), 360)
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
