package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/blob"
)

// createArtifact POSTs a body-based artifact and returns its id.
func createArtifact(t *testing.T, r *Router, payload map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp["artifact"].(map[string]any)["id"].(string)
}

// getArtifactBody GETs the stored source body of an artifact through the API.
func getArtifactBody(t *testing.T, r *Router, id string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/artifacts/"+id+"?body=true", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got struct {
		Body string `json:"body"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	return got.Body
}

func TestExtractTitle(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"simple", `<html><head><title>My Tool</title></head><body></body></html>`, "My Tool"},
		{"whitespace trimmed", `<html><head><title>  Spaced  </title></head></html>`, "Spaced"},
		{"no title", `<html><body><h1>No title here</h1></body></html>`, ""},
		{"empty title", `<html><head><title></title></head></html>`, ""},
		{"first title wins", `<html><head><title>First</title></head><body><title>Second</title></body></html>`, "First"},
		{"fragment without head", `<title>Bare</title><div>content</div>`, "Bare"},
		{"empty input", ``, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, extractTitle(c.html))
		})
	}
}

func TestCreateArtifactFromURL(t *testing.T) {
	r := newTestRouter(t)

	const page = `<html><head><title>Fetched Tool</title></head><body><h1>hi</h1></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, page)
	}))
	defer srv.Close()

	id := createArtifact(t, r, map[string]any{"url": srv.URL, "network_allowlist": []string{}})

	// Title is extracted from the fetched <title>.
	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var art map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	assert.Equal(t, "Fetched Tool", art["title"])

	// Fetched content is stored with only the <base href> fallback injected
	// (exhibit-lwb.6), so surviving relative references resolve against the
	// source site instead of the render origin.
	withBase := `<html><head><base href="` + srv.URL + `"><title>Fetched Tool</title></head><body><h1>hi</h1></body></html>`
	assert.Equal(t, withBase, getArtifactBody(t, r, id))
}

func TestCreateArtifactFromURLRecordsSourceURL(t *testing.T) {
	r := newTestRouter(t)

	const page = `<html><head><title>Fetched Tool</title></head><body><h1>hi</h1></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, page)
	}))
	defer srv.Close()

	id := createArtifact(t, r, map[string]any{"url": srv.URL, "network_allowlist": []string{}})

	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var art map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	// The source URL is recorded and returned by the API.
	assert.Equal(t, srv.URL, art["source_url"])
}

func TestCreateArtifactFromBodyHasEmptySourceURL(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Pasted",
		"body":              "<html><body>pasted</body></html>",
		"network_allowlist": []string{},
	})

	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var art map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	// Paste-based creation leaves the source URL empty.
	assert.Equal(t, "", art["source_url"])
}

func TestCreateArtifactFromURLTitleFallback(t *testing.T) {
	r := newTestRouter(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `<html><body>no title element</body></html>`)
	}))
	defer srv.Close()

	id := createArtifact(t, r, map[string]any{"url": srv.URL, "network_allowlist": []string{}})

	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var art map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	// No <title> and no provided title → falls back to the URL itself.
	assert.Equal(t, srv.URL, art["title"])
}

func TestCreateArtifactExplicitTitleBeatsURL(t *testing.T) {
	r := newTestRouter(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `<html><head><title>Page Title</title></head></html>`)
	}))
	defer srv.Close()

	id := createArtifact(t, r, map[string]any{"url": srv.URL, "title": "Caller Title", "network_allowlist": []string{}})

	req := httptest.NewRequest("GET", "/api/artifacts/"+id, nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var art map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&art))
	assert.Equal(t, "Caller Title", art["title"])
}

func TestCreateArtifactRequiresBodyOrURL(t *testing.T) {
	r := newTestRouter(t)

	b, _ := json.Marshal(map[string]any{"title": "Empty"})
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "body or url is required")
}

// createArtifactResp POSTs an artifact and returns the decoded create response.
func createArtifactResp(t *testing.T, r *Router, payload map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/artifacts", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

func TestCreateArtifactDoesNotSeedAllowlistFromScan(t *testing.T) {
	r := newTestRouter(t)

	body := `<html><head><script src="https://cdn.jsdelivr.net/npm/chart.js"></script></head><body></body></html>`
	resp := createArtifactResp(t, r, map[string]any{
		"title":             "Charty",
		"body":              body,
		"network_allowlist": []string{},
	})

	// The scanned footprint is surfaced to the caller as transparency...
	assert.Contains(t, resp["network_footprint"], "https://cdn.jsdelivr.net")
	// ...but is NOT auto-approved: the allowlist stays empty until the user
	// explicitly approves, so the render CSP stays connect-src 'none'.
	art := resp["artifact"].(map[string]any)
	assert.Empty(t, art["network_allowlist"])
}

func TestCreateArtifactExplicitAllowlistWins(t *testing.T) {
	r := newTestRouter(t)

	// Body references a CDN, but the caller supplies an explicit allowlist:
	// the explicit list must win over the scan.
	body := `<html><head><script src="https://cdn.jsdelivr.net/npm/chart.js"></script></head></html>`
	resp := createArtifactResp(t, r, map[string]any{
		"title":             "Explicit",
		"body":              body,
		"network_allowlist": []string{"https://example.com"},
	})

	art := resp["artifact"].(map[string]any)
	assert.Equal(t, []any{"https://example.com"}, art["network_allowlist"])
}

func TestCreateArtifactFromURLDoesNotSeedAllowlist(t *testing.T) {
	r := newTestRouter(t)

	const page = `<html><head><title>Fetcher</title><script src="https://cdn.jsdelivr.net/npm/x"></script></head><body></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, page)
	}))
	defer srv.Close()

	resp := createArtifactResp(t, r, map[string]any{"url": srv.URL, "network_allowlist": []string{}})
	// The origin is surfaced as footprint but must not be auto-approved.
	assert.Contains(t, resp["network_footprint"], "https://cdn.jsdelivr.net")
	art := resp["artifact"].(map[string]any)
	assert.Empty(t, art["network_allowlist"])
}

