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
// when it was an agent session rather than a person.
const agentGrantKey contextKey = "agentGrant"

const defaultOwnerID int64 = 1

// authMiddleware authenticates an API request, says who made it, and — for the
// one caller that is not a person — how far it may reach.
//
// Three credentials are accepted, in this order:
//
//  1. A session cookie, when an identity provider is configured (av-30rj).
//     Browser requests carry it automatically, so a logged-in user's page JS
//     is attributed to that user rather than to whatever token the page was
//     rendered with. The session is looked up per request, which is what makes
//     logout immediate.
//  2. The static bearer token — the API/CLI credential, and the only
//     credential a single-user instance has. Full authority, every route.
//  3. An agent session's scoped token (agentscope, av-e0yj): authority over
//     exactly the artifact that session was opened against, and nothing else.
//     Agent sessions are steered by text Exhibit does not author, so their
//     credential is deny-by-default and checked here rather than trusted to
//     the tools the model calls.
//
// The two restrictions compose rather than substitute for one another. The
// grant names an owner, which becomes this request's ownerID and therefore
// bounds every owner-scoped Store call (av-ep8k); the path check below bounds
// it further to a single artifact *within* that owner. An agent session
// reaches one artifact, and never another owner's anything.
//
// Requests carrying none of the three are rejected, unless the deployment
// configured no token at all — in which case app auth is off, but an agent
// credential is still scoped, because containment is not the same question as
// authentication.
func (ro *Router) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ownerID, ok := ro.sessionUser(r); ok {
			ctx := context.WithValue(r.Context(), ownerIDKey, ownerID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

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
			// Resolve the owner here rather than leaving it to
			// ownerMiddleware's default: the grant knows whose session this
			// is, and every owner-scoped Store call downstream depends on it.
			ctx := context.WithValue(r.Context(), agentGrantKey, g)
			ctx = context.WithValue(ctx, ownerIDKey, scope.OwnerID)
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

// agentSubResources are the sub-paths of an artifact an agent session may
// touch on its *own* artifact — one entry per tool the extension exposes
// (state: av-lvi1, widget: av-fafu), with the methods that tool uses.
//
// It is spelled as an allowlist so a new artifact sub-route is out of an
// agent's reach until someone adds it here deliberately. Notably absent:
// `refetch` (re-pulls a remote page into the body), `collections`/`tags`
// (library organisation, not the artifact), `transcripts` (other sessions'
// conversations), and `widget/generate` (an agent spawning another agent).
var agentSubResources = map[string][]string{
	"state":  {http.MethodGet, http.MethodPut, http.MethodDelete},
	"widget": {http.MethodGet, http.MethodPut, http.MethodDelete},
}

// agentScopeAllows is the entire reach of an agent session credential, written
// as a deny-by-default allowlist so that adding a route to the API never
// silently widens it.
//
// One artifact, a handful of routes:
//
//	POST   /api/artifacts               — only while the session is still unbound
//	GET    /api/artifacts/{id}          — only the session's own artifact
//	PATCH  /api/artifacts/{id}          — only the session's own artifact
//	*      /api/artifacts/{id}/{sub}    — only the session's own artifact, and
//	                                      only the subs in agentSubResources
//
// Everything else is refused: DELETE of the artifact itself, the BYO provider
// key, shares, tags, collections, transcripts, and every other artifact in the
// library. That is what bounds a prompt-injected session to the one artifact
// the user opened rather than to the whole library.
//
// This is the per-*artifact* half of the boundary only. The per-*owner* half
// is the grant's OwnerID flowing into ownerIDFromCtx and from there into the
// owner-scoped Store methods (av-ep8k); neither half is sufficient alone.
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

	// Compare the decoded segment, not the raw path: an id escaped past a
	// literal-prefix check would otherwise read as a different route.
	idSeg, sub, hasSub := strings.Cut(rest, "/")
	id, err := url.PathUnescape(idSeg)
	if err != nil || scope.ArtifactID == "" || id != scope.ArtifactID {
		return false
	}

	if !hasSub {
		return method == http.MethodGet || method == http.MethodPatch
	}
	// Only a single-segment sub-resource, and only a listed one. A deeper
	// path is something these tools don't call, so it is not theirs to reach.
	if strings.Contains(sub, "/") {
		return false
	}
	for _, m := range agentSubResources[sub] {
		if m == method {
			return true
		}
	}
	return false
}

// agentGrantFromCtx returns the scoped credential this request authenticated
// with, or nil when it was a person's.
func agentGrantFromCtx(ctx context.Context) *agentscope.Grant {
	g, _ := ctx.Value(agentGrantKey).(*agentscope.Grant)
	return g
}

// ownerMiddleware supplies the owner for requests authMiddleware did not
// attribute to a session or an agent grant — token-authenticated API clients,
// and every request on a single-user instance. It never overwrites an owner
// already resolved upstream.
func ownerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(ownerIDKey).(int64); ok {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), ownerIDKey, defaultOwnerID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerIDFromCtx(ctx context.Context) int64 {
	if v, ok := ctx.Value(ownerIDKey).(int64); ok {
		return v
	}
	return defaultOwnerID
}

// publicPathPrefixes are the app-origin paths the login gate must never guard:
// the login flow itself (guarding it would be a redirect loop), the static
// assets a login page would need, the API group (which authenticates itself,
// and answers JSON clients rather than redirecting them), and the public share
// route, whose share row *is* its authorization.
var publicPathPrefixes = []string{
	"/auth/",
	"/assets/",
	"/api/",
	"/s/",
	"/manifest.json",
	"/favicon",
}

// sessionGate protects the server-rendered pages once an identity provider is
// configured, sending an unauthenticated visitor to the provider and back to
// the page they asked for.
//
// With no provider configured it is a pass-through — an unconfigured instance
// has no login to send anyone to, and its pages stay exactly as open as they
// have always been.
func (ro *Router) sessionGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ro.identityEnabled() || isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := ro.sessionUser(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		// Only a top-level navigation can survive a trip to the provider.
		// A fetch (htmx fragment, page JS) gets a status its caller can
		// act on instead of an opaque redirect to an HTML login page.
		if r.Method != http.MethodGet || r.Header.Get("HX-Request") != "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		dest := "/auth/login"
		if next := safeNext(r.URL.RequestURI()); next != "" {
			dest += "?next=" + url.QueryEscape(next)
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})
}

func isPublicPath(path string) bool {
	for _, prefix := range publicPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
