// Package kit is Exhibit's built-in component kits (Exh-61ix, experimental):
// named, versioned sets of design tokens and component styles that artifacts
// share, so a library built over months reads as one interface rather than
// forty.
//
// An artifact opts in by naming, in its own source, the kit contract its
// markup is written against:
//
//	<meta name="exhibit-kit" content="v1">
//
// The contract (contracts/v1.json) is what the artifact writes: components,
// their classes and markup, and token names. Which kit draws that markup is
// not in the source. It is chosen outside it — by the artifact's own setting,
// else its owner's library setting, else the default kit — and resolved on
// every request (Resolve), so changing it restyles an artifact without
// touching its source or its history. Every kit implementing a contract takes
// identical markup, which is what makes that swap safe.
//
// Inline replaces the marker with the resolved kit's stylesheet wherever the
// artifact leaves storage: the render surface and the export. The stored body
// stays small and authorable, and the exported file is still one
// self-contained document, carrying whichever kit applied when it was made.
//
// Released kits and contracts are immutable. Changing one means adding a new
// version directory; a test hashes every released file to hold that.
//
// A meta element was chosen as the marker because nothing fetches it: the
// footprint scanner reports no origin for it, and an unknown contract left in
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

//go:embed kits contracts
var kitsFS embed.FS

// MarkerName is the meta element's name attribute that pins a kit.
const MarkerName = "exhibit-kit"

// Layer is the cascade layer every kit rule is wrapped in.
const Layer = "exhibit-kit"

// DefaultName names the kit new artifacts use unless the user picks another.
const DefaultName = "exhibit"

// Contract is the markup an artifact writes: the components, their classes
// and example markup, and the tokens it may style its own rules with. Kits
// implementing the same contract differ only in how that markup looks, so
// moving an artifact between them is a one-line change to its marker.
// Contracts, like kits, never change once released.
type Contract struct {
	Version     string      `json:"contract"`
	Description string      `json:"description"`
	Base        Base        `json:"base"`
	Tokens      []Token     `json:"tokens"`
	Components  []Component `json:"components"`
}

// Kit is one released version of one named kit: a stylesheet implementing a
// contract.
type Kit struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Contract    *Contract `json:"-"`

	stylesheet   string
	componentCSS map[string]string
}

// Base describes the contract's element defaults and utility classes.
type Base struct {
	Description string   `json:"description"`
	Classes     []string `json:"classes"`
}

// Token documents one design token, or one family of them.
type Token struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Component is one named unit of a contract: its description, the classes it
// defines, an example of the markup it expects and, when read from a kit,
// that kit's stylesheet for it.
type Component struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Classes     []string `json:"classes"`
	Usage       string   `json:"usage"`
	CSS         string   `json:"css,omitempty"`
}

// ID is what a marker names: "exhibit/v1".
func (k *Kit) ID() string { return k.Name + "/" + k.Version }

// Stylesheet is the kit's complete CSS, in cascade order: tokens, base, then
// components in contract order, all inside the kit layer.
func (k *Kit) Stylesheet() string { return k.stylesheet }

// Component returns the named component with this kit's CSS for it.
func (k *Kit) Component(name string) (Component, bool) {
	for _, c := range k.Contract.Components {
		if c.Name == name {
			c.CSS = k.componentCSS[name]
			return c, true
		}
	}
	return Component{}, false
}

// Marker is the markup an artifact writes to use this contract.
func (c *Contract) Marker() string {
	return fmt.Sprintf(`<meta name="%s" content="%s">`, MarkerName, c.Version)
}

var (
	contracts = map[string]*Contract{}
	released  = map[string]*Kit{}
	ids       []string
)

func init() {
	entries, err := fs.ReadDir(kitsFS, "contracts")
	if err != nil {
		panic("kit: read contracts: " + err.Error())
	}
	for _, e := range entries {
		c := &Contract{}
		if err := readJSON(path.Join("contracts", e.Name()), c); err != nil {
			panic("kit: load contract " + e.Name() + ": " + err.Error())
		}
		if c.Version+".json" != e.Name() {
			panic("kit: contract " + e.Name() + " declares version " + c.Version)
		}
		contracts[c.Version] = c
	}

	names, err := fs.ReadDir(kitsFS, "kits")
	if err != nil {
		panic("kit: read kits: " + err.Error())
	}
	for _, n := range names {
		versions, err := fs.ReadDir(kitsFS, path.Join("kits", n.Name()))
		if err != nil {
			panic("kit: read kits/" + n.Name() + ": " + err.Error())
		}
		for _, v := range versions {
			k, err := load(n.Name(), v.Name())
			if err != nil {
				panic("kit: load " + n.Name() + "/" + v.Name() + ": " + err.Error())
			}
			released[k.ID()] = k
			ids = append(ids, k.ID())
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := released[ids[i]], released[ids[j]]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return versionNumber(a.Version) < versionNumber(b.Version)
	})
	if Default() == "" {
		panic("kit: no " + DefaultName + " kit")
	}
}

