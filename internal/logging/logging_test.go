package logging

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net"
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

func TestRequestMiddlewareLogsRecoveredPanicsAs500(t *testing.T) {
	// When RequestMiddleware is outermost and an inner recoverer writes a 500,
	// the structured request log must still see the 500 status.
	buf := captureLogger(t, slog.LevelInfo)

	recoverer := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recover() != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}

	h := RequestMiddleware(recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifacts", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	out := buf.String()
	assert.Contains(t, out, "level=ERROR")
	assert.Contains(t, out, "status=500")
	assert.Contains(t, out, "path=/api/artifacts")
}

func TestStatusWriterWriteOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}
	sw.WriteHeader(http.StatusOK)
	sw.WriteHeader(http.StatusInternalServerError) // must be ignored
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, http.StatusOK, sw.status)
}

func TestStatusWriterDefaultsToOKWhenHandlerDoesNotWrite(t *testing.T) {
	buf := captureLogger(t, slog.LevelInfo)

	h := RequestMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// handler neither calls WriteHeader nor Write
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, buf.String(), "status=200")
}

func TestStatusWriterPreservesFlusher(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}

	f, ok := http.ResponseWriter(sw).(http.Flusher)
	assert.True(t, ok, "statusWriter must implement http.Flusher")
	assert.NotPanics(t, func() { f.Flush() })
}

// mockOptionalWriter implements http.ResponseWriter plus http.Flusher,
// http.Hijacker, and http.Pusher so we can assert statusWriter delegates
// optional interfaces to the underlying writer.
type mockOptionalWriter struct {
	*httptest.ResponseRecorder
	flushed  bool
	hijacked bool
	pushed   bool
}

func (m *mockOptionalWriter) Flush() { m.flushed = true }
func (m *mockOptionalWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijacked = true
	return nil, nil, nil
}
func (m *mockOptionalWriter) Push(target string, _ *http.PushOptions) error {
	m.pushed = true
	_ = target
	return nil
}

func TestStatusWriterPreservesOptionalInterfaces(t *testing.T) {
	mock := &mockOptionalWriter{ResponseRecorder: httptest.NewRecorder()}
	sw := &statusWriter{ResponseWriter: mock}

	f, ok := http.ResponseWriter(sw).(http.Flusher)
	require.True(t, ok)
	f.Flush()
	assert.True(t, mock.flushed)

	h, ok := http.ResponseWriter(sw).(http.Hijacker)
	require.True(t, ok)
	_, _, err := h.Hijack()
	assert.NoError(t, err)
	assert.True(t, mock.hijacked)

	p, ok := http.ResponseWriter(sw).(http.Pusher)
	require.True(t, ok)
	assert.NoError(t, p.Push("/x", nil))
	assert.True(t, mock.pushed)
}
