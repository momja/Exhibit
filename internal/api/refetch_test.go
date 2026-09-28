package api

// Refetch regression tests for av-reo3 (refetch overwrites the artifact in
// place, with no way back) and av-b17a (refetch skips the snapshot vendorer
// and seeds the allowlist from a scan).
//
// Every test drives POST /api/artifacts/:id/refetch through the real chi
// router, store, and blob backend, with a mutable httptest server standing in
// for the source site.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refetchUpstream modes.
const (
	upstreamV1      = "v1"      // valid page with relative assets + a runtime wasm fetch
	upstreamV2      = "v2"      // different but valid page: relative img + new external origin
	upstreamBare    = "bare"    // valid page with no assets at all
	upstreamErr404  = "err404"  // 404 status carrying an HTML error body
	upstreamErr500  = "err500"  // 500 status carrying an HTML error body
	upstreamEmpty   = "empty"   // 200 with an empty body
	upstreamNonHTML = "nonhtml" // 200 with a text/plain body
)

const refetchV1Page = `<html><head><title>v1</title><script src="js/app.js"></script><script>fetch('/build/app.wasm')</script></head><body><h1>first</h1><img src="img/a.png"></body></html>`

const refetchV2Page = `<html><head><title>v2</title><script src="https://cdn.example.com/lib.js"></script></head><body><h1>second</h1><img src="img/a.png"></body></html>`

const refetchBarePage = `<html><head><title>bare</title></head><body><h1>no assets here</h1></body></html>`

const refetchWasmBody = "\x00asm\x01\x00\x00\x00module-bytes"

// mutableUpstream serves /page.html according to its current mode and keeps
// the relative assets behind both page versions fetchable, so the snapshot
// pipeline vendors deterministically in either mode.
type mutableUpstream struct {
	srv *httptest.Server

	mu   sync.Mutex
	mode string
}

func (u *mutableUpstream) setMode(mode string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.mode = mode
}

func (u *mutableUpstream) getMode() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.mode
}

func newMutableUpstream(t *testing.T) *mutableUpstream {
	t.Helper()
	u := &mutableUpstream{mode: upstreamV1}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img/a.png":
			w.Header().Set("Content-Type", "image/png")
			io.WriteString(w, "PNG-a-bytes")
			return
		case "/js/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			io.WriteString(w, `console.log("app")`)
			return
		case "/build/app.wasm":
			w.Header().Set("Content-Type", "application/wasm")
			io.WriteString(w, refetchWasmBody)
			return
		case "/page.html":
			// Serve per mode below.
		default:
			http.NotFound(w, r)
			return
		}
		switch u.getMode() {
		case upstreamV1, upstreamV2, upstreamBare:
			page := refetchV1Page
			switch u.getMode() {
			case upstreamV2:
				page = refetchV2Page
			case upstreamBare:
				page = refetchBarePage
			}
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, page)
		case upstreamErr404:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.Error(w, "Not Found: no such page", http.StatusNotFound)
		case upstreamErr500:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		case upstreamEmpty:
			w.Header().Set("Content-Type", "text/html")
		case upstreamNonHTML:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, "just some plain text, not html")
		}
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *mutableUpstream) pageURL() string { return u.srv.URL + "/page.html" }

// doRefetch POSTs a refetch and returns the raw recorder.
func doRefetch(t *testing.T, r *Router, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/refetch", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// refetchBlobID reads the artifact's current source blob id.
func refetchBlobID(t *testing.T, r *Router, id string) string {
	t.Helper()
	var blobID string
	require.NoError(t, json.Unmarshal([]byte(artifactField(t, r, id, "source_blob_id")), &blobID))
	return blobID
}

// createURLArtifact ingests a plain (non-snapshot) artifact from the upstream
// page and returns its id.
func createURLArtifact(t *testing.T, r *Router, pageURL string) string {
	t.Helper()
	return createArtifact(t, r, map[string]any{"url": pageURL, "network_allowlist": []string{}})
}

func TestRefetchRejectsErrorStatus(t *testing.T) {
	for _, mode := range []string{upstreamErr404, upstreamErr500} {
		t.Run(mode, func(t *testing.T) {
			r := newTestRouter(t)
			u := newMutableUpstream(t)

			id := createURLArtifact(t, r, u.pageURL())
			before := getArtifactBody(t, r, id)
			blobBefore := refetchBlobID(t, r, id)

			u.setMode(mode)
			w := doRefetch(t, r, id)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "artifact unchanged")

			// The artifact is byte-identical: same body, same blob.
			assert.Equal(t, before, getArtifactBody(t, r, id))
			assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
		})
	}
}

func TestRefetchRejectsEmptyBody(t *testing.T) {
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	before := getArtifactBody(t, r, id)
	blobBefore := refetchBlobID(t, r, id)

	u.setMode(upstreamEmpty)
	w := doRefetch(t, r, id)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "empty body")

	assert.Equal(t, before, getArtifactBody(t, r, id))
	assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
}

func TestRefetchRejectsNonHTMLContent(t *testing.T) {
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	before := getArtifactBody(t, r, id)
	blobBefore := refetchBlobID(t, r, id)

	u.setMode(upstreamNonHTML)
	w := doRefetch(t, r, id)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "content type")

	assert.Equal(t, before, getArtifactBody(t, r, id))
	assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
}

