package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

	// Fetched content is stored verbatim as the artifact body.
	assert.Equal(t, page, getArtifactBody(t, r, id))
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

func TestPatchArtifactBodyRescansAllowlist(t *testing.T) {
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
	assert.Contains(t, updated["network_allowlist"], "https://cdn.jsdelivr.net")

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
	assert.Equal(t, []any{"https://example.com"}, updated["network_allowlist"])
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
	assert.Equal(t, "Renamed", updated["title"])
	assert.Equal(t, original, getArtifactBody(t, r, id))
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
