package api

// Refetch regression tests for av-reo3 (refetch overwrites the artifact in
// place, with no way back) and av-b17a (refetch skips the snapshot vendorer
// and seeds the allowlist from a scan).
//
// Every test drives POST /api/artifacts/:id/refetch through the real chi
// router, store, and blob backend, with a mutable httptest server standing in
// for the source site.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/store"
)

// refetchV1Page carries relative markup assets and a runtime wasm fetch, so a
// snapshot ingest of it records one runtime asset.
const refetchV1Page = `<html><head><title>v1</title><script src="js/app.js"></script><script>fetch('/build/app.wasm')</script></head><body><h1>first</h1><img src="img/a.png"></body></html>`

// refetchV2Page is a different but valid page. Its new external origin is a
// fetch literal rather than a <script src>, so the vendorer never tries to
// reach it: the scan reports it and no test touches the real network.
const refetchV2Page = `<html><head><title>v2</title><script>fetch("https://api.example.com/v2/data")</script></head><body><h1>second</h1><img src="img/a.png"></body></html>`

const refetchBarePage = `<html><head><title>bare</title></head><body><h1>no assets here</h1></body></html>`

const refetchWasmV1 = "\x00asm\x01\x00\x00\x00module-bytes"

// upstreamPage is what the source site answers for /page.html.
type upstreamPage struct {
	status int
	ctype  string
	body   string
}

var (
	upstreamV1   = upstreamPage{http.StatusOK, "text/html", refetchV1Page}
	upstreamV2   = upstreamPage{http.StatusOK, "text/html", refetchV2Page}
	upstreamBare = upstreamPage{http.StatusOK, "text/html", refetchBarePage}
	// The error pages are HTML on purpose. If they were plain text the
	// content-type guard would refuse them too, and the status guard could be
	// deleted without any test noticing.
	upstream404   = upstreamPage{http.StatusNotFound, "text/html; charset=utf-8", `<html><body><h1>Not Found</h1></body></html>`}
	upstream500   = upstreamPage{http.StatusInternalServerError, "text/html; charset=utf-8", `<html><body><h1>Server Error</h1></body></html>`}
	upstreamEmpty = upstreamPage{http.StatusOK, "text/html", ""}
	upstreamJSON  = upstreamPage{http.StatusOK, "application/json", `{"not":"a page"}`}
)

// mutableUpstream serves /page.html and the wasm payload from whatever it was
// last told to, and keeps the relative assets behind every page version
// fetchable, so the snapshot pipeline vendors deterministically.
type mutableUpstream struct {
	srv *httptest.Server

	mu   sync.Mutex
	page upstreamPage
	wasm string
}

func newMutableUpstream(t *testing.T) *mutableUpstream {
	t.Helper()
	u := &mutableUpstream{page: upstreamV1, wasm: refetchWasmV1}
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *mutableUpstream) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	page, wasm := u.page, u.wasm
	u.mu.Unlock()

	switch r.URL.Path {
	case "/page.html":
		w.Header().Set("Content-Type", page.ctype)
		w.WriteHeader(page.status)
		io.WriteString(w, page.body)
	case "/build/app.wasm":
		w.Header().Set("Content-Type", "application/wasm")
		io.WriteString(w, wasm)
	case "/img/a.png":
		w.Header().Set("Content-Type", "image/png")
		io.WriteString(w, "PNG-a-bytes")
	case "/js/app.js":
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, `console.log("app")`)
	default:
		http.NotFound(w, r)
	}
}

func (u *mutableUpstream) serves(page upstreamPage) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.page = page
}

