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

// releasedHashes pins the bytes of every released kit version. A released
// version is immutable: an artifact that pinned it must render exactly as it
// did the day it was saved. If this test fails because you edited a released
// kit, put the change in a new version directory instead. Add the new
// version's hash here when you release it.
var releasedHashes = map[string]string{
	"v1": "4f1cf131851c5ccb12b2555471e79e63bea5b2e0b92c196d8169994dc671ae39",
}

func TestReleasedKitsAreImmutable(t *testing.T) {
	for _, v := range Versions() {
		want, ok := releasedHashes[v]
		require.Truef(t, ok, "kit %s has no recorded hash; record %s once it is released", v, hashDir(t, v))
		assert.Equalf(t, want, hashDir(t, v), "kit %s changed after release; add a new version instead", v)
	}
}

func hashDir(t *testing.T, version string) string {
	t.Helper()
	root := path.Join("kits", version)
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

func TestEveryKitIsConsistentWithItsManifest(t *testing.T) {
	classPattern := regexp.MustCompile(`\.([a-z][a-z0-9_-]*)`)
	for _, v := range Versions() {
		k, ok := Get(v)
		require.True(t, ok)

		// Every stylesheet on disk is listed, so nothing ships undocumented.
		entries, err := fs.ReadDir(kitsFS, path.Join("kits", v, "components"))
		require.NoError(t, err)
		for _, e := range entries {
			_, listed := k.Component(strings.TrimSuffix(e.Name(), ".css"))
			assert.Truef(t, listed, "%s: components/%s is not in the manifest", v, e.Name())
		}

		sheet := k.Stylesheet()
		// Inlined into a <style> element, so it must not be able to close one.
		assert.NotContains(t, strings.ToLower(sheet), "</style", v)
		assert.True(t, strings.HasPrefix(sheet, "/* Exhibit component kit "+v), v)

		// Every documented class exists, so the agent is never told about
		// one that does nothing.
		documented := append([]string(nil), k.Base.Classes...)
		for _, c := range k.Components {
			assert.NotEmptyf(t, c.Description, "%s/%s", v, c.Name)
			assert.NotEmptyf(t, c.Usage, "%s/%s", v, c.Name)
			documented = append(documented, c.Classes...)
		}
		for _, sel := range documented {
			for _, m := range classPattern.FindAllStringSubmatch(sel, -1) {
				assert.Containsf(t, sheet, "."+m[1], "%s documents %s but no rule defines it", v, sel)
			}
		}
	}
}

func TestLatestIsTheNewestVersion(t *testing.T) {
	vs := Versions()
	require.NotEmpty(t, vs)
	assert.Equal(t, vs[len(vs)-1], Latest())
}

func TestInlineReplacesAPinnedMarker(t *testing.T) {
	k, _ := Get("v1")
	body := `<!doctype html><html><head><meta name="exhibit-kit" content="v1"><title>t</title></head><body></body></html>`

	out := Inline(body)

	assert.NotContains(t, out, `<meta name="exhibit-kit"`)
	assert.Contains(t, out, `<head><style data-exhibit-kit="v1">`+k.Stylesheet()+`</style><title>t</title>`)
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
	body := `<head><meta name="exhibit-kit" content="v999"></head>`
	assert.Equal(t, body, Inline(body))
}

func TestInlineIncludesAVersionOnce(t *testing.T) {
	body := `<head><meta name="exhibit-kit" content="v1"><meta name=exhibit-kit content=v1 /></head>`
	out := Inline(body)
	assert.Equal(t, 1, strings.Count(out, `<style data-exhibit-kit="v1">`))
	assert.NotContains(t, out, "<meta")
}

func TestInlineIgnoresMarkersInScriptsAndComments(t *testing.T) {
	body := `<head><!-- <meta name="exhibit-kit" content="v1"> --><script>const m = '<meta name="exhibit-kit" content="v1">';</script></head>`
	assert.Equal(t, body, Inline(body))
}

func TestInlineAcceptsAttributeOrderAndCase(t *testing.T) {
	body := `<HEAD><META CONTENT=" v1 " NAME="exhibit-kit"></HEAD>`
	out := Inline(body)
	assert.Contains(t, out, `<style data-exhibit-kit="v1">`)
	assert.NotContains(t, out, "META")
}

func TestPinned(t *testing.T) {
	assert.Nil(t, Pinned(`<p>no kit</p>`))
	assert.Equal(t, []string{"v1", "v999"}, Pinned(`<meta name="exhibit-kit" content="v1"><meta name="exhibit-kit" content="v999">`))
}

func TestMarkerRoundTrips(t *testing.T) {
	k, _ := Get(Latest())
	assert.Equal(t, []string{k.Version}, Pinned(k.Marker()))
}
