package export

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/store"
)

// Where the manifest lands decides whether it runs at all: a fetch wrapper
// only shadows callers that come after it, and one spliced into a comment or a
// script's string literal never executes. The literal text "<head>" appears in
// documents that are not declaring a head — an artifact that builds an iframe
// document out of strings is the ordinary case — so the position comes from
// tokenizing rather than from the first match.
func TestHeadInsertionIgnoresHeadTextThatIsNotTheElement(t *testing.T) {
	cases := map[string]struct {
		body   string
		offset int
		before bool
	}{
		"plain head": {
			body:   `<html><head><title>t</title></head></html>`,
			offset: len(`<html><head>`),
		},
		"head with attributes": {
			body:   `<html><head lang="en"><title>t</title></head></html>`,
			offset: len(`<html><head lang="en">`),
		},
		"comment first": {
			body:   `<html><!-- <head> --><head><title>t</title></head></html>`,
			offset: len(`<html><!-- <head> --><head>`),
		},
		"script string first": {
			body:   `<html><script>var s = "<head>";</script><head></head></html>`,
			offset: len(`<html><script>var s = "<head>";</script><head>`),
		},
		"end tag only": {
			body:   `<html></head><body></body></html>`,
			offset: len(`<html>`),
			before: true,
		},
		"no head at all": {
			body:   `<div>just a fragment</div>`,
			offset: -1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			at, before := headInsertion(tc.body)
			assert.Equal(t, tc.offset, at)
			assert.Equal(t, tc.before, before)
		})
	}
}

// A runtime payload keeps the literal the page was ingested with, so it comes
// back through a manifest rather than by rewriting the body — and that manifest
// has to run before the page's own scripts.
func TestMaterializeInjectsTheManifestAheadOfThePagesScripts(t *testing.T) {
	body := `<html><!-- <head> --><head><script>fetch('https://cdn.test/app.wasm')</script></head></html>`

	out, err := Materialize(body, []store.ArtifactAsset{{
		BlobID: "b1", SourceURL: "https://cdn.test/app.wasm", ContentType: "application/wasm",
	}}, func(store.ArtifactAsset) string { return "https://render.test/a/x/assets/as-1" },
		func(string) ([]byte, error) { return []byte("\x00asm"), nil })
	require.NoError(t, err)

	manifest := strings.Index(out, "var M = ")
	page := strings.Index(out, "fetch('https://cdn.test/app.wasm')")
	require.Positive(t, manifest)
	assert.Less(t, manifest, page, "a fetch wrapper only shadows callers that run after it")
	assert.Greater(t, manifest, strings.Index(out, "<!-- <head> -->"),
		"the comment mentioning <head> is not the head")
	assert.Contains(t, out, "data:application/wasm;base64,")
}

// An artifact with no assets is the common case and must come back untouched.
func TestMaterializeLeavesAnAssetlessArtifactAlone(t *testing.T) {
	body := `<html><head></head><body>hi</body></html>`
	out, err := Materialize(body, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, body, out)
}
