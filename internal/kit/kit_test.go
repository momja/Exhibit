package kit

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releasedHashes pins the bytes of every released kit and contract. Both are
// immutable: an artifact that pinned a kit must render exactly as it did the
// day it was saved. If this test fails because you edited a released one, put
// the change in a new version directory instead. Add the new version's hash
// here when you release it.
var releasedHashes = map[string]string{
	"contracts/v1.json": "50e49bc06e4245b821476ec9937a647bb8ae9d8dcd63191b5b40456c00b89569",
	"kits/exhibit/v1":   "9e644a38be2da7b3163ce2fd75cb6d9f2859566318d2518875d6446b62401bf4",
	"kits/paper/v1":     "106b6730b154c183a94ab5e476606369c1513950d87a64bef5e9c867c26dfe7e",
	"kits/brutal/v1":    "82545aa5b174f859a9b7f1dbc7e83c6c77828c7bb3102ec382009a3d261bb881",
}

func TestReleasedKitsAreImmutable(t *testing.T) {
	released := []string{}
	for _, id := range IDs() {
		released = append(released, "kits/"+id)
	}
	for v := range contracts {
		released = append(released, "contracts/"+v+".json")
	}
	for _, name := range released {
		want, ok := releasedHashes[name]
		require.Truef(t, ok, "%s has no recorded hash; record %s once it is released", name, hashFiles(t, name))
		assert.Equalf(t, want, hashFiles(t, name), "%s changed after release; add a new version instead", name)
	}
}

// hashFiles hashes one file, or every file under a directory in a stable order.
func hashFiles(t *testing.T, root string) string {
	t.Helper()
	var files []string
	require.NoError(t, fs.WalkDir(kitsFS, root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	}))
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := kitsFS.ReadFile(f)
		require.NoError(t, err)
		fmt.Fprintf(h, "%s\x00%d\x00", strings.TrimPrefix(f, root+"/"), len(b))
		h.Write(b)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TestEveryKitImplementsItsContract is what makes kits interchangeable: markup
// written against a contract must find every class it uses, and every token
// it styles its own rules with, in whichever kit on that contract it pins.
func TestEveryKitImplementsItsContract(t *testing.T) {
	classPattern := regexp.MustCompile(`\.([a-z][a-z0-9_-]*)`)
	tokenDefinition := regexp.MustCompile(`(--kit-[a-z0-9-]+)\s*:`)
	tokenUse := regexp.MustCompile(`var\((--kit-[a-z0-9-]+)`)
	tokensByContract := map[string][]string{}

	for _, id := range IDs() {
		k, ok := Get(id)
		require.True(t, ok)
		c := k.Contract

		// One stylesheet per contract component, and nothing undocumented.
		entries, err := fs.ReadDir(kitsFS, path.Join("kits", id, "components"))
		require.NoError(t, err)
		for _, e := range entries {
			_, listed := k.Component(strings.TrimSuffix(e.Name(), ".css"))
			assert.Truef(t, listed, "%s: components/%s is not in contract %s", id, e.Name(), c.Version)
		}

		sheet := k.Stylesheet()
		// Inlined into a <style> element, so it must not be able to close one.
		assert.NotContains(t, strings.ToLower(sheet), "</style", id)
		assert.True(t, strings.HasPrefix(sheet, "/* Exhibit component kit "+id), id)

		// Every documented class exists, so neither the agent nor an artifact
		// that swaps kits is relying on one that does nothing.
		documented := append([]string(nil), c.Base.Classes...)
		for _, comp := range c.Components {
			assert.NotEmptyf(t, comp.Description, "%s/%s", c.Version, comp.Name)
			assert.NotEmptyf(t, comp.Usage, "%s/%s", c.Version, comp.Name)
			documented = append(documented, comp.Classes...)
		}
		for _, sel := range documented {
			for _, m := range classPattern.FindAllStringSubmatch(sel, -1) {
				assert.Containsf(t, sheet, "."+m[1], "%s documents %s but no rule in %s defines it", c.Version, sel, id)
			}
		}

		// Every token the kit reads, it defines.
		defined := map[string]bool{}
		for _, m := range tokenDefinition.FindAllStringSubmatch(sheet, -1) {
			defined[m[1]] = true
		}
		for _, m := range tokenUse.FindAllStringSubmatch(sheet, -1) {
			assert.Truef(t, defined[m[1]], "%s reads %s but never defines it", id, m[1])
		}

		// Kits on one contract define the same tokens, so an artifact's own
		// rules written with them survive a swap.
		names := make([]string, 0, len(defined))
		for name := range defined {
			names = append(names, name)
		}
		sort.Strings(names)
		if first, seen := tokensByContract[c.Version]; seen {
			assert.Equalf(t, first, names, "%s defines different tokens from the other kits on contract %s", id, c.Version)
		} else {
			tokensByContract[c.Version] = names
		}
	}
}

