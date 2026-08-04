package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/momja/Exhibit/internal/agentscope"
)

type contextKey string

const ownerIDKey contextKey = "ownerID"

// agentGrantKey carries the scoped credential a request authenticated with,
// when it was an agent session rather than the operator.
const agentGrantKey contextKey = "agentGrant"

const defaultOwnerID int64 = 1

// authMiddleware admits two kinds of credential and treats them very
// differently.
//
//   - The operator's service token: full authority, every route.
//   - An agent session's scoped token (agentscope, av-e0yj): authority over
//     exactly the artifact that session was opened against, and nothing else.
//     Agent sessions are steered by text Exhibit does not author, so their
//     credential is deny-by-default and checked here rather than trusted to
//     the tools the model calls.
//
// Requests carrying neither are rejected, unless the deployment configured no
// token at all — in which case app auth is off, but an agent credential is
// still scoped, because containment is not the same question as authentication.
func (ro *Router) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)

		if ro.cfg.AuthToken != "" && token == ro.cfg.AuthToken {
			next.ServeHTTP(w, r)
			return
		}

		if g := ro.resolveAgentGrant(token); g != nil {
			scope := g.Scope()
			if !agentScopeAllows(scope, r.Method, r.URL.EscapedPath()) {
				slog.WarnContext(r.Context(), "agent credential refused outside its scope",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("scoped_artifact_id", scope.ArtifactID),
				)
				writeError(w, http.StatusForbidden,
					"this agent session may only act on the artifact it was opened against")
				return
			}
			ctx := context.WithValue(r.Context(), agentGrantKey, g)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if ro.cfg.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// bearerToken pulls the Authorization bearer value, or "" when absent.
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

func (ro *Router) resolveAgentGrant(token string) *agentscope.Grant {
	if ro.cfg.AgentCredentials == nil {
		return nil
	}
	return ro.cfg.AgentCredentials.Resolve(token)
}

// agentScopeAllows is the entire reach of an agent session credential, written
// as a deny-by-default allowlist so that adding a route to the API never
// silently widens it.
//
// Two routes, one artifact:
//
//	POST  /api/artifacts       — only while the session is still unbound
//	GET   /api/artifacts/{id}  — only the session's own artifact
//	PATCH /api/artifacts/{id}  — only the session's own artifact
//
// Everything else is refused: the BYO provider key, shares, deletes, artifact
// state, tags, collections, transcripts, and every other artifact in the
// library. That is what bounds a prompt-injected session to the one artifact
// the user opened rather than to the whole library.
func agentScopeAllows(scope agentscope.Scope, method, escapedPath string) bool {
	rest, ok := strings.CutPrefix(escapedPath, "/api/artifacts")
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return false
	}
	rest = strings.TrimPrefix(rest, "/")

	if rest == "" {
		// Create. Allowed only until the session binds: the artifact the
		// first create returns becomes the scope, so a second create — or any
		// create in a modify session — is refused here.
		return method == http.MethodPost && scope.ArtifactID == ""
	}
	if strings.Contains(rest, "/") {
		// Sub-resources (state, tags, collections, transcripts, refetch) are
		// out of scope even on the session's own artifact.
		return false
	}
	// Compare the decoded segment, not the raw path: an id escaped past a
	// literal-prefix check would otherwise read as a different route.
	id, err := url.PathUnescape(rest)
	if err != nil || scope.ArtifactID == "" || id != scope.ArtifactID {
		return false
	}
	return method == http.MethodGet || method == http.MethodPatch
}

// agentGrantFromCtx returns the scoped credential this request authenticated
// with, or nil when it was the operator's token.
func agentGrantFromCtx(ctx context.Context) *agentscope.Grant {
	g, _ := ctx.Value(agentGrantKey).(*agentscope.Grant)
	return g
}

func ownerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ownerID := defaultOwnerID
		if g := agentGrantFromCtx(r.Context()); g != nil {
			ownerID = g.Scope().OwnerID
		}
		ctx := context.WithValue(r.Context(), ownerIDKey, ownerID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerIDFromCtx(ctx context.Context) int64 {
	if v, ok := ctx.Value(ownerIDKey).(int64); ok {
		return v
	}
	return defaultOwnerID
}
