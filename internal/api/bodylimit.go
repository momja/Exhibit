// Request body limit (av-ombn).
//
// Every request the app surface answers carries a body of at most
// MAX_REQUEST_BODY_BYTES. One number for every route rather than one per
// route. The routes that matter are the document writes (POST and PATCH of
// an artifact, PUT of a widget), and every other route takes a JSON object of
// a few fields, for which a separate smaller ceiling would buy nothing the
// document routes do not already concede.
//
// # Why 32 MiB
//
// The largest legitimate body measured is ~16.3 MB: a snapshot that vendored a
// wasm runtime, exported to one file (av-vnkt) and written back. Since av-20fk
// a URL ingest keeps those payloads out of line and never sends them in a
// request at all, but a paste of the exported file still does. 32 MiB is about
// twice that, and the doubling is not padding: JSON escaping is paid on top of
// the document, and a Go client escapes `<`, `>` and `&` to six bytes each, so
// a markup-heavy 16 MiB document arrives as a 30 MiB request (measured).
//
// It is no larger because of what a body costs once it is in. A write holds
// the document as the decoded string, a []byte copy at Blob.Put and three
// parse trees (title, footprint scan, search text), and peaks at about ten
// times the request (measured: 160 MiB for a 16 MiB write, 315 MiB for a
// 30 MiB one). At the default that is ~320 MiB for one maximal write, which
// the 1 GB the smallest documented deployment provisions (fly.toml) can hold
// beside an agent sidecar. An operator on a smaller machine lowers it.
//
// # Where it is enforced
//
// Twice, because a client either declares its length or does not. A declared
// Content-Length over the limit is answered 413 by limitRequestBody before
// any handler runs, and the body is never read: not one byte, and a client
// that sent `Expect: 100-continue` (curl does, above 1 MiB) never sends it.
// An undeclared length (chunked) is wrapped in http.MaxBytesReader, which
// stops reading one byte past the limit; the handler's decode then fails,
// and decodeJSON answers 413 rather than the 400 a malformed body gets.
// decodeJSON is the only place in this package allowed to read r.Body, and
// bodylimit_test.go walks the package's AST to keep it that way.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/momja/Exhibit/internal/humanize"
)

const envMaxRequestBodyBytes = "MAX_REQUEST_BODY_BYTES"

// DefaultMaxRequestBodyBytes is the limit when MAX_REQUEST_BODY_BYTES is
// unset. The package comment above carries the measurements behind it.
const DefaultMaxRequestBodyBytes int64 = 32 << 20

// MaxRequestBodyBytesFromEnv reads MAX_REQUEST_BODY_BYTES, a number of bytes,
// returning an error the caller makes fatal.
//
// Unset is the default, never "no limit": an unbounded body is the defect this
// exists to close, so there is no value that turns it off. Zero and negative
// numbers are errors rather than a limit, because a limit of zero refuses
// every write on the instance and nobody sets that on purpose. An unparseable
// value is an error rather than the default quietly substituted, on the
// reasoning the spend caps take: an instance that boots looking configured
// and enforces something other than what the operator wrote is worse than one
// that does not boot.
func MaxRequestBodyBytesFromEnv() (int64, error) {
	raw := strings.TrimSpace(os.Getenv(envMaxRequestBodyBytes))
	if raw == "" {
		return DefaultMaxRequestBodyBytes, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive number of bytes, got %q", envMaxRequestBodyBytes, raw)
	}
	return n, nil
}

// maxRequestBodyBytes is the limit this router enforces: the configured one,
// or the default when Config left it zero, as tests and any caller that
// predates the field do.
func (c Config) maxRequestBodyBytes() int64 {
	if c.MaxRequestBodyBytes > 0 {
		return c.MaxRequestBodyBytes
	}
	return DefaultMaxRequestBodyBytes
}

// limitRequestBody bounds every request body on the app surface. It runs
// before authentication on purpose: refusing a body costs nothing and reveals
// nothing, and an unauthenticated client is the one most worth refusing early.
func limitRequestBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				// Nothing reads the body. net/http closes the connection
				// after this response rather than drain more than a few
				// hundred KiB of it, so the bytes still on the wire cost a
				// socket and no memory.
				writeBodyTooLarge(w, limit)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// decodeJSON reads the request body into v. When it cannot, it answers the
// request itself and returns false: 413 when the body ran past the limit, 400
// for anything else. Every JSON handler in this package reads its body through
// here, which is what makes "an over-limit body is a 413 on every route" true
// for chunked requests too, not only for the ones limitRequestBody can refuse
// from the header.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeBodyTooLarge(w, tooLarge.Limit)
		return false
	}
	writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
	return false
}

// bodyReadStatus is decodeJSON's choice of status for a body read that does
// not go through it: the login form, whose failures are a rendered page
// rather than JSON. It is the same rule: 413 for an overrun, 400 otherwise.
func bodyReadStatus(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// bodyTooLargeResponse is the 413's JSON. The sentence is for whoever reads
// the error (the edit page and the ingest page show `error` as-is), and
// limit_bytes is for a client that wants to compute with it; the agent's tools
// use it to tell the model how far over it went.
type bodyTooLargeResponse struct {
	Error      string `json:"error"`
	LimitBytes int64  `json:"limit_bytes"`
}

func writeBodyTooLarge(w http.ResponseWriter, limit int64) {
	writeJSON(w, http.StatusRequestEntityTooLarge, bodyTooLargeResponse{
		Error:      "request body too large: this instance accepts at most " + humanize.Bytes(limit) + " per request",
		LimitBytes: limit,
	})
}
