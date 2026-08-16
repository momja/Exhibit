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

	// The stored body carries the payload inline, so at render the artifact
	// makes no request for it at all.
	body := storedBody(t, r, resp.Artifact.ID)
	assert.Contains(t, body, "data:application/wasm;base64,")
	assert.Contains(t, body, "window.fetch = function")
	// Substitution is by interception: the page's own literal is untouched.
	assert.Contains(t, body, `fetch('/build/app.wasm'`)
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
	assert.Contains(t, storedBody(t, r, resp.Artifact.ID), "data:application/wasm;base64,")
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
