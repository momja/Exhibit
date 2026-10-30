// Package kit is Exhibit's built-in component kit (Exh-61ix, experimental): a
// versioned set of design tokens and component styles that artifacts can share,
// so a library built over months reads as one interface rather than forty.
//
// An artifact opts in by pinning a version in its own source:
//
//	<meta name="exhibit-kit" content="v1">
//
// Inline replaces that marker with the kit's stylesheet, exactly as released,
// wherever the artifact leaves storage: the render surface and the export.
// The stored body therefore stays small and authorable, and the exported file
// is still one self-contained document — the kit is part of the file the
// moment it leaves the service, never a live dependency.
//
// A released version is immutable. Changing the kit means adding kits/v2,
// never editing kits/v1, because an artifact pinned a year ago must render
// exactly as it did then; a test hashes every released version to hold that.
// Moving an artifact to a newer kit is an edit to its marker, which makes it a
// new version of the artifact and therefore one restore away from undone.
//
// A meta element was chosen as the marker because nothing fetches it: the
// footprint scanner reports no origin for it, and an unknown version left in
// place does nothing at all in a browser.
//
// All kit rules sit in one cascade layer, `@layer exhibit-kit`. Unlayered
// styles always beat layered ones, so whatever the artifact writes overrides
// the kit regardless of selector specificity. That is the override rule the
// kit depends on, and it costs the artifact author nothing to use.
package kit

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

//go:embed kits
var kitsFS embed.FS

// MarkerName is the meta element's name attribute that pins a kit version.
const MarkerName = "exhibit-kit"

// Layer is the cascade layer every kit rule is wrapped in.
const Layer = "exhibit-kit"

// Kit is one released version.
type Kit struct {
	Version     string      `json:"version"`
	Description string      `json:"description"`
	Base        Base        `json:"base"`
	Tokens      []Token     `json:"tokens"`
	Components  []Component `json:"components"`

	stylesheet string
}

// Base describes the kit's element defaults and utility classes.
type Base struct {
	Description string   `json:"description"`
	Classes     []string `json:"classes"`
}

// Token documents one design token, or one family of them.
type Token struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Component is one named unit of the kit: its description, the classes it
// defines, an example of the markup it expects, and its stylesheet.
type Component struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Classes     []string `json:"classes"`
	Usage       string   `json:"usage"`
	CSS         string   `json:"css,omitempty"`
}

// Stylesheet is the version's complete CSS, in cascade order: tokens, base,
// then components in manifest order, all inside the kit layer.
func (k *Kit) Stylesheet() string { return k.stylesheet }

// Component returns the named component, with its CSS.
func (k *Kit) Component(name string) (Component, bool) {
	for _, c := range k.Components {
		if c.Name == name {
			return c, true
		}
	}
	return Component{}, false
}

// Marker is the markup that pins this version.
func (k *Kit) Marker() string {
	return fmt.Sprintf(`<meta name="%s" content="%s">`, MarkerName, k.Version)
}

var (
	released = map[string]*Kit{}
	versions []string
)

func init() {
	dirs, err := fs.ReadDir(kitsFS, "kits")
	if err != nil {
		panic("kit: read kits: " + err.Error())
	}
	for _, d := range dirs {
		k, err := load(d.Name())
		if err != nil {
			panic("kit: load " + d.Name() + ": " + err.Error())
		}
		released[k.Version] = k
		versions = append(versions, k.Version)
	}
	sort.Slice(versions, func(i, j int) bool { return versionNumber(versions[i]) < versionNumber(versions[j]) })
}

func versionNumber(v string) int {
	var n int
	fmt.Sscanf(v, "v%d", &n)
	return n
}

func load(dir string) (*Kit, error) {
	root := path.Join("kits", dir)
	raw, err := kitsFS.ReadFile(path.Join(root, "manifest.json"))
	if err != nil {
		return nil, err
	}
	k := &Kit{}
	if err := json.Unmarshal(raw, k); err != nil {
		return nil, err
	}
	if k.Version != dir {
		return nil, fmt.Errorf("manifest version %q does not match directory", k.Version)
	}

	var sheet strings.Builder
	fmt.Fprintf(&sheet, "/* Exhibit component kit %s */\n@layer %s{\n", k.Version, Layer)
	for _, name := range []string{"tokens.css", "base.css"} {
		css, err := kitsFS.ReadFile(path.Join(root, name))
		if err != nil {
			return nil, err
		}
		sheet.Write(css)
	}
	for i := range k.Components {
		c := &k.Components[i]
		css, err := kitsFS.ReadFile(path.Join(root, "components", c.Name+".css"))
		if err != nil {
			return nil, err
		}
		c.CSS = string(css)
		sheet.Write(css)
	}
	sheet.WriteString("}\n")
	k.stylesheet = sheet.String()
	return k, nil
}

// Versions lists every released version, oldest first.
func Versions() []string { return append([]string(nil), versions...) }

// Latest is the newest released version, the one new artifacts should pin.
func Latest() string { return versions[len(versions)-1] }

// Get returns a released version.
func Get(version string) (*Kit, bool) {
	k, ok := released[version]
	return k, ok
}

// Inline replaces each kit marker in body with that version's stylesheet.
//
// A version pinned twice is inlined once; the second marker is dropped. A
// marker naming no released version is left exactly as written — it does
// nothing in a browser, and silently substituting another version would
// restyle the artifact behind its author's back. A body with no marker comes
// back unchanged, which is every artifact that never opted in.
//
// The markers are found with the HTML tokenizer rather than a string search,
// so the text of a script, a style block or a comment that happens to spell
// the marker is never touched.
func Inline(body string) string {
	if !strings.Contains(body, MarkerName) {
		return body
	}
	var out strings.Builder
	z := html.NewTokenizer(strings.NewReader(body))
	done := map[string]bool{}
	offset, copied := 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		width := len(z.Raw())
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			if version, ok := markerVersion(z); ok {
				if k, released := Get(version); released {
					out.WriteString(body[copied:offset])
					if !done[version] {
						done[version] = true
						fmt.Fprintf(&out, `<style data-exhibit-kit="%s">%s</style>`, k.Version, k.Stylesheet())
					}
					copied = offset + width
				}
			}
		}
		offset += width
	}
	if copied == 0 {
		return body
	}
	out.WriteString(body[copied:])
	return out.String()
}

// Pinned reports which released versions body pins, in document order.
func Pinned(body string) []string {
	if !strings.Contains(body, MarkerName) {
		return nil
	}
	var pinned []string
	z := html.NewTokenizer(strings.NewReader(body))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return pinned
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			if v, ok := markerVersion(z); ok {
				pinned = append(pinned, v)
			}
		}
	}
}

// markerVersion reads the current tag as a kit marker. It must be called
// before anything else consumes the tokenizer's current token.
func markerVersion(z *html.Tokenizer) (string, bool) {
	name, hasAttr := z.TagName()
	if !bytes.Equal(name, []byte("meta")) || !hasAttr {
		return "", false
	}
	var isMarker bool
	var content string
	for {
		key, val, more := z.TagAttr()
		switch string(key) {
		case "name":
			// Exact, like the fast path in Inline: one spelling, so a cheap
			// substring test can rule a body out before tokenizing it.
			isMarker = string(val) == MarkerName
		case "content":
			content = strings.TrimSpace(string(val))
		}
		if !more {
			break
		}
	}
	return content, isMarker && content != ""
}
