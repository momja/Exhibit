package api

// The render preamble and the snapshot vendorer BOTH wrap window.fetch, and each
// wrapper captures whatever window.fetch is at its own install time — so the
// order they install in is part of the contract between them.
//
//   - agaf-02xs (the preamble) answers data: URLs from locally constructed
//     Responses, because WebKit refuses large data: fetches from an
//     opaque-origin sandbox.
//   - av-ghvs (the vendorer) inlines a runtime-fetched asset and injects a
//     manifest + fetch wrapper at the top of the artifact's <head>.
//
// The vendorer's wrapper decodes its own manifest entries rather than re-issuing
// fetch() against the data: URI, so it does not *depend* on the preamble — that
// is deliberate, and `TestInlineRuntimeAssetsDecodesLocallyNotViaFetch` pins it.
// What still depends on order is every other data: fetch in the frame: one the
// artifact's own code performs, or one a future wrapper delegates. Those reach
// the preamble's shim only if it installed first.
//
// injectPreamble inserts the preamble immediately after <head>, ahead of
// anything the artifact body carries. Move it later and the preamble stops
// shadowing native fetch for scripts that ran before it — Safari regresses on
// exactly the payloads that motivated the shim, with nothing visible in
// Chromium and no other test failing. Pin the order here.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreambleFetchWrapperPrecedesArtifactScripts(t *testing.T) {
	r := newTestRouter(t)

	// Stands in for a vendored artifact: the shape InlineRuntimeAssets emits,
	// a manifest plus a fetch wrapper at the top of <head>.
	body := `<html><head>
<script>
(function () {
  var M = {"https://src.test/app.wasm":"data:application/wasm;base64,AGFzbQ=="};
  var nativeFetch = window.fetch;
  window.fetch = function (input, init) { return nativeFetch(input, init); };
})();
</script>
</head><body>vendored</body></html>`

	w, resp := postArtifact(t, r, map[string]any{"title": "vendored", "body": body})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	tok := r.tokens.Mint(resp.Artifact.ID, defaultOwnerID)
	req := httptest.NewRequest("GET", "/a/"+resp.Artifact.ID+"?"+rendertoken.Param+"="+tok, nil)
	rec := httptest.NewRecorder()
	r.RenderHandler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	doc := rec.Body.String()

	preamble := strings.Index(doc, "Local-scheme fetch shim")
	manifest := strings.Index(doc, "var M = {")
	require.NotEqual(t, -1, preamble, "framed preamble must carry the data: fetch shim")
	require.NotEqual(t, -1, manifest, "artifact's own manifest wrapper must survive rendering")

	assert.Less(t, preamble, manifest,
		"the preamble's fetch wrapper must install before the artifact's own, "+
			"or the artifact's wrapper captures native fetch and data: URIs go "+
			"back to the path WebKit refuses")
}