func TestRefetchConnectionFailureLeavesArtifactAlone(t *testing.T) {
	r := newTestRouter(t)

	// Body-supplied creation only records the URL; nothing is fetched, so a
	// dead source can be set up without a server behind it. (Creation still
	// injects the <base href> fallback for the recorded URL.)
	const body = `<html><body>local copy</body></html>`
	const want = `<base href="http://127.0.0.1:1/closed">` + body
	id := createArtifact(t, r, map[string]any{
		"title": "Offline", "body": body,
		"url": "http://127.0.0.1:1/closed", "network_allowlist": []string{},
	})
	require.Equal(t, want, getArtifactBody(t, r, id))

	w := doRefetch(t, r, id)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, want, getArtifactBody(t, r, id))
}

// TestRefetchValidReplacementRepointsBlob is the feature working: a source
// that serves a different but valid page replaces the body — through a new
// blob, never an in-place overwrite — and the response carries the footprint
// contract without touching the allowlist.
func TestRefetchValidReplacementRepointsBlob(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	blobBefore := refetchBlobID(t, r, id)
	require.FileExists(t, filepath.Join(blobDir, blobBefore))

	u.setMode(upstreamV2)
	w := doRefetch(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp refetchArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	// The new page is stored, with the <base href> fallback the ingest path
	// applies — not the raw fetched bytes.
	after := getArtifactBody(t, r, id)
	assert.Contains(t, after, "<h1>second</h1>")
	assert.Contains(t, after, `<base href="`+u.pageURL()+`">`)
	// The new external origin surfaces as footprint for explicit approval...
	assert.Contains(t, resp.NetworkFootprint, "https://cdn.example.com")
	assert.True(t, resp.FootprintChanged)
	// ...and is never written to the allowlist.
	assert.Empty(t, resp.Artifact.NetworkAllowlist)

	// New blob, old bytes drained: the repoint retired the previous body.
	blobAfter := refetchBlobID(t, r, id)
	require.NotEqual(t, blobBefore, blobAfter)
	assert.NoFileExists(t, filepath.Join(blobDir, blobBefore))
	require.FileExists(t, filepath.Join(blobDir, blobAfter))
}

func TestRefetchUnchangedWritesNothing(t *testing.T) {
	// Snapshot-ingested, so both halves run the same pipeline and the
	// comparison is like with like: a plain-ingested body is not pipeline
	// output, so its first refetch legitimately takes the write path.
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	w, created := postArtifact(t, r, map[string]any{
		"url": u.pageURL(), "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := created.Artifact.ID
	blobBefore := refetchBlobID(t, r, id)

	w = doRefetch(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp refetchArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.FootprintChanged)
	assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
}

// TestRefetchRunsSnapshotPipeline (av-b17a): a refetch re-runs the ingest
// pipeline, so a vendored page stays vendored instead of silently returning
// to live external references.
func TestRefetchRunsSnapshotPipeline(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	w, created := postArtifact(t, r, map[string]any{
		"url": u.pageURL(), "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := created.Artifact.ID

	// Ingest vendored the relative image into a data: URI.
	before := getArtifactBody(t, r, id)
	assert.Contains(t, before, "data:image/png;base64,")
	assert.Contains(t, before, `<base href="`+u.pageURL()+`">`)

	// Upstream changes to a page that still carries a relative image.
	u.setMode(upstreamV2)
	w = doRefetch(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp refetchArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	after := getArtifactBody(t, r, id)
	// Still vendored (data: URI, no live relative ref), still based, and the
	// newly observed external origin is footprint — not allowlist.
	assert.Contains(t, after, "data:image/png;base64,")
	assert.NotContains(t, after, `src="img/a.png"`)
	assert.Contains(t, after, `<base href="`+u.pageURL()+`">`)
	assert.Contains(t, resp.NetworkFootprint, "https://cdn.example.com")
	assert.Empty(t, resp.Artifact.NetworkAllowlist)
	require.NotNil(t, resp.Snapshot)
	assert.True(t, resp.Snapshot.Applied)
}

// TestRefetchWithoutAssetsKeepsExistingAssets (av-reo3 assets trap): a
// refetch that produces no assets must not retire the artifact's existing
// asset generation. Replacing with an empty set would delete every asset row
// and condemn every payload blob.
func TestRefetchWithoutAssetsKeepsExistingAssets(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	// v1 fetches a wasm payload at runtime, so snapshot ingest records a
	// one-row asset generation.
	w, created := postArtifact(t, r, map[string]any{
		"url": u.pageURL(), "snapshot": true, "network_allowlist": []string{},
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	id := created.Artifact.ID

	assetsBefore, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	require.Len(t, assetsBefore, 1)
	require.Equal(t, u.srv.URL+"/build/app.wasm", assetsBefore[0].SourceURL)

	// The bare page vendors nothing at all — no markup assets, no runtime
	// payloads — so the refetch must leave that generation standing.
	u.setMode(upstreamBare)
	w = doRefetch(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, getArtifactBody(t, r, id), "<h1>no assets here</h1>")

	assetsAfter, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Equal(t, assetsBefore, assetsAfter,
		"an asset-less refetch must leave the existing generation untouched")
}

// TestRefetchMissingOldBodyRepairs mirrors the PATCH rule (av-wu9d): when the
// previous bytes are gone, the rewrite still goes through — it is the only
// way to repair the artifact — and the footprint counts as changed.
func TestRefetchMissingOldBodyRepairs(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	require.NoError(t, os.Remove(filepath.Join(blobDir, refetchBlobID(t, r, id))))

	u.setMode(upstreamV2)
	w := doRefetch(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp refetchArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.FootprintChanged)
	assert.Contains(t, getArtifactBody(t, r, id), "<h1>second</h1>")
}
