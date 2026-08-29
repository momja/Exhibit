package api

// In-process API integration tests for the runtime-asset half of the snapshot
// vendorer (av-ghvs): a page that fetches a binary payload from JavaScript must
// come back self-contained, because relocating it to the render origin turns
// that same-origin fetch into a cross-origin one the source site never sends
// CORS headers for.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withFetcherLimits is withUnguardedFetcher with a custom budget, so the
// over-cap path can be exercised without a multi-megabyte fixture.
func withFetcherLimits(t *testing.T, limits snapshot.Limits) {
	t.Helper()
	orig := newSnapshotFetcher
	newSnapshotFetcher = func(pageURL string) (*snapshot.Fetcher, error) {
		return snapshot.NewFetcherForTests(pageURL, limits)
	}
	t.Cleanup(func() { newSnapshotFetcher = orig })
}

const wasmFixturePage = `<!DOCTYPE html><html><head>
<title>Wasm Fixture</title>
<script>
  async function boot() {
    const res = await fetch('/build/app.wasm', { cache: 'no-store' });
    return WebAssembly.compile(await res.arrayBuffer());
  }
</script>
</head><body></body></html>`

const wasmFixtureBody = "\x00asm\x01\x00\x00\x00module-bytes"

func wasmFixture(t *testing.T) *httptest.Server {
	t.Helper()
	files := map[string]struct{ contentType, body string }{
		"/page.html":      {"text/html", wasmFixturePage},
		"/build/app.wasm": {"application/wasm", wasmFixtureBody},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", f.contentType)
		_, _ = io.WriteString(w, f.body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSnapshotIngestVendorsRuntimeFetchedWasm(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := wasmFixture(t)

	w, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	rep := resp.Snapshot
	require.NotNil(t, rep)
	assert.True(t, rep.Applied)
	assert.Empty(t, rep.Error)
	assert.Empty(t, rep.Failures)

	// The wasm the markup walker cannot see is vendored, and its bytes are
	// counted in the same budget as the markup assets.
	assert.Equal(t, []string{srv.URL + "/build/app.wasm"}, rep.VendoredURLs)
	assert.Equal(t, int64(len(wasmFixtureBody)), rep.VendoredBytes)

	// av-20fk: the payload is a blob of its own, and the stored body is the
	// page as it was fetched. This is the property the whole ticket rests on —
	// the body's size is now independent of its payloads', so an agent can
	// read and rewrite it, and the render document does not re-transfer 21 MB
	// of base64 on every view.
	body := storedBody(t, r, resp.Artifact.ID)
	assert.NotContains(t, body, "base64,", "the payload must not be inlined in the body")
	assert.NotContains(t, body, "window.fetch = function",
		"the manifest belongs to the render preamble, not the stored body")
	// The page's own literal is untouched: nothing is rewritten at ingest.
	assert.Contains(t, body, `fetch('/build/app.wasm'`)

	// And the bytes are recorded as an asset, under the URL the page will ask
	// for at run time and the type instantiateStreaming demands.
	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, resp.Artifact.ID)
	require.NoError(t, err)
	require.Len(t, assets, 1)
	assert.Equal(t, srv.URL+"/build/app.wasm", assets[0].SourceURL)
	assert.Equal(t, "application/wasm", assets[0].ContentType)
	assert.Equal(t, int64(len(wasmFixtureBody)), assets[0].SizeBytes)
}

// The end-to-end claim: a page whose wasm was vendored renders with a manifest
// pointing at the asset route, and that route hands back the real bytes with
// the headers the browser needs — CORS for the opaque-origin frame, the exact
// wasm type for instantiateStreaming, and a cache directive, which is what
// makes the second view (and every agent-preview reload) free.
func TestRuntimeAssetIsServedFromTheAssetRoute(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := wasmFixture(t)

	_, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	require.NotNil(t, resp.Artifact)
	id := resp.Artifact.ID

	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	require.Len(t, assets, 1)

	// The render document carries the manifest, keyed by the URL the page
	// requests and pointing at this artifact's own asset path.
	doc := renderGet(t, r, "/a/"+id+"?"+rendertoken.Param+"="+r.tokens.Mint(id, defaultOwnerID))
	require.Equal(t, http.StatusOK, doc.Code, doc.Body.String())
	assetURL := "/a/" + id + "/assets/" + assets[0].ID
	assert.Contains(t, doc.Body.String(), srv.URL+"/build/app.wasm")
	assert.Contains(t, doc.Body.String(), assetURL)

	// The CSP permits it, as a system source scoped to this artifact's path —
	// and the allowlist stays empty, so the user was asked to approve nothing.
	csp := doc.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "/a/"+id+"/assets/")
	assert.Empty(t, resp.Artifact.NetworkAllowlist)

	// And the route itself.
	got := renderGet(t, r, assetURL)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	assert.Equal(t, wasmFixtureBody, got.Body.String())
	assert.Equal(t, "application/wasm", got.Header().Get("Content-Type"))
	assert.Equal(t, "*", got.Header().Get("Access-Control-Allow-Origin"),
		"an opaque-origin sandbox sends Origin: null, which only * matches")
	assert.Contains(t, got.Header().Get("Cache-Control"), "immutable",
		"caching across views is half the reason the payload left the body")
	assert.NotEqual(t, "no-store", got.Header().Get("Cache-Control"))
}

// An asset is reachable only through the artifact that owns it, even by
// somebody holding both ids.
func TestAssetRouteRefusesAnotherArtifactsAsset(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := wasmFixture(t)

	_, first := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	_, second := postArtifact(t, r, map[string]any{
		"body": "<html><body>unrelated</body></html>", "network_allowlist": []string{},
	})

	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, first.Artifact.ID)
	require.NoError(t, err)
	require.Len(t, assets, 1)

	w := renderGet(t, r, "/a/"+second.Artifact.ID+"/assets/"+assets[0].ID)
	assert.Equal(t, http.StatusNotFound, w.Code,
		"an asset must not be addressable through an artifact that does not own it")
}

