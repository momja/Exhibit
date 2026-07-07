package logging

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"  info  ", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"bogus", slog.LevelInfo},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, ParseLevel(c.in), "input %q", c.in)
	}
}

// captureLogger swaps in a TextHandler writing to a buffer at the given level
// and restores the previous default on cleanup. Records are returned as a
// slice of raw text lines for substring assertions.
func captureLogger(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	return &buf
}

func TestDebugEnabled(t *testing.T) {
	t.Run("off at info", func(t *testing.T) {
		captureLogger(t, slog.LevelInfo)
		assert.False(t, DebugEnabled())
	})
	t.Run("on at debug", func(t *testing.T) {
		captureLogger(t, slog.LevelDebug)
		assert.True(t, DebugEnabled())
	})
}

func TestRequestMiddlewareInfo(t *testing.T) {
	captureLogger(t, slog.LevelInfo)

	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/abc?q=z", nil)
	req.Header.Set("X-Request-ID", "rid-1")
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Equal(t, "hi", rec.Body.String())
}

func TestRequestMiddlewareLogsStatusAndDuration(t *testing.T) {
	buf := captureLogger(t, slog.LevelInfo)

	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/a/xyz", nil)
	h.ServeHTTP(rec, req)

	out := buf.String()
	assert.Contains(t, out, "level=INFO")
	assert.Contains(t, out, "msg=request")
	assert.Contains(t, out, "method=GET")
	assert.Contains(t, out, "path=/a/xyz")
	assert.Contains(t, out, "status=200")
	assert.Contains(t, out, "duration=")
}

func TestRequestMiddlewareDebugAddsDetail(t *testing.T) {
	buf := captureLogger(t, slog.LevelDebug)

	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts?q=foo&tags=a,b", nil)
	req.Header.Set("X-Request-ID", "rid-9")
	req.RemoteAddr = "1.2.3.4:5555"
	h.ServeHTTP(rec, req)

	out := buf.String()
	assert.Contains(t, out, "level=DEBUG")
	assert.Contains(t, out, "remote=1.2.3.4:5555")
	assert.Contains(t, out, "bytes=7")
	assert.Contains(t, out, `query="q=foo&tags=a,b"`)
	assert.Contains(t, out, "request_id=rid-9")
}

func TestRequestMiddlewarePromotes5xxToError(t *testing.T) {
	buf := captureLogger(t, slog.LevelInfo)

	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts", nil)
	h.ServeHTTP(rec, req)

	out := buf.String()
	assert.Contains(t, out, "level=ERROR")
	assert.Contains(t, out, "status=500")
}

func TestRequestMiddlewarePromotes4xxToWarn(t *testing.T) {
	buf := captureLogger(t, slog.LevelInfo)

	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusNotFound)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/missing", nil)
	h.ServeHTTP(rec, req)

	out := buf.String()
	assert.Contains(t, out, "level=WARN")
	assert.Contains(t, out, "status=404")
}

func TestRequestMiddlewareSkippedBelowInfo(t *testing.T) {
	// At warn level the default info-level request line is suppressed, but a
	// 5xx must still surface as an error.
	buf := captureLogger(t, slog.LevelWarn)

	ok := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Empty(t, buf.String(), "200 below warn should not log")

	fail := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	rec = httptest.NewRecorder()
	fail.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Contains(t, buf.String(), "level=ERROR")
}

func TestStatusWriterDefaultsToOK(t *testing.T) {
	// A handler that only writes a body (no WriteHeader) must record 200.
	captureLogger(t, slog.LevelInfo)
	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestMiddlewareUsesRequestContext(t *testing.T) {
	// The middleware passes r.Context() to slog; a cancelled context must not
	// panic and the handler must still run.
	buf := captureLogger(t, slog.LevelInfo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := RequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
	require.True(t, strings.Contains(buf.String(), "status=201"))
}
