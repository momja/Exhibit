package snapshot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collectRuntime runs the runtime-asset pass with the dial guard disabled so it
// can reach the loopback httptest origin.
func collectRuntime(t *testing.T, base, body string, limits Limits) ([]RuntimeAsset, []*FetchError) {
	t.Helper()
	f := testFetcher(t, base, limits)
	assets, errs, err := CollectRuntimeAssets(context.Background(), f, body)
	require.NoError(t, err)
	return assets, errs
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

// byURL keys the collected assets the way the render manifest does — by the
// absolute URL the page will actually request.
func byURL(assets []RuntimeAsset) map[string]RuntimeAsset {
	out := make(map[string]RuntimeAsset, len(assets))
	for _, a := range assets {
		out[a.SourceURL] = a
	}
	return out
}

func TestCollectRuntimeAssets(t *testing.T) {
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

	t.Run("collects a root-relative runtime fetch", func(t *testing.T) {
		body := `<html><head><script>
			fetch('/build/app.wasm').then(r => r.arrayBuffer());
		</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)

		// Keyed by absolute URL, because that is what the render manifest's
		// wrapper resolves a request to at call time.
		got := byURL(assets)
		require.Contains(t, got, srv.URL+"/build/app.wasm")
		assert.Equal(t, wasm, got[srv.URL+"/build/app.wasm"].Body)
		assert.Equal(t, "application/wasm", got[srv.URL+"/build/app.wasm"].ContentType)
	})

	t.Run("a constructed call is covered when its URL also appears as a literal", func(t *testing.T) {
		// The literal is what creates the asset; the render wrapper then matches
		// the constructed call at run time because both resolve to the same
		// absolute URL. Collection comes from literals alone — consumption does
		// not, which is exactly why a vanished literal can never authorize
		// deleting an asset.
		body := `<html><head><script>
			fetch('/build/app.wasm');
			var u = BASE + '/build/app.wasm';
			fetch(u);
		</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Contains(t, byURL(assets), srv.URL+"/build/app.wasm")
	})

	t.Run("a constructed-only URL is not collected (limitation, pinned)", func(t *testing.T) {
		// The path fragment sits beside fetch(u), not inside a fetch('...')
		// literal, so nothing is collected and the runtime request still reaches
		// the network. This is the documented boundary of the heuristic — do not
		// 'fix' it by widening the regex into something that speculatively GETs
		// arbitrary paths.
		body := `<html><head><script>
			var u = BASE + '/build/app.wasm';
			fetch(u);
		</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Empty(t, assets)
	})

	t.Run("forces application/wasm when the server sent no type", func(t *testing.T) {
		body := `<html><head><script>fetch('/plain.wasm')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		// Without this the asset route serves the wrong type and
		// instantiateStreaming rejects the response outright.
		assert.Equal(t, "application/wasm", byURL(assets)[srv.URL+"/plain.wasm"].ContentType)
	})

	t.Run("ignores a query string when matching the extension", func(t *testing.T) {
		body := `<html><head><script>fetch('/versioned.wasm?v=3')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Contains(t, byURL(assets), srv.URL+"/versioned.wasm?v=3")
	})

	t.Run("collects non-wasm binary extensions too", func(t *testing.T) {
		body := `<html><head><script>fetch('/heap.data')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Contains(t, byURL(assets), srv.URL+"/heap.data")
	})

	t.Run("leaves non-asset endpoints alone", func(t *testing.T) {
		// No extension match, so it is never fetched: the pass must not
		// speculatively GET arbitrary endpoints.
		body := `<html><head><script>fetch('/api/data')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Empty(t, assets)
	})

	t.Run("ignores a fetch shown in page text rather than script", func(t *testing.T) {
		body := `<html><head></head><body><pre>fetch('/build/app.wasm')</pre></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Empty(t, assets)
	})

	t.Run("records over-cap assets rather than failing the ingest", func(t *testing.T) {
		limits := runtimeLimits()
		limits.MaxInlineAssetBytes = 4 // smaller than the fixture body
		body := `<html><head><script>fetch('/build/app.wasm')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, limits)

		require.Len(t, errs, 1)
		assert.Equal(t, ErrTooLarge, errs[0].Kind)
		// Nothing collected, and the artifact is no worse off than before: its
		// own fetch reaches the network, and the failure is reported rather
		// than surfacing as a bare TypeError at render.
		assert.Empty(t, assets)
	})

	t.Run("records a missing asset", func(t *testing.T) {
		body := `<html><head><script>fetch('/nope.wasm')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Len(t, errs, 1)
		assert.Equal(t, ErrHTTPStatus, errs[0].Kind)
		assert.Empty(t, assets)
	})

	t.Run("fetches each distinct URL once", func(t *testing.T) {
		body := `<html><head><script>
			fetch('/build/app.wasm'); fetch('/build/app.wasm');
		</script><script>fetch('/build/app.wasm')</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Len(t, assets, 1)
	})

	t.Run("leaves ESM import literals to the allowlist", func(t *testing.T) {
		// Native module loading never consults window.fetch, so an asset keyed
		// from an import literal could never be matched by the render wrapper —
		// collecting it would spend budget on bytes nothing reads while the
		// module loader still requests the original URL. Import-derived origins
		// belong to the script-src allowlist, via the footprint pass.
		body := `<html><head><script type="module">import x from '/from-import.bin';</script></head><body></body></html>`
		assets, errs := collectRuntime(t, base, body, runtimeLimits())
		require.Empty(t, errs)
		assert.Empty(t, assets)
	})
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