func TestSnapshotIngestReportsOverCapRuntimeAsset(t *testing.T) {
	limits := snapshot.DefaultLimits()
	limits.MaxInlineAssetBytes = 4 // smaller than the fixture payload
	withFetcherLimits(t, limits)
	r := newTestRouter(t)
	srv := wasmFixture(t)

	w, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// Ingest still succeeds — the artifact is stored, just not self-contained.
	rep := resp.Snapshot
	require.NotNil(t, rep)
	assert.True(t, rep.Applied)

	// The reason is reported rather than left to surface as a TypeError in the
	// browser. The gallery's ingest panel flags any report carrying failures.
	require.Len(t, rep.Failures, 1)
	assert.Equal(t, string(snapshot.ErrTooLarge), rep.Failures[0].Kind)
	assert.Equal(t, srv.URL+"/build/app.wasm", rep.Failures[0].URL)

	// And the origin still surfaces for approval, since the fetch will happen.
	assert.Contains(t, resp.NetworkFootprint, srv.URL)

	body := storedBody(t, r, resp.Artifact.ID)
	assert.NotContains(t, body, "data:application/wasm;base64,")
	assert.Contains(t, body, `fetch('/build/app.wasm'`)
}

// Vendoring must not depend on the artifact's allowlist: the whole point is
// that the tool runs without approving the origin it was imported from.
func TestSnapshotIngestRuntimeAssetNeedsNoAllowlist(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	srv := wasmFixture(t)

	_, resp := postArtifact(t, r, map[string]any{
		"url": srv.URL + "/page.html", "snapshot": true, "network_allowlist": []string{},
	})
	require.NotNil(t, resp.Artifact)
	assert.Empty(t, resp.Artifact.NetworkAllowlist, "ingest must never seed the allowlist")

	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, resp.Artifact.ID)
	require.NoError(t, err)
	require.Len(t, assets, 1, "the payload is vendored with no origin approved")
}

// storedBody reads an artifact's stored body back through the API.
func storedBody(t *testing.T, r *Router, id string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/artifacts/"+id+"?body=true", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var out struct {
		Body string `json:"body"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&out))
	require.NotEmpty(t, strings.TrimSpace(out.Body))
	return out.Body
}
