// Package tile is the default face of an artifact that has no widget
// (av-fafu): a monogram on a tint derived from the artifact's id.
//
// Two surfaces draw it and they must agree. The gallery card renders it as
// markup inside the app page, and a shared widget link (av-cp7j) serves it as
// a document of its own on the render origin, where no gallery stylesheet
// exists. Both take the letters and the hue from here, so an embed shows the
// same face the owner sees on their shelf.
package tile

import (
	"bytes"
	"hash/fnv"
	"html/template"
	"unicode"
)

// Monogram reduces a title to the one or two letters the default tile shows.
// It walks runes rather than bytes so a non-ASCII title yields a real letter
// instead of half a UTF-8 sequence, and falls back to a dash for a title with
// no letters at all (an untitled artifact, an emoji-only name).
func Monogram(title string) string {
	var letters []rune
	takeNext := true
	for _, r := range title {
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			takeNext = true
			continue
		}
		if takeNext && unicode.IsLetter(r) {
			letters = append(letters, unicode.ToUpper(r))
			takeNext = false
			if len(letters) == 2 {
				break
			}
		}
	}
	if len(letters) == 0 {
		return "—"
	}
	return string(letters)
}

// Palette is the closed set of hues a default tile may wear (Exh-z8i2): the
// logo's red, yellow and blue pigments, two greens, and the dusk indigo of the
// dark paintings. Saturation and lightness come from the stylesheet's tile
// tokens, so every entry is held to the same text contrast in both schemes.
// Adding a hue means checking it in TestTilePaletteKeepsTextContrast.
var Palette = [...]int{8, 40, 96, 136, 214, 231}

// Hue picks a stable palette hue for an artifact id, so a tile keeps its face
// between visits. FNV-1a because the requirement is "spread ids across the
// palette", not secrecy.
func Hue(artifactID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(artifactID))
	return Palette[int(h.Sum32()%uint32(len(Palette)))]
}

// documentTmpl is the standalone default tile. It carries no script at all,
// so the only thing its CSP has to permit is the inline style and the grain's
// data: image. The colors mirror .card-widget-default, .grain and
// .card-widget-monogram in web/gallery/components.css and the tile tokens in
// tokens.css; change them together.
var documentTmpl = template.Must(template.New("tile").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Title}}</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{color-scheme:light dark;--a:62% 88%;--b:58% 78%;--fill:60% 97%;--ink:55% 26%;--shadow:40% 30% / .18;--grain:.55}
@media (prefers-color-scheme:dark){:root{--a:38% 30%;--b:36% 20%;--fill:34% 34%;--ink:60% 88%;--shadow:40% 8% / .5;--grain:.4}}
html,body{margin:0;height:100%}
body{position:relative;isolation:isolate;display:flex;align-items:center;justify-content:center;font-family:system-ui,-apple-system,sans-serif;background:linear-gradient(150deg,hsl({{.Hue}} var(--a)),hsl({{.Hue}} var(--b)))}
body::before{content:"";position:absolute;inset:0;z-index:0;pointer-events:none;background:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' stitchTiles='stitch'/%3E%3CfeColorMatrix values='.6 .6 .6 0 -.4 .6 .6 .6 0 -.4 .6 .6 .6 0 -.4 0 0 0 0 1'/%3E%3C/filter%3E%3Crect width='160' height='160' filter='url(%23n)'/%3E%3C/svg%3E");mix-blend-mode:overlay;opacity:var(--grain)}
span{position:relative;z-index:1;display:flex;align-items:center;justify-content:center;width:56px;height:56px;border-radius:14px;background:hsl({{.Hue}} var(--fill));box-shadow:0 1px 3px hsl({{.Hue}} var(--shadow));color:hsl({{.Hue}} var(--ink));font-size:22px;font-weight:600;letter-spacing:.5px}
@media (forced-colors:active){body{background:Canvas}body::before{display:none}span{border:1px solid CanvasText}}
</style></head>
<body><span aria-label="{{.Title}}">{{.Monogram}}</span></body></html>
`))

// Document renders the default tile as a complete HTML document for the
// render origin. The title is artifact-chosen text and is escaped by
// html/template like any other.
func Document(artifactID, title string) []byte {
	var buf bytes.Buffer
	// The template is fixed and its inputs are a string and an int, so
	// Execute cannot fail short of a bug caught by the package tests.
	_ = documentTmpl.Execute(&buf, struct {
		Title, Monogram string
		Hue             int
	}{title, Monogram(title), Hue(artifactID)})
	return buf.Bytes()
}