func TestDefaultIsTheNewestExhibitKit(t *testing.T) {
	assert.Equal(t, "exhibit/v1", Default())
	assert.Equal(t, "", Latest("no-such-kit"))
	assert.Contains(t, IDs(), "paper/v1")
}

func TestInlineReplacesAPinnedMarker(t *testing.T) {
	k, _ := Get("exhibit/v1")
	body := `<!doctype html><html><head><meta name="exhibit-kit" content="exhibit/v1"><title>t</title></head><body></body></html>`

	out := Inline(body)

	assert.NotContains(t, out, `<meta name="exhibit-kit"`)
	assert.Contains(t, out, `<head><style data-exhibit-kit="exhibit/v1">`+k.Stylesheet()+`</style><title>t</title>`)
}

func TestInlineLeavesUnmarkedBodiesByteIdentical(t *testing.T) {
	for _, body := range []string{
		"",
		`<p>hello</p>`,
		`<html><head><meta name="viewport" content="width=device-width"></head></html>`,
	} {
		assert.Equal(t, body, Inline(body))
	}
}

func TestInlineLeavesAnUnknownVersionAlone(t *testing.T) {
	body := `<head><meta name="exhibit-kit" content="exhibit/v999"></head>`
	assert.Equal(t, body, Inline(body))
}

func TestInlineIncludesAVersionOnce(t *testing.T) {
	body := `<head><meta name="exhibit-kit" content="exhibit/v1"><meta name=exhibit-kit content=exhibit/v1 /></head>`
	out := Inline(body)
	assert.Equal(t, 1, strings.Count(out, `<style data-exhibit-kit="exhibit/v1">`))
	assert.NotContains(t, out, "<meta")
}

func TestInlineIgnoresMarkersInScriptsAndComments(t *testing.T) {
	body := `<head><!-- <meta name="exhibit-kit" content="exhibit/v1"> --><script>const m = '<meta name="exhibit-kit" content="exhibit/v1">';</script></head>`
	assert.Equal(t, body, Inline(body))
}

func TestInlineAcceptsAttributeOrderAndCase(t *testing.T) {
	body := `<HEAD><META CONTENT=" exhibit/v1 " NAME="exhibit-kit"></HEAD>`
	out := Inline(body)
	assert.Contains(t, out, `<style data-exhibit-kit="exhibit/v1">`)
	assert.NotContains(t, out, "META")
}

func TestPinned(t *testing.T) {
	assert.Nil(t, Pinned(`<p>no kit</p>`))
	assert.Equal(t, []string{"exhibit/v1", "exhibit/v999"}, Pinned(`<meta name="exhibit-kit" content="exhibit/v1"><meta name="exhibit-kit" content="exhibit/v999">`))
}

func TestMarkerRoundTrips(t *testing.T) {
	for _, id := range IDs() {
		k, _ := Get(id)
		assert.Equal(t, []string{id}, Pinned(k.Marker()))
	}
}

func TestInlineIncludesEachPinnedKitInOrder(t *testing.T) {
	out := Inline(`<head><meta name="exhibit-kit" content="paper/v1"><meta name="exhibit-kit" content="brutal/v1"></head>`)
	paper := strings.Index(out, `<style data-exhibit-kit="paper/v1">`)
	brutal := strings.Index(out, `<style data-exhibit-kit="brutal/v1">`)
	assert.True(t, paper >= 0 && brutal > paper, "both inlined, in document order")
}
