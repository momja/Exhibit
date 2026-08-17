package api

// Response compression (av-f9b2). The render document is the payload that
// matters: it is composed per request and served no-store, so without
// compression every view pays its full size over the wire.

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gzipGet issues a request advertising gzip support and reports whether the
// response came back compressed, along with the decoded body.
func gzipGet(t *testing.T, h http.Handler, path string) (encoding string, body string, code int) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	encoding = w.Header().Get("Content-Encoding")
	code = w.Code
	if encoding == "gzip" {
		zr, err := gzip.NewReader(w.Body)
		require.NoError(t, err, "response claimed gzip but is not readable as gzip")
		defer zr.Close()
		raw, err := io.ReadAll(zr)
		require.NoError(t, err)
		return encoding, string(raw), code
	}
	return encoding, w.Body.String(), code
}

func TestRenderDocumentIsCompressed(t *testing.T) {
	r := newTestRouter(t)
	// A body with enough repetition to make the win unambiguous.
	body := "<html><head><title>Compressible</title></head><body>" +
		strings.Repeat("<p>the quick brown fox jumps over the lazy dog</p>", 400) +
		"</body></html>"

	w, resp := postArtifact(t, r, map[string]any{"title": "Compressible", "body": body})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	renderRouter := r.RenderHandler()
	tok := r.tokens.Mint(resp.Artifact.ID, defaultOwnerID)
	renderPath := "/a/" + resp.Artifact.ID + "?" + rendertoken.Param + "=" + tok
	enc, got, code := gzipGet(t, renderRouter, renderPath)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "gzip", enc, "render documents must be compressed")
	// Round-trips to the real document, not a truncated or corrupted stream.
	assert.Contains(t, got, "the quick brown fox")
	assert.Contains(t, got, "<title>Compressible</title>")

	// And the compressed transfer is materially smaller than the raw document.
	req := httptest.NewRequest("GET", renderPath, nil)
	plain := httptest.NewRecorder()
	renderRouter.ServeHTTP(plain, req)
	require.Equal(t, http.StatusOK, plain.Code)
	assert.Empty(t, plain.Header().Get("Content-Encoding"), "no encoding when the client does not ask")

	compressedReq := httptest.NewRequest("GET", renderPath, nil)
	compressedReq.Header.Set("Accept-Encoding", "gzip")
	compressed := httptest.NewRecorder()
	renderRouter.ServeHTTP(compressed, compressedReq)
	assert.Less(t, compressed.Body.Len(), plain.Body.Len()/2,
		"compressed render document should be well under half the raw size")
}

// The response body now depends on a request header, so any cache in front of
// the service has to key on it. Without Vary, a shared cache can hand a
// gzip-encoded body to a client that never asked for one — which arrives as
// binary garbage, not as a slow page. The render surface is no-store, but the
// app origin's pages and assets are not, and the operator's proxy is theirs to
// choose (technical_stack.md §12), so this cannot rest on our own caching
// choices.
func TestNegotiatedResponsesCarryVary(t *testing.T) {
	r := newTestRouter(t)

	for _, tc := range []struct {
		name    string
		handler http.Handler
		path    string
	}{
		{"app page", r, "/"},
		{"render document", r.RenderHandler(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if path == "" {
				w, resp := postArtifact(t, r, map[string]any{
					"title": "vary", "body": "<html><body>" + strings.Repeat("x", 2048) + "</body></html>",
				})
				require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
				tok := r.tokens.Mint(resp.Artifact.ID, defaultOwnerID)
				path = "/a/" + resp.Artifact.ID + "?" + rendertoken.Param + "=" + tok
			}

			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			req.Header.Set("Authorization", authHeader())
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "gzip", rec.Header().Get("Content-Encoding"), "expected this response to be compressed")
			assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding",
				"a content-negotiated response must tell caches it varies on the request header")
		})
	}
}

// SSE must never be compressed: the agent surface streams events and a
// buffering encoder would stall the stream. text/event-stream is deliberately
// absent from compressibleTypes, and this pins that.
func TestEventStreamIsNotCompressed(t *testing.T) {
	assert.NotContains(t, compressibleTypes, "text/event-stream",
		"compressing SSE would stall the agent event stream")

	// Drive the middleware over a handler that streams like the SSE route does,
	// including the http.Flusher assertion agent.go depends on.
	h := compressor()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: one\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: two\n\n")
		flusher.Flush()
	}))

	req := httptest.NewRequest("GET", "/api/agent/sessions/x/events", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "the Flusher assertion must still succeed through the compressor")
	assert.Empty(t, w.Header().Get("Content-Encoding"), "SSE must be served uncompressed")
	assert.Equal(t, "data: one\n\ndata: two\n\n", w.Body.String())
}

// Already-compressed payloads gain nothing from a second pass.
func TestBinaryTypesAreNotCompressed(t *testing.T) {
	for _, ct := range []string{"image/png", "font/woff2", "application/wasm", "application/octet-stream"} {
		assert.NotContains(t, compressibleTypes, ct, "%s is already compressed", ct)
	}
}

func TestGalleryPageIsCompressed(t *testing.T) {
	r := newTestRouter(t)
	enc, body, code := gzipGet(t, r, "/")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "gzip", enc)
	assert.Contains(t, strings.ToLower(body), "<html")
}

// acceptsGzip is the negotiation the whole compressor turns on, so it gets
// its own table rather than relying only on end-to-end requests: chi's
// middleware.Compress matched Accept-Encoding with strings.Contains, which
// cannot tell "gzip;q=0" (explicitly refused) from bare "gzip".
func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"GZIP", true},
		{"gzip;q=0", false},
		{"gzip;q=0.0", false},
		{"gzip;q=0.001", true},
		{"gzip;q=1.0", true},
		{"deflate", false},
		{"deflate, br", false},
		{"*", true},
		{"*;q=0", false},
		{"identity", false},
		{"gzip;q=0, *;q=1", false},
		{"*;q=0, gzip;q=0.5", true},
		{"deflate;q=1, gzip;q=0.3", true},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			assert.Equal(t, tc.want, acceptsGzip(tc.header))
		})
	}
}

// A client that explicitly refuses gzip (q=0) must be served uncompressed —
// the bug this regression pins is a substring match treating "gzip;q=0" as
// if it were "gzip".
func TestGzipQZeroIsNotCompressed(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip;q=0, deflate")
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"), "gzip;q=0 must be honored as \"not acceptable\"")
}

// The compressor is gzip-only by design (technical_stack.md §1) — a client
// offering only deflate must not be compressed with it.
func TestDeflateOnlyIsNotCompressed(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "deflate")
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"), "gzip-only negotiation must not fall back to deflate")
}
