package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogs swaps the default slog logger for a TextHandler writing to a
// buffer at the given level, restoring the prior default on cleanup. It
// returns the buffer to read the emitted records back.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	return &buf
}

func TestRequestLoggingWired(t *testing.T) {
	r := newTestRouter(t)

	t.Run("info logs each request and promotes 404 to warn", func(t *testing.T) {
		buf := captureLogs(t, slog.LevelInfo)

		req := httptest.NewRequest(http.MethodGet, "/api/artifacts/does-not-exist", nil)
		req.Header.Set("Authorization", authHeader())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)

		out := buf.String()
		assert.Contains(t, out, "msg=request")
		assert.Contains(t, out, "path=/api/artifacts/does-not-exist")
		assert.Contains(t, out, "status=404")
		assert.Contains(t, out, "level=WARN")
	})

	t.Run("debug adds remote, bytes, query, request_id", func(t *testing.T) {
		buf := captureLogs(t, slog.LevelDebug)

		req := httptest.NewRequest(http.MethodGet, "/api/artifacts?limit=5&q=hi", nil)
		req.Header.Set("Authorization", authHeader())
		req.Header.Set("X-Request-ID", "trace-7")
		req.RemoteAddr = "9.9.9.9:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		out := buf.String()
		assert.Contains(t, out, "level=DEBUG")
		assert.Contains(t, out, "remote=9.9.9.9:1234")
		assert.Contains(t, out, "query=")
		assert.Contains(t, out, "request_id=trace-7")
	})

	t.Run("successful ingest logs artifact created at debug", func(t *testing.T) {
		buf := captureLogs(t, slog.LevelDebug)

		body := map[string]any{
			"title":             "Logged Artifact",
			"body":              "<html><body>hi</body></html>",
			"network_allowlist": []string{},
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/artifacts", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader())
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		out := buf.String()
		assert.Contains(t, out, "msg=\"artifact created\"")
		assert.Contains(t, out, "title=\"Logged Artifact\"")
		assert.Contains(t, out, "body_bytes=")
	})

	t.Run("info mode does not emit debug artifact-created trace", func(t *testing.T) {
		buf := captureLogs(t, slog.LevelInfo)

		body := map[string]any{
			"title":             "Quiet Artifact",
			"body":              "<html></html>",
			"network_allowlist": []string{},
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/artifacts", bytes.NewReader(b))
		req.Header.Set("Authorization", authHeader())
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		assert.False(t, strings.Contains(buf.String(), "artifact created"),
			"debug trace should be suppressed at info level; got: %s", buf.String())
		assert.Contains(t, buf.String(), "msg=request") // but the request line is present
	})
}
