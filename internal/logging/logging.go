// Package logging configures the application's structured logger and provides
// the HTTP request-logging middleware.
//
// Logging is leveled through log/slog so the service stays quiet in production
// (info) and verbose in test environments (debug). Debug mode is enabled by
// setting LOG_LEVEL=debug or DEBUG=1; the request middleware then emits the
// extra per-request detail (remote address, bytes, query, request id) that a
// developer needs to diagnose behavior in test environments. See ticket
// av-11qx.
package logging

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// ParseLevel maps a level name (debug/info/warn/error) to an slog.Level.
// Unknown values fall back to Info so a typo never silences the service.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// DebugEnabled reports whether the default logger will emit debug-level
// records — i.e. verbose developer feedback is on. Callers that build
// expensive debug values guard on this to avoid the cost when debug mode
// is off.
func DebugEnabled() bool { return slog.Default().Enabled(nil, slog.LevelDebug) }

// Configure sets the default slog logger to write stdout at the given level
// and returns it. A single TextHandler keeps output greppable and free of
// external dependencies, matching the self-contained deployment stance.
func Configure(level slog.Level) *slog.Logger {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

// statusWriter wraps http.ResponseWriter to capture the status code and body
// size for request logging. It is write-once: the first WriteHeader wins and
// an implicit 200 is recorded on the first Write if WriteHeader was skipped.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// RequestMiddleware logs every HTTP request through the default slog logger.
//
// At info level it records one line per request: method, path, status,
// duration. At debug level it adds the remote address, response size, the
// raw query, and any client-supplied X-Request-ID — the rich feedback a
// developer wants in test environments. Server errors (5xx) are always
// promoted to error level and client errors (4xx) to warn, so a noisy log
// still surfaces real problems without requiring debug mode.
func RequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Duration("duration", duration),
		}
		level := slog.LevelInfo
		if DebugEnabled() {
			level = slog.LevelDebug
			attrs = append(attrs,
				slog.String("remote", r.RemoteAddr),
				slog.Int("bytes", sw.bytes),
				slog.String("query", r.URL.RawQuery),
				slog.String("request_id", r.Header.Get("X-Request-ID")),
			)
		}
		switch {
		case sw.status >= http.StatusInternalServerError:
			level = slog.LevelError
		case sw.status >= http.StatusBadRequest && level < slog.LevelWarn:
			level = slog.LevelWarn
		}
		slog.LogAttrs(r.Context(), level, "request", attrs...)
	})
}