func versionNumber(v string) int {
	var n int
	fmt.Sscanf(v, "v%d", &n)
	return n
}

func readJSON(name string, v any) error {
	raw, err := kitsFS.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func load(name, version string) (*Kit, error) {
	root := path.Join("kits", name, version)
	var manifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Contract    string `json:"contract"`
		Description string `json:"description"`
	}
	if err := readJSON(path.Join(root, "manifest.json"), &manifest); err != nil {
		return nil, err
	}
	if manifest.Name != name || manifest.Version != version {
		return nil, fmt.Errorf("manifest names %s/%s", manifest.Name, manifest.Version)
	}
	contract, ok := contracts[manifest.Contract]
	if !ok {
		return nil, fmt.Errorf("no contract %q", manifest.Contract)
	}
	k := &Kit{
		Name:         name,
		Version:      version,
		Description:  manifest.Description,
		Contract:     contract,
		componentCSS: map[string]string{},
	}

	var sheet strings.Builder
	fmt.Fprintf(&sheet, "/* Exhibit component kit %s */\n@layer %s{\n", k.ID(), Layer)
	for _, file := range []string{"tokens.css", "base.css"} {
		css, err := kitsFS.ReadFile(path.Join(root, file))
		if err != nil {
			return nil, err
		}
		sheet.Write(css)
	}
	for _, c := range contract.Components {
		css, err := kitsFS.ReadFile(path.Join(root, "components", c.Name+".css"))
		if err != nil {
			return nil, err
		}
		k.componentCSS[c.Name] = string(css)
		sheet.Write(css)
	}
	sheet.WriteString("}\n")
	k.stylesheet = sheet.String()
	return k, nil
}

// IDs lists every released kit, grouped by name, oldest version first.
func IDs() []string { return append([]string(nil), ids...) }

// Default is the newest version of the default kit, the one new artifacts
// pin unless the user asks for another.
func Default() string { return Latest(DefaultName) }

// Latest is the newest released version of the named kit, or "" if there is
// no kit by that name.
func Latest(name string) string {
	latest := ""
	for _, id := range ids {
		if released[id].Name == name {
			latest = id
		}
	}
	return latest
}

// Get returns a released kit by ID.
func Get(id string) (*Kit, bool) {
	k, ok := released[id]
	return k, ok
}

// Choice is where an artifact's kit is chosen, nearest first: the artifact's
// own setting, then its owner's library setting. Each is a kit ID or "".
type Choice struct {
	Artifact string
	Library  string
}

// Resolve picks the kit that styles markup written against contract. It
// takes the first choice that names a kit on that contract — or, when a
// choice names a kit on another contract, the newest version of the same kit
// that is on it — then falls back to the default kit, and returns nil only for
// a contract nothing implements.
func Resolve(contract string, choice Choice) *Kit {
	for _, id := range []string{choice.Artifact, choice.Library, Default()} {
		if id == "" {
			continue
		}
		name := id
		if k, ok := released[id]; ok {
			if k.Contract.Version == contract {
				return k
			}
			name = k.Name
		}
		if k := newestOn(name, contract); k != nil {
			return k
		}
	}
	for _, id := range ids {
		if released[id].Contract.Version == contract {
			return released[id]
		}
	}
	return nil
}

func newestOn(name, contract string) *Kit {
	var newest *Kit
	for _, id := range ids {
		if k := released[id]; k.Name == name && k.Contract.Version == contract {
			newest = k
		}
	}
	return newest
}

// GetContract returns a released contract by version.
func GetContract(version string) (*Contract, bool) {
	c, ok := contracts[version]
	return c, ok
}

// Contracts lists every released contract version.
func Contracts() []string {
	versions := make([]string, 0, len(contracts))
	for v := range contracts {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versionNumber(versions[i]) < versionNumber(versions[j]) })
	return versions
}

// Inline replaces each kit marker in body with the stylesheet of the kit
// choice resolves for that marker's contract.
//
// A contract named twice is inlined once; the second marker is dropped. A
// marker naming no released contract is left exactly as written — it does
// nothing in a browser. A body with no marker comes back unchanged, which is
// every artifact that never opted in.
//
// The markers are found with the HTML tokenizer rather than a string search,
// so the text of a script, a style block or a comment that happens to spell
// the marker is never touched.
func Inline(body string, choice Choice) string {
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
			if contract, ok := markerContract(z); ok {
				if k := Resolve(contract, choice); k != nil {
					out.WriteString(body[copied:offset])
					if !done[contract] {
						done[contract] = true
						fmt.Fprintf(&out, `<style data-exhibit-kit="%s">%s</style>`, k.ID(), k.Stylesheet())
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

// Pinned reports which contracts body's markers name, released or not, in
// document order.
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
			if contract, ok := markerContract(z); ok {
				pinned = append(pinned, contract)
			}
		}
	}
}

// markerContract reads the current tag as a kit marker. It must be called
// before anything else consumes the tokenizer's current token.
func markerContract(z *html.Tokenizer) (string, bool) {
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
