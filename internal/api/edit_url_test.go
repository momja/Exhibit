package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
