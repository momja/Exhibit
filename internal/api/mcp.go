package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/mcpserver"
)

// The remote MCP endpoint (Exh-uhvu): the same tools as cmd/exhibit-mcp,
// served at APP_ORIGIN/mcp over Streamable HTTP, for clients that only speak
// remote MCP or whose users have nowhere to run a local binary.
//
// # Authentication
//
// Only an owner-issued agent grant, as `Authorization: Bearer`. A session
// cookie is never read here, so a page the owner visits cannot drive their
// artifact through this endpoint (there is no ambient authority to forge a
// request with), and neither the operator's static token nor a Pi session's
// token resolves. Each MCP session is bound to the grant that opened it: the
// SDK refuses a later request in that session made under a different grant.
//
// # The write path
//
// The tools do not call handlers or the store. Each is an ordinary API request
// made in-process through the router with the session's own token, so it
// passes the body limit, authMiddleware, agentScopeAllows, owner scoping and
// the revision checks exactly as a request from cmd/exhibit-mcp does. There is
// one write path, and the remote endpoint is just another client of it.
//
// # Origin
//
// The transport spec requires the Origin header to be checked against DNS
// rebinding. A browser page on another origin is refused; a request with no
// Origin (every non-browser client) or with the app's own is admitted. The
// SDK's own localhost Host check is off, because behind a reverse proxy on the
// same machine every request arrives from 127.0.0.1 with the public Host
// header, and it would refuse them all; with bearer-only authentication and
// no cookie, a rebound page would hold no credential to use anyway.

// mcpPath is where the endpoint is served.
const mcpPath = "/mcp"

// mcpSessionIdle is how long an MCP session may sit unused before it is
// closed. A client whose session expired starts a new one transparently.
const mcpSessionIdle = 30 * time.Minute

func (ro *Router) mcpHandler() http.Handler {
	version := "devel"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpserver.New(&loopbackAPI{handler: ro, origin: ro.cfg.AppOrigin, token: bearerToken(r)}, version)
	}, &mcp.StreamableHTTPOptions{
		SessionTimeout:             mcpSessionIdle,
		MaxRequestBodyBytes:        ro.cfg.maxRequestBodyBytes(),
		DisableLocalhostProtection: true,
		Logger:                     slog.Default(),
	})
	bearer := auth.RequireBearerToken(ro.verifyMCPToken, &auth.RequireBearerTokenOptions{
		// A grant lives until it is revoked, so it carries no expiry; the
		// verifier is asked on every request, which is what revocation needs.
		AllowMissingExpiration: true,
	})
	return ro.mcpOriginCheck(bearer(stream))
}

// verifyMCPToken admits an owner-issued agent grant and nothing else. The
// UserID it reports is the grant's id, which is what the SDK binds a session
// to.
func (ro *Router) verifyMCPToken(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
	if !agentscope.IsPersistedToken(token) {
		return nil, auth.ErrInvalidToken
	}
	g, err := ro.cfg.Store.ResolveAgentGrant(ctx, agentscope.PersistedTokenDigest(token))
	if err != nil {
		return nil, fmt.Errorf("resolve agent grant: %w", err)
	}
	if g == nil {
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{UserID: "grant:" + g.ID}, nil
}

func (ro *Router) mcpOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != ro.cfg.AppOrigin {
			http.Error(w, "cross-origin requests are not accepted", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackAPI serves the MCP tools' requests through the router itself, with
// the token the session was opened with.
type loopbackAPI struct {
	handler http.Handler
	origin  string
	token   string
}

func (l *loopbackAPI) Do(ctx context.Context, method, path string, body any) (*mcpserver.Response, error) {
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(valuesDropped{ctx}, method, l.origin+path, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := &loopbackRecorder{header: http.Header{}}
	l.handler.ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return &mcpserver.Response{Status: rec.status, Body: rec.body.Bytes()}, nil
}

// valuesDropped keeps a context's deadline and cancellation and none of its
// values. A tool runs under the context of the /mcp request that carried it,
// and that request's values (chi's route context above all, which would make
// the router treat the loopback as a sub-request mounted under /mcp) must not
// leak into a request that has to be authenticated and routed from scratch.
type valuesDropped struct{ context.Context }

func (valuesDropped) Value(any) any { return nil }

// loopbackRecorder is the response a loopback request writes into.
type loopbackRecorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *loopbackRecorder) Header() http.Header { return r.header }

func (r *loopbackRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *loopbackRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}
