package api

// The render preamble and the snapshot vendorer BOTH wrap window.fetch, and the
// two fixes only work composed:
//
//   - av-ghvs (the vendorer) inlines a runtime-fetched asset as a data: URI and
//     injects a manifest + fetch wrapper at the top of the artifact's <head>, so
//     the artifact's original request is answered locally instead of cross-origin.
//   - agaf-02xs (the preamble) wraps fetch to answer data: URLs from locally
//     constructed Responses, because WebKit refuses large data: fetches from an
//     opaque-origin sandbox.
//
// So the vendorer routes the request to a data: URI, and the preamble is what
// makes that data: URI work in Safari's iframe. Neither is sufficient alone.
//
// Their composition depends entirely on install order. Each wrapper captures
// whatever window.fetch is at its own install time, and injectPreamble inserts
// the preamble immediately after <head> — ahead of the manifest the vendorer put
// there at ingest. That ordering makes the chain
//
//	artifact fetch -> manifest (matches, hands over a data: URI)
//	               -> preamble (decodes it to a local Response)
//
// Invert it and the manifest would capture the *native* fetch, quietly sending
// the data: URI back to the path WebKit refuses: Safari breaks again, with no
// test failing and nothing visible in Chromium. Nothing else pins this, so pin
// it here.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	req := httptest.NewRequest("GET", "/a/"+resp.Artifact.ID, nil)
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