func TestPatchArtifactBodyDoesNotAddScannedOrigins(t *testing.T) {
	r := newTestRouter(t)

	// Start with a no-network artifact.
	id := createArtifact(t, r, map[string]any{
		"title":             "Plain",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})

	// PATCH a new body that references an external CDN.
	newBody := `<html><head><script src="https://cdn.jsdelivr.net/npm/chart.js"></script></head><body></body></html>`
	patch := map[string]any{"body": newBody}
	pb, _ := json.Marshal(patch)
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(pb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	art := updated["artifact"].(map[string]any)
	// Editing the body must NOT silently grant network access: the newly
	// scanned origin stays out of the allowlist until the user approves it.
	assert.Empty(t, art["network_allowlist"])

	// The edit re-scanned the body (it differs from the previous no-network
	// version) and surfaced the footprint plus the change flag so the edit
	// dialog can re-run the explicit-approval flow — without seeding allowlist.
	assert.Contains(t, updated["network_footprint"], "https://cdn.jsdelivr.net")
	assert.True(t, updated["footprint_changed"].(bool))

	// The blob body is overwritten with the new content.
	assert.Equal(t, newBody, getArtifactBody(t, r, id))
}

func TestPatchArtifactBodyKeepsExplicitAllowlist(t *testing.T) {
	r := newTestRouter(t)

	id := createArtifact(t, r, map[string]any{
		"title":             "Plain",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})

	// Body references a CDN, but the caller also supplies an explicit allowlist:
	// the explicit list must win over the re-scan.
	newBody := `<html><head><script src="https://cdn.jsdelivr.net/npm/chart.js"></script></head></html>`
	patch := map[string]any{"body": newBody, "network_allowlist": []string{"https://example.com"}}
	pb, _ := json.Marshal(patch)
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(pb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	art := updated["artifact"].(map[string]any)
	assert.Equal(t, []any{"https://example.com"}, art["network_allowlist"])
}

func TestPatchArtifactEmptyBodyIgnored(t *testing.T) {
	r := newTestRouter(t)

	original := "<html><body>keep me</body></html>"
	id := createArtifact(t, r, map[string]any{
		"title":             "Plain",
		"body":              original,
		"network_allowlist": []string{},
	})

	// An empty body must not wipe the stored blob.
	patch := map[string]any{"title": "Renamed", "body": ""}
	pb, _ := json.Marshal(patch)
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(pb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	art := updated["artifact"].(map[string]any)
	assert.Equal(t, "Renamed", art["title"])
	assert.Equal(t, original, getArtifactBody(t, r, id))
}

func TestPatchArtifactUnchangedBodyReportsNoChange(t *testing.T) {
	r := newTestRouter(t)

	original := `<html><head><script src="https://cdn.jsdelivr.net/npm/x"></script></head><body>hi</body></html>`
	id := createArtifact(t, r, map[string]any{
		"title":             "Plain",
		"body":              original,
		"network_allowlist": []string{},
	})

	// PATCH the same body back — no diff, so the scan must not report a change.
	patch := map[string]any{"body": original}
	pb, _ := json.Marshal(patch)
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(pb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.False(t, updated["footprint_changed"].(bool), "identical body must not report a footprint change")
	assert.Empty(t, updated["network_footprint"])
}

func TestPatchArtifactSameFootprintReportsNoChange(t *testing.T) {
	r := newTestRouter(t)

	// Two different bodies that contact the SAME origin — the content diffs
	// but the network footprint does not, so the scan re-runs yet reports no
	// change (no re-approval prompt).
	first := `<html><head><script src="https://cdn.jsdelivr.net/npm/a"></script></head><body>v1</body></html>`
	second := `<html><head><script src="https://cdn.jsdelivr.net/npm/b"></script></head><body>v2</body></html>`
	id := createArtifact(t, r, map[string]any{
		"title":             "Plain",
		"body":              first,
		"network_allowlist": []string{},
	})

	patch := map[string]any{"body": second}
	pb, _ := json.Marshal(patch)
	req := httptest.NewRequest("PATCH", "/api/artifacts/"+id, bytes.NewReader(pb))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var updated map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	assert.False(t, updated["footprint_changed"].(bool), "same origin set must not report a footprint change")
	assert.Contains(t, updated["network_footprint"], "https://cdn.jsdelivr.net")
	assert.Equal(t, second, getArtifactBody(t, r, id))
}

// patchBodyFootprint PATCHes a new source body and decodes the footprint half
// of the update response.
func patchBodyFootprint(t *testing.T, r *Router, id, body string) updateArtifactResponse {
	t.Helper()
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"body": body})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp updateArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// ingestFootprint creates an artifact and returns its id and the footprint
// ingest reported.
func ingestFootprint(t *testing.T, r *Router, payload map[string]any) (string, []string) {
	t.Helper()
	resp := createArtifactResp(t, r, payload)
	footprint := []string{}
	for _, o := range resp["network_footprint"].([]any) {
		footprint = append(footprint, o.(string))
	}
	return resp["artifact"].(map[string]any)["id"].(string), footprint
}

// TestPatchURLEditKeepsIngestFootprint is the av-wu9d acceptance pin: a
// trivial edit to the body a URL ingest actually stored reports the footprint
// ingest reported, exactly, with no change flagged. Editing the stored bytes
// rather than a hand-written copy is the point: the injected base is whatever
// InjectBaseHref produced.
func TestPatchURLEditKeepsIngestFootprint(t *testing.T) {
	r := newTestRouter(t)

	first := `<html><head><title>t</title></head><body><h1>hello</h1>` +
		`<img src="/images/logo.png"><script src="https://cdn.other.com/lib.js"></script>` +
		`<script>fetch('/api/data')</script></body></html>`
	id, ingested := ingestFootprint(t, r, map[string]any{
		"title": "wu9d", "url": "https://example.com/blog/post", "body": first, "network_allowlist": []string{},
	})
	assert.ElementsMatch(t, []string{"https://example.com", "https://cdn.other.com"}, ingested)

	stored := getArtifactBody(t, r, id)
	edited := strings.Replace(stored, "<h1>hello</h1>", "<h1>hello edited</h1>", 1)
	require.NotEqual(t, stored, edited)

	updated := patchBodyFootprint(t, r, id, edited)
	assert.ElementsMatch(t, ingested, updated.NetworkFootprint)
	assert.False(t, updated.FootprintChanged)
}

// A fetched page that declares its own <base> keeps it (InjectBaseHref skips
// injection), so its relatives reach that base's origin and not the page URL.
// Ingest used to resolve them against the page URL and drop the base tag's
// origin, asking the user to approve a host the artifact never contacts while
// CSP blocked the one it does. (av-wu9d review)
func TestURLIngestFollowsThePagesOwnBase(t *testing.T) {
	r := newTestRouter(t)

	page := `<html><head><base href="https://cdn.site/app/"></head><body><h1>hi</h1><img src="logo.png"></body></html>`
	id, ingested := ingestFootprint(t, r, map[string]any{
		"title": "own base", "url": "https://example.com/page", "body": page, "network_allowlist": []string{},
	})
	assert.Equal(t, []string{"https://cdn.site"}, ingested)

	stored := getArtifactBody(t, r, id)
	updated := patchBodyFootprint(t, r, id, strings.Replace(stored, "<h1>hi</h1>", "<h1>hey</h1>", 1))
	assert.Equal(t, ingested, updated.NetworkFootprint, "an edit must see what ingest saw")
	assert.False(t, updated.FootprintChanged)
}

// A pasted document's own <base> governs its relatives too. Before, paste
// ingest ran a scan that drops relatives, and once the base tag stopped being
// reported the footprint came back empty: nothing to approve at the door.
func TestPasteIngestFollowsTheDocumentsOwnBase(t *testing.T) {
	r := newTestRouter(t)

	_, ingested := ingestFootprint(t, r, map[string]any{
		"title": "pasted base", "network_allowlist": []string{},
		"body": `<base href="https://x.com/"><img src="a.png"><script>fetch('api')</script>`,
	})
	assert.Equal(t, []string{"https://x.com"}, ingested)
}

// TestPatchDetectsRelativeChangeUnderStableBase covers the case plain Scan
// misses while the injected base is preserved on both sides: adding or
// removing a relative changes what the page contacts, so the gate must fire.
func TestPatchDetectsRelativeChangeUnderStableBase(t *testing.T) {
	r := newTestRouter(t)

	const sourceURL = "https://example.com/blog/post"
	base := `<base href="` + sourceURL + `">`
	plain := `<html><head>` + base + `<title>t</title></head><body><h1>hi</h1></body></html>`
	withImg := `<html><head>` + base + `<title>t</title></head><body><h1>hi</h1><img src="/new.png"></body></html>`
	id := createArtifact(t, r, map[string]any{
		"title": "wu9d", "url": sourceURL, "body": plain, "network_allowlist": []string{},
	})

	// Adding a relative under the stable base introduces the source origin.
	added := patchBodyFootprint(t, r, id, withImg)
	assert.Equal(t, []string{"https://example.com"}, added.NetworkFootprint)
	assert.True(t, added.FootprintChanged, "added relative must report a footprint change")

	// Removing it again drops the origin.
	removed := patchBodyFootprint(t, r, id, plain)
	assert.Empty(t, removed.NetworkFootprint)
	assert.True(t, removed.FootprintChanged, "removed relative must report a footprint change")
}

// TestPatchDroppedBaseResolvesLocally pins the Exhibit-namespace rule: once
// the author deletes the fallback tag, surviving relatives are local paths
// (render origin), not source contacts, so the scan must not attribute them
// to the source site.
func TestPatchDroppedBaseResolvesLocally(t *testing.T) {
	r := newTestRouter(t)

	const sourceURL = "https://example.com/blog/post"
	withBase := `<html><head><base href="` + sourceURL + `"><title>t</title></head><body><img src="/a.png"></body></html>`
	noBase := `<html><head><title>t</title></head><body><img src="/a.png"></body></html>`
	id := createArtifact(t, r, map[string]any{
		"title": "wu9d", "url": sourceURL, "body": withBase, "network_allowlist": []string{},
	})

	updated := patchBodyFootprint(t, r, id, noBase)
	assert.Empty(t, updated.NetworkFootprint)
	assert.True(t, updated.FootprintChanged)
}

// A body whose bytes are gone has no baseline to diff, and the rewrite is the
// only way to repair the artifact. So the PATCH goes through, the bundled
// title applies, and the unknown comparison reports a change so the approval
// gate still runs. Refusing it instead left every body PATCH on that artifact
// answering 500 forever. (av-wu9d review)
func TestPatchMissingOldBodyRepairsTheArtifact(t *testing.T) {
	r, blobDir := newTestRouterWithBlobDir(t)

	id := createArtifact(t, r, map[string]any{
		"title": "wu9d", "body": "<html><body>original</body></html>", "network_allowlist": []string{},
	})
	var blobID string
	require.NoError(t, json.Unmarshal([]byte(artifactField(t, r, id, "source_blob_id")), &blobID))
	require.NoError(t, os.Remove(filepath.Join(blobDir, blobID)))

	repaired := `<html><body>new<script src="https://cdn.example.com/a.js"></script></body></html>`
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id, map[string]any{"title": "renamed", "body": repaired})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp updateArtifactResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	assert.Equal(t, []string{"https://cdn.example.com"}, resp.NetworkFootprint)
	assert.True(t, resp.FootprintChanged, "an unknown baseline must run the approval gate")
	assert.Equal(t, repaired, getArtifactBody(t, r, id))
	assert.Equal(t, `"renamed"`, artifactField(t, r, id, "title"))
}

// failingGets is a blob store whose Get fails the way an outage does, with an
// error that is not fs.ErrNotExist, and which counts Puts so a test can show
// nothing was written after the failure.
type failingGets struct {
	blob.Store
	puts int
}

func (f *failingGets) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("blob backend unavailable")
}

func (f *failingGets) Put(ctx context.Context, id string, body io.Reader) error {
	f.puts++
	return f.Store.Put(ctx, id, body)
}

// A read that fails for any other reason aborts the PATCH before anything is
// written: not the body, not the title bundled with it. A silent empty
// baseline would report a phantom diff against a body the store could not
// show us, and overwrite it. (av-wu9d)
func TestPatchUnreadableOldBodyWritesNothing(t *testing.T) {
	r := newTestRouter(t)

	const original = "<html><body>original</body></html>"
	id := createArtifact(t, r, map[string]any{
		"title": "wu9d", "body": original, "network_allowlist": []string{},
	})

	healthy := r.cfg.Blob
	failing := &failingGets{Store: healthy}
	r.cfg.Blob = failing
	w := doJSON(t, r, "PATCH", "/api/artifacts/"+id,
		map[string]any{"title": "renamed", "body": "<html><body>new</body></html>"})
	r.cfg.Blob = healthy

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Zero(t, failing.puts, "no blob may be written once the baseline read failed")
	assert.Equal(t, original, getArtifactBody(t, r, id))
	assert.Equal(t, `"wu9d"`, artifactField(t, r, id, "title"))
}

func TestRefetchArtifactOverwritesBody(t *testing.T) {
	r := newTestRouter(t)

	// The upstream page changes between create and refetch.
	page := `<html><head><title>v1</title></head><body><h1>first</h1></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, page)
	}))
	defer srv.Close()

	id := createArtifact(t, r, map[string]any{"url": srv.URL, "network_allowlist": []string{}})
	// URL ingest injects the <base href> fallback into the stored body.
	require.Equal(t,
		`<html><head><base href="`+srv.URL+`"><title>v1</title></head><body><h1>first</h1></body></html>`,
		getArtifactBody(t, r, id))

	// Upstream now serves new content that also references an external origin.
	page = `<html><head><script src="https://cdn.jsdelivr.net/npm/chart.js"></script></head><body><h1>second</h1></body></html>`

	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/refetch", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&updated))
	// The stored body is overwritten with the fresh snapshot.
	assert.Equal(t, page, getArtifactBody(t, r, id))
	// The allowlist is re-scanned from the new content.
	assert.Contains(t, updated["network_allowlist"], "https://cdn.jsdelivr.net")
}

func TestRefetchArtifactWithoutSourceURL(t *testing.T) {
	r := newTestRouter(t)

	// Paste-created artifact has no source URL.
	id := createArtifact(t, r, map[string]any{
		"title":             "Pasted",
		"body":              "<html><body>pasted</body></html>",
		"network_allowlist": []string{},
	})

	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/refetch", nil)
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "no source URL")
}

// htmlEsc mirrors the escaping html/template applies to the characters this
// file's fixtures contain (it does not escape ', which html/template does —
// keep fixtures apostrophe-free or the expectation diverges).
func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}

func TestGalleryEditPage(t *testing.T) {
	r := newTestRouter(t)

	body := `<html><head><title>Editable</title></head><body>content</body></html>`
	id := createArtifact(t, r, map[string]any{
		"title":             "Editable",
		"body":              body,
		"network_allowlist": []string{},
	})

	req := httptest.NewRequest("GET", "/artifacts/"+id+"/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")

	page := w.Body.String()
	// The edit page pre-fills the title and the (HTML-escaped) source body.
	assert.Contains(t, page, `value="Editable"`)
	assert.Contains(t, page, htmlEsc(body))
}

func TestGalleryEditPageNotFound(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest("GET", "/artifacts/does-not-exist/edit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