func (u *mutableUpstream) servesWasm(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.wasm = body
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

// refetchOK refetches, requires a 200, and decodes the response.
func refetchOK(t *testing.T, r *Router, id string) refetchArtifactResponse {
	t.Helper()
	w := doRefetch(t, r, id)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp refetchArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
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

// createSnapshotArtifact ingests the upstream page with snapshot on. The
// caller must have installed withUnguardedFetcher, since the fixture server
// is on loopback.
func createSnapshotArtifact(t *testing.T, r *Router, pageURL string, allowlist []string) string {
	t.Helper()
	w, created := postArtifact(t, r, map[string]any{
		"url": pageURL, "snapshot": true, "network_allowlist": allowlist,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return created.Artifact.ID
}

func listAssets(t *testing.T, r *Router, id string) []store.ArtifactAsset {
	t.Helper()
	assets, err := r.cfg.Store.ListArtifactAssets(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	return assets
}

func TestRefetchArtifactWithoutSourceURL(t *testing.T) {
	r := newTestRouter(t)

	// Paste-created artifact has no source URL.
	id := createArtifact(t, r, map[string]any{
		"title":             "Pasted",
		"body":              "<html><body>pasted</body></html>",
		"network_allowlist": []string{},
	})

	w := doRefetch(t, r, id)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "no source URL")
}

// Each guard refuses a response the protocol says is not the page, names why,
// and leaves the artifact byte-identical: same body, same blob.
func TestRefetchRefusesWhatIsNotThePage(t *testing.T) {
	cases := []struct {
		name string
		page upstreamPage
		want string
	}{
		{"404", upstream404, "source returned 404"},
		{"500", upstream500, "source returned 500"},
		{"empty body", upstreamEmpty, "empty body"},
		{"non-HTML content type", upstreamJSON, `content type "application/json"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(t)
			u := newMutableUpstream(t)

			id := createURLArtifact(t, r, u.pageURL())
			before := getArtifactBody(t, r, id)
			blobBefore := refetchBlobID(t, r, id)

			u.serves(tc.page)
			w := doRefetch(t, r, id)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tc.want)
			assert.Contains(t, w.Body.String(), "artifact unchanged")

			assert.Equal(t, before, getArtifactBody(t, r, id))
			assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
		})
	}
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
// that serves a different but valid page replaces the body, through a new
// blob and never an in-place overwrite. No content heuristic may make this
// refuse.
func TestRefetchValidReplacementRepointsBlob(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	blobBefore := refetchBlobID(t, r, id)
	require.FileExists(t, filepath.Join(blobDir, blobBefore))

	u.serves(upstreamV2)
	resp := refetchOK(t, r, id)

	// The new page is stored, with the <base href> fallback the ingest path
	// applies, not the raw fetched bytes.
	after := getArtifactBody(t, r, id)
	assert.Contains(t, after, "<h1>second</h1>")
	assert.Contains(t, after, `<base href="`+u.pageURL()+`">`)
	assert.True(t, resp.Updated)
	assert.Contains(t, resp.NetworkFootprint, "https://api.example.com")
	assert.Contains(t, resp.UndecidedOrigins, "https://api.example.com")
	assert.True(t, resp.FootprintChanged)

	// New blob, old bytes drained: the repoint retired the previous body.
	blobAfter := refetchBlobID(t, r, id)
	require.NotEqual(t, blobBefore, blobAfter)
	assert.NoFileExists(t, filepath.Join(blobDir, blobBefore))
	require.FileExists(t, filepath.Join(blobDir, blobAfter))
}

// TestRefetchRevendorsAndLeavesOriginDecisionsAlone (av-b17a): a refetch of a
// snapshotted artifact re-runs the ingest pipeline, so a vendored page stays
// vendored, and never writes the allowlist. The artifact starts with an
// approved origin and a refused one, so a refetch that wiped decisions fails
// here as surely as one that widened them. The refused one is the origin the
// new page contacts: it is footprint, but not undecided, since the user has
// already answered for it.
func TestRefetchRevendorsAndLeavesOriginDecisionsAlone(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	id := createSnapshotArtifact(t, r, u.pageURL(), []string{"https://approved.example.com"})
	require.NoError(t, r.cfg.Store.SetOriginDecision(t.Context(), defaultOwnerID, id,
		"https://api.example.com", store.DecisionBlock, "user"))
	decisionsBefore, err := r.cfg.Store.ListOriginDecisions(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	require.Len(t, decisionsBefore, 2)

	// Ingest vendored the relative image into a data: URI.
	before := getArtifactBody(t, r, id)
	assert.Contains(t, before, "data:image/png;base64,")

	// Upstream changes to a page that still carries a relative image and
	// adds an external origin.
	u.serves(upstreamV2)
	resp := refetchOK(t, r, id)

	after := getArtifactBody(t, r, id)
	assert.Contains(t, after, "<h1>second</h1>")
	assert.Contains(t, after, "data:image/png;base64,")
	assert.NotContains(t, after, `src="img/a.png"`)
	assert.Contains(t, after, `<base href="`+u.pageURL()+`">`)
	require.NotNil(t, resp.Snapshot)
	assert.True(t, resp.Snapshot.Applied)

	// The new origin is footprint, and the refusal already covers it...
	assert.Contains(t, resp.NetworkFootprint, "https://api.example.com")
	assert.Empty(t, resp.UndecidedOrigins)
	// ...and every existing decision is exactly as it was.
	assert.Equal(t, []string{"https://approved.example.com"}, resp.Artifact.NetworkAllowlist)
	decisionsAfter, err := r.cfg.Store.ListOriginDecisions(t.Context(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Equal(t, decisionsBefore, decisionsAfter)
}

func TestRefetchUnchangedWritesNothing(t *testing.T) {
	// Snapshot-ingested, so both halves run the same pipeline and the
	// comparison is like with like: a plain-ingested body is not pipeline
	// output, so its first refetch legitimately takes the write path.
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	id := createSnapshotArtifact(t, r, u.pageURL(), []string{})
	blobBefore := refetchBlobID(t, r, id)
	assetsBefore := listAssets(t, r, id)
	require.Len(t, assetsBefore, 1)

	resp := refetchOK(t, r, id)
	assert.False(t, resp.Updated)
	assert.False(t, resp.FootprintChanged)
	assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
	assert.Equal(t, assetsBefore, listAssets(t, r, id), "no new asset generation")
}

// TestRefetchPicksUpAChangedRuntimeAsset: a runtime payload never appears in
// the body (av-20fk), so an upstream that ships a new build behind the same
// HTML changes the assets alone. That is an update, not "unchanged".
func TestRefetchPicksUpAChangedRuntimeAsset(t *testing.T) {
	withUnguardedFetcher(t)
	r, blobDir := newTestRouterWithBlobDir(t)
	u := newMutableUpstream(t)

	id := createSnapshotArtifact(t, r, u.pageURL(), []string{})
	blobBefore := refetchBlobID(t, r, id)
	assetsBefore := listAssets(t, r, id)
	require.Len(t, assetsBefore, 1)

	const rebuilt = "\x00asm\x01\x00\x00\x00module-bytes, rebuilt"
	u.servesWasm(rebuilt)
	resp := refetchOK(t, r, id)
	assert.True(t, resp.Updated)
	assert.False(t, resp.FootprintChanged)

	// The body did not change, so it was not rewritten.
	assert.Equal(t, blobBefore, refetchBlobID(t, r, id))

	// The asset now serves the new bytes, and the old payload is drained.
	assetsAfter := listAssets(t, r, id)
	require.Len(t, assetsAfter, 1)
	assert.Equal(t, u.srv.URL+"/build/app.wasm", assetsAfter[0].SourceURL)
	assert.Equal(t, store.AssetBlobID(defaultOwnerID, []byte(rebuilt)), assetsAfter[0].BlobID)
	assert.Equal(t, int64(len(rebuilt)), assetsAfter[0].SizeBytes)
	assert.NoFileExists(t, filepath.Join(blobDir, assetsBefore[0].BlobID))
}

// TestRefetchWithoutAssetsKeepsExistingAssets (av-reo3 assets trap): a
// refetch that produces no assets must not retire the artifact's existing
// asset generation. Replacing with an empty set would delete every asset row
// and condemn every payload blob.
func TestRefetchWithoutAssetsKeepsExistingAssets(t *testing.T) {
	withUnguardedFetcher(t)
	r := newTestRouter(t)
	u := newMutableUpstream(t)

	id := createSnapshotArtifact(t, r, u.pageURL(), []string{})
	assetsBefore := listAssets(t, r, id)
	require.Len(t, assetsBefore, 1)
	require.Equal(t, u.srv.URL+"/build/app.wasm", assetsBefore[0].SourceURL)

	// The bare page vendors nothing at all, no markup assets and no runtime
	// payloads, so the refetch must leave that generation standing.
	u.serves(upstreamBare)
	refetchOK(t, r, id)
	assert.Contains(t, getArtifactBody(t, r, id), "<h1>no assets here</h1>")
	assert.Equal(t, assetsBefore, listAssets(t, r, id),
		"an asset-less refetch must leave the existing generation untouched")
}

// TestRefetchMissingOldBodyRepairs mirrors the PATCH rule (av-wu9d): when the
// previous bytes are gone, the rewrite still goes through, since it is the
// only way to repair the artifact, and the footprint counts as changed.
func TestRefetchMissingOldBodyRepairs(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	require.NoError(t, os.Remove(filepath.Join(blobDir, refetchBlobID(t, r, id))))

	u.serves(upstreamV2)
	resp := refetchOK(t, r, id)
	assert.True(t, resp.FootprintChanged)
	assert.Contains(t, getArtifactBody(t, r, id), "<h1>second</h1>")
}

// failingRepointStore is the real store with the refetch repoint broken: the
// new body has been written and the row never moves to it, which is the
// window a crash mid-refetch lands in.
type failingRepointStore struct {
	store.Store
}

func (failingRepointStore) ReplaceSourceBlob(context.Context, int64, string, string, string) ([]string, error) {
	return nil, errors.New("disk full")
}

// TestRefetchFailedRepointLeavesThePreviousBody (av-reo3): nothing is
// destroyed until the replacement is in place, so a refetch that dies before
// the repoint leaves the artifact on its previous body, and the new bytes it
// had already written do not linger on disk.
func TestRefetchFailedRepointLeavesThePreviousBody(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)
	u := newMutableUpstream(t)

	id := createURLArtifact(t, r, u.pageURL())
	before := getArtifactBody(t, r, id)
	blobBefore := refetchBlobID(t, r, id)
	filesBefore := blobFileNames(t, blobDir)

	real := r.cfg.Store
	r.cfg.Store = failingRepointStore{Store: real}
	u.serves(upstreamV2)
	w := doRefetch(t, r, id)
	r.cfg.Store = real

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Equal(t, before, getArtifactBody(t, r, id))
	assert.Equal(t, blobBefore, refetchBlobID(t, r, id))
	assert.Equal(t, filesBefore, blobFileNames(t, blobDir), "the unused new body must be removed")
}

func blobFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}
