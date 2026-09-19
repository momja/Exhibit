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

// Hue derives a stable 0–359 hue from an artifact id, so every tile gets a
// distinct-looking but unchanging tint. FNV-1a because the requirement is
// "spread ids across the wheel", not secrecy.
func Hue(artifactID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(artifactID))
	return int(h.Sum32() % 360)
}

// documentTmpl is the standalone default tile. It carries no script at all,
// so the only thing its CSP has to permit is the inline style. The colors
// mirror .card-widget-default and .card-widget-monogram in
// web/gallery/components.css; change them together.
var documentTmpl = template.Must(template.New("tile").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>{{.Title}}</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
html,body{margin:0;height:100%}
body{display:flex;align-items:center;justify-content:center;font-family:system-ui,-apple-system,sans-serif;background:linear-gradient(140deg,hsl({{.Hue}} 58% 96%),hsl(calc({{.Hue}} + 28) 52% 89%))}
span{display:flex;align-items:center;justify-content:center;width:56px;height:56px;border-radius:16px;background:hsl({{.Hue}} 60% 100% / .72);box-shadow:0 1px 4px hsl({{.Hue}} 40% 30% / .16);color:hsl({{.Hue}} 42% 34%);font-size:22px;font-weight:600;letter-spacing:.5px}
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
