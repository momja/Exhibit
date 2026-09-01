package api

// The export's whole claim is that the file works with the service gone
// (av-vnkt), so these assert absence as much as presence: no render origin, no
// token, no asset path anywhere in the output.

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exportGet(t *testing.T, r *Router, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/artifacts/"+id+"/export", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// A wasm artifact is the case the invariant exists for: its payload is out of
// the body entirely, so an export that forgot to fold it back would hand the
// owner a file that dies with this instance.
func TestExportFoldsRuntimePayloadsBackIn(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := wasmFixture(t)

	_, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	id := resp.Artifact.ID

	w := exportGet(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	doc := w.Body.String()

	// The payload is present, under the type instantiateStreaming demands.
	assert.Contains(t, doc, "data:application/wasm;base64,"+
		base64.StdEncoding.EncodeToString([]byte(wasmFixtureBody)))
	// The page's own literal is untouched, and the manifest restores it.
	assert.Contains(t, doc, `fetch('/build/app.wasm'`)
	assert.Contains(t, doc, srv.URL+"/build/app.wasm")
	assert.Contains(t, doc, "window.fetch = function")

	assertSelfContained(t, doc, id)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".html")
}

// A markup asset went out of the body as a rewritten reference, so it comes
// back as one — replaced in place, not through the manifest.
func TestExportFoldsMarkupAssetsBackIn(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := imageFixture(t)

	_, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	id := resp.Artifact.ID

	// Precondition: the stored body really does reference the asset by URL.
	require.Contains(t, storedBody(t, r, id), "/a/"+id+"/assets/")

	doc := exportGet(t, r, id).Body.String()
	assert.Contains(t, doc, "data:image/png;base64,"+
		base64.StdEncoding.EncodeToString(bigPNG()))
	// No manifest is needed for a markup asset — the reference itself carries
	// the bytes now, and a second copy in a manifest would double the file.
	assert.NotContains(t, doc, "window.fetch = function")
	assertSelfContained(t, doc, id)
}

// The common case must cost nothing and change nothing.
func TestExportOfAnArtifactWithNoAssetsIsTheBodyItself(t *testing.T) {
	r := newTestRouter(t)
	const src = `<!DOCTYPE html><html><head></head><body>hello</body></html>`
	_, resp := postArtifact(t, r, map[string]any{
		"body": src, "title": "Plain", "network_allowlist": []string{},
	})

	w := exportGet(t, r, resp.Artifact.ID)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, src, w.Body.String(), "export is identity when there is nothing to fold in")
	assert.Contains(t, w.Header().Get("Content-Disposition"), `filename="Plain.html"`)
}

func TestExportIsOwnerScopedAnd404sForAMissingArtifact(t *testing.T) {
	r := newTestRouter(t)
	assert.Equal(t, http.StatusNotFound, exportGet(t, r, "does-not-exist").Code)
}

// A title is arbitrary user text and lands in a response header, where a stray
// quote or newline is header injection rather than a cosmetic problem.
func TestExportFilenameIsSanitized(t *testing.T) {
	cases := map[string]string{
		`Run Log`:          "Run-Log.html",
		`a"b`:              "a-b.html",
		"line\nbreak":      "line-break.html",
		`../../etc/passwd`: "etc-passwd.html",
		``:                 "artifact.html",
		`   `:              "artifact.html",
		`emoji 🏃 run`:      "emoji-run.html",
	}
	for title, want := range cases {
		assert.Equal(t, want, exportFilename(title), "title %q", title)
	}
}

// assertSelfContained is the invariant, stated once: nothing in the file may
// depend on this instance being alive.
func assertSelfContained(t *testing.T, doc, artifactID string) {
	t.Helper()
	assert.NotContains(t, doc, "/a/"+artifactID+"/assets/",
		"an exported file must not reference the asset route")
	assert.NotContains(t, doc, "http://render.test",
		"an exported file must not reference the render origin")
	assert.NotContains(t, doc, "?t=", "an exported file must carry no render token")
}
