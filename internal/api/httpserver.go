package api

import (
	"net/http"
	"time"
)

// Connection-level bounds for every listener this process opens (av-ombn).
// The request body limit (bodylimit.go) bounds how much a client may send;
// these bound how long it may take and how much it may say before the body.
// Bare http.ListenAndServe sets none of them, and docker-compose publishes the
// process with no proxy in front, so a client that never finishes its headers
// held a goroutine and a socket for as long as it liked.
const (
	// serverReadHeaderTimeout is how long a client has to finish the request
	// line and headers. Generous for any real client; what it ends is a
	// connection that trickles a header a byte at a time to stay open.
	serverReadHeaderTimeout = 10 * time.Second
	// serverReadTimeout bounds reading the whole request, body included, so it
	// has to cover a maximal body on a slow uplink: 32 MiB in five minutes is
	// about 0.9 Mbit/s. It does not cut a long response short: the agent's
	// SSE stream outlives it, because the read deadline governs reads and the
	// stream only writes.
	serverReadTimeout = 5 * time.Minute
	// serverIdleTimeout is how long a keep-alive connection waits for its next
	// request. Left unset it would default to serverReadTimeout, which is
	// sized for an upload, not for an idle socket.
	serverIdleTimeout = 2 * time.Minute
	// serverMaxHeaderBytes caps the request line and headers together. The
	// largest legitimate ones are a session cookie, a bearer token and a
	// render token in a query string, each well under a kilobyte; 64 KiB is
	// already more than the proxies this sits behind accept by default.
	serverMaxHeaderBytes = 64 << 10
)

// NewServer returns the http.Server cmd/server listens with, for either
// surface or both behind a host dispatcher.
//
// WriteTimeout is left at zero on purpose. It bounds the whole response, and
// the agent's SSE stream is a response that stays open for as long as a chat
// page does; a server-wide write deadline would cut every one of them. The
// cost is stated rather than hidden: a client that reads a response very
// slowly keeps its handler, and for a render the document, alive until it
// finishes. Bounding that takes a per-route deadline
// (http.ResponseController.SetWriteDeadline), which nothing sets yet.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		IdleTimeout:       serverIdleTimeout,
		MaxHeaderBytes:    serverMaxHeaderBytes,
	}
}
