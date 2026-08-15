package snapshot

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// inlineRuntime runs the runtime-asset transform with the dial guard disabled
// so it can reach the loopback httptest origin.
func inlineRuntime(t *testing.T, base, body string, limits Limits) (string, []*FetchError) {
	t.Helper()
	f := testFetcher(t, base, limits)
	out, errs, err := InlineRuntimeAssets(context.Background(), f, body)
	require.NoError(t, err)
	return out, errs
}

func runtimeLimits() Limits {
	return Limits{
		MaxAssetBytes:       1 << 10,
		MaxInlineAssetBytes: 1 << 20,
		MaxTotalBytes:       4 << 20,
		MaxAssets:           10,
		Timeout:             5 * time.Second,
	}
}

// manifestOf extracts the injected manifest object so tests assert on the
// mapping rather than on the exact shape of the surrounding wrapper.
func manifestOf(t *testing.T, doc string) map[string]string {
	t.Helper()
	m := regexp.MustCompile(`var M = (\{.*?\});`).FindStringSubmatch(doc)
	require.Len(t, m, 2, "no manifest found in document")
	var out map[string]string
	require.NoError(t, json.Unmarshal([]byte(m[1]), &out))
	return out
}

func TestInlineRuntimeAssets(t *testing.T) {
	wasm := []byte("\x00asm-module-bytes")
	srv := assetOrigin(t, map[string]testAsset{
		"/build/app.wasm":  {contentType: "application/wasm", body: wasm},
		"/plain.wasm":      {body: wasm}, // server sends no Content-Type
		"/heap.data":       {body: []byte("heap-bytes")},
		"/api/data":        {contentType: "application/json", body: []byte(`{"a":1}`)},
		"/versioned.wasm":  {contentType: "application/wasm", body: wasm},
		"/from-import.bin": {body: []byte("bin-bytes")},
	})
	base := srv.URL + "/index.html"

	t.Run("vendors a root-relative runtime fetch", func(t *testing.T) {
		body := `<html><head><script>
			fetch('/build/app.wasm').then(r => r.arrayBuffer());
		</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)

		// Keyed by absolute URL, because that is what the interceptor resolves
		// a request to at call time.
		man := manifestOf(t, out)
		require.Contains(t, man, srv.URL+"/build/app.wasm")
		assert.Equal(t, "data:application/wasm;base64,"+b64(wasm), man[srv.URL+"/build/app.wasm"])

		// The original literal is deliberately left in place — substitution is
		// by interception, not by rewriting the source.
		assert.Contains(t, out, `fetch('/build/app.wasm')`)
	})

	t.Run("a runtime-constructed call is served when its URL also appears as a literal fetch ref", func(t *testing.T) {
		// The literal fetch below is what puts the URL in the manifest; the
		// wrapper then matches the constructed call at run time because both
		// resolve to the same absolute URL. The interception mechanism serves
		// the constructed call — the manifest entry, not the call, is what a
		// literal produced.
		body := `<html><head><script>
			fetch('/build/app.wasm');
			var u = BASE + '/build/app.wasm';
			fetch(u);
		</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Contains(t, manifestOf(t, out), srv.URL+"/build/app.wasm")
	})

	t.Run("a constructed-only URL is not vendored (limitation, pinned)", func(t *testing.T) {
		// The path fragment here sits beside fetch(u), not inside a fetch('...')
		// literal, so no manifest entry is created and no wrapper is injected:
		// the runtime-constructed request still reaches the network. This is the
		// documented boundary of the heuristic — do not 'fix' it by widening the
		// regex into something that speculatively GETs arbitrary paths.
		body := `<html><head><script>
			var u = BASE + '/build/app.wasm';
			fetch(u);
		</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.NotContains(t, out, "var M =")
		assert.Equal(t, body, out)
	})

	t.Run("forces application/wasm when the server sent no type", func(t *testing.T) {
		body := `<html><head><script>fetch('/plain.wasm')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		// Without this, instantiateStreaming rejects the response outright.
		assert.Equal(t, "data:application/wasm;base64,"+b64(wasm), manifestOf(t, out)[srv.URL+"/plain.wasm"])
	})

	t.Run("ignores a query string when matching the extension", func(t *testing.T) {
		body := `<html><head><script>fetch('/versioned.wasm?v=3')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Contains(t, manifestOf(t, out), srv.URL+"/versioned.wasm?v=3")
	})

	t.Run("vendors non-wasm binary extensions too", func(t *testing.T) {
		body := `<html><head><script>fetch('/heap.data')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Contains(t, manifestOf(t, out), srv.URL+"/heap.data")
	})

	t.Run("leaves non-asset endpoints alone", func(t *testing.T) {
		// No extension match, so it is never fetched and no manifest is injected
		// at all — the pass must not speculatively GET arbitrary endpoints.
		body := `<html><head><script>fetch('/api/data')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.NotContains(t, out, "var M =")
		assert.Equal(t, body, out)
	})

	t.Run("ignores a fetch shown in page text rather than script", func(t *testing.T) {
		body := `<html><head></head><body><pre>fetch('/build/app.wasm')</pre></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.NotContains(t, out, "var M =")
	})

	t.Run("records over-cap assets and leaves the reference untouched", func(t *testing.T) {
		limits := runtimeLimits()
		limits.MaxInlineAssetBytes = 4 // smaller than the fixture body
		body := `<html><head><script>fetch('/build/app.wasm')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, limits)

		require.Len(t, errs, 1)
		assert.Equal(t, ErrTooLarge, errs[0].Kind)
		// No manifest, original reference intact: the artifact is no worse off
		// than before, and the failure is reported rather than silent.
		assert.NotContains(t, out, "var M =")
		assert.Contains(t, out, `fetch('/build/app.wasm')`)
	})

	t.Run("records a missing asset", func(t *testing.T) {
		body := `<html><head><script>fetch('/nope.wasm')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Len(t, errs, 1)
		assert.Equal(t, ErrHTTPStatus, errs[0].Kind)
		assert.NotContains(t, out, "var M =")
	})

	t.Run("fetches each distinct URL once", func(t *testing.T) {
		body := `<html><head><script>
			fetch('/build/app.wasm'); fetch('/build/app.wasm');
		</script><script>fetch('/build/app.wasm')</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Len(t, manifestOf(t, out), 1)
	})

	t.Run("leaves ESM import literals to the allowlist", func(t *testing.T) {
		// Native module loading never consults window.fetch, so a manifest entry
		// keyed from an import literal could never be matched — vendoring it
		// would spend budget on bytes nothing reads while the module loader
		// still requests the original URL. Import-derived origins belong to the
		// script-src allowlist, via the footprint pass.
		body := `<html><head><script type="module">import x from '/from-import.bin';</script></head><body></body></html>`
		out, errs := inlineRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.NotContains(t, out, "var M =")
		assert.Equal(t, body, out)
	})
}

func TestInlineRuntimeAssetsInterceptorShape(t *testing.T) {
	wasm := []byte("\x00asm")
	srv := assetOrigin(t, map[string]testAsset{
		"/a.wasm": {contentType: "application/wasm", body: wasm},
	})
	body := `<html><head><script>fetch('/a.wasm')</script></head><body></body></html>`
	out, errs := inlineRuntime(t, srv.URL+"/index.html", body, runtimeLimits())
	require.Empty(t, errs)

	// Installed ahead of the page's own script so it is in place before any
	// artifact code can call fetch.
	manifestAt := strings.Index(out, "var M =")
	pageScriptAt := strings.Index(out, `fetch('/a.wasm')`)
	require.NotEqual(t, -1, manifestAt)
	assert.Less(t, manifestAt, pageScriptAt, "interceptor must be injected before page scripts")

	assert.Contains(t, out, "window.fetch = function", "must wrap fetch rather than rewrite literals")
	assert.Contains(t, out, "toUpperCase() === 'GET'", "non-GET must reach the network")
	assert.Contains(t, out, "document.baseURI", "relative requests resolve against the document base")
}

// A manifest key is attacker-influenced text — it comes from the imported page —
// and it lands inside a <script> element as raw text that html.Render emits
// verbatim. json.Marshal's \u00NN escaping of < > & is the only thing keeping it
// inert, so assert that directly rather than through a fetch (Go's own HTTP
// server rejects a request URI containing raw markup with a 400, so this path is
// not reachable end-to-end).
func TestInjectManifestEscapesMarkup(t *testing.T) {
	const hostile = `https://evil.test/x.wasm?p=</script><img src=x onerror=alert(1)>`

	doc, err := html.Parse(strings.NewReader(`<html><head></head><body></body></html>`))
	require.NoError(t, err)

	in := &runtimeInliner{manifest: map[string]string{hostile: "data:application/wasm;base64,AA=="}}
	in.injectManifest(doc)

	var buf strings.Builder
	require.NoError(t, html.Render(&buf, doc))
	out := buf.String()

	assert.NotContains(t, out, "</script><img", "manifest must not close its script and inject markup")
	assert.NotContains(t, out, "<img", "manifest must not emit live markup")
	assert.Contains(t, out, `<`, "markup characters are unicode-escaped by json.Marshal")
	// Escaped on the wire, but still the real URL once the browser parses it,
	// so the interceptor's lookup still matches.
	assert.Equal(t, "data:application/wasm;base64,AA==", manifestOf(t, out)[hostile])
}

func TestInlinableExt(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{"wasm", "/app.wasm", true},
		{"wasm with query", "/app.wasm?v=2", true},
		{"wasm with fragment", "/app.wasm#x", true},
		{"uppercase", "/APP.WASM", true},
		{"emscripten data", "/game.data", true},
		{"bin", "/model.bin", true},
		{"mem", "/heap.mem", true},
		{"absolute url", "https://cdn.example.com/a/b.wasm", true},
		{"no extension", "/api/data", false},
		{"json", "/config.json", false},
		{"js", "/main.js", false},
		{"png", "/logo.png", false},
		{"extension only in query", "/api?file=a.wasm", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, inlinableExt(tt.ref))
		})
	}
}

// The runtime pass gets a larger per-asset cap than the markup walker, and a
// too-large verdict recorded under the small cap must not leak to the large one.
func TestFetchWithCapOverridesPerAssetLimit(t *testing.T) {
	payload := []byte(strings.Repeat("x", 2048))
	srv := assetOrigin(t, map[string]testAsset{
		"/big.wasm": {contentType: "application/wasm", body: payload},
	})
	limits := Limits{
		MaxAssetBytes:       1 << 10, // 1 KiB — too small for the payload
		MaxInlineAssetBytes: 1 << 20,
		MaxTotalBytes:       4 << 20,
		MaxAssets:           10,
		Timeout:             5 * time.Second,
	}
	f := testFetcher(t, srv.URL+"/index.html", limits)

	_, err := f.Fetch(context.Background(), "/big.wasm")
	require.Error(t, err)
	assert.Equal(t, ErrTooLarge, fetchErr(t, err).Kind)

	// Same URL, bigger cap: the cached too-large verdict must be re-evaluated.
	asset, err := f.FetchWithCap(context.Background(), "/big.wasm", limits.MaxInlineAssetBytes)
	require.NoError(t, err)
	assert.Equal(t, payload, asset.Body)
}
