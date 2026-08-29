package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/momja/Exhibit/internal/agentscope"
)

type contextKey string

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
// Requests carrying none of the three are rejected, with two exceptions, both
// below the three credentials so neither can shadow one:
//
//   - Public mode (av-wmp6): a public instance answers a *read* of its
//     published library to a request that resolved no credential. GET only, and
//     only the routes publicReadable names — never a mutation, whatever the
//     configuration says.
//   - No token configured: app auth is off entirely. An agent credential is
//     still scoped, because containment is not the same question as
//     authentication.
//
// The public branch sits above that last one deliberately. Both let an
// anonymous request through, but they disagree about *whose* library it reads:
// the pass-through leaves the owner unresolved (ownerMiddleware's default,
// owner 1) while public mode names an owner explicitly. On an instance that is
// both open and public with PUBLIC_OWNER_ID set to anything but 1, the
// pass-through's answer is the wrong library — so the branch that knows the
// owner is asked first.
func (ro *Router) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ownerID, ok := ro.sessionUser(r); ok {
			ctx := withPrincipal(r.Context(), Principal{OwnerID: ownerID, Kind: PrincipalSession})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		token := bearerToken(r)
		if ro.matchesServiceToken(token) {
			// Every service-token request is the operator, full authority,
			// under the single-user default owner — the same value
			// ownerMiddleware's backstop would supply, set here explicitly so
			// this Principal is complete on its own (below, PrincipalNone's
			// backstop is the only case that still relies on the backstop).
			ctx := withPrincipal(r.Context(), Principal{OwnerID: defaultOwnerID, Kind: PrincipalServiceToken})
			next.ServeHTTP(w, r.WithContext(ctx))
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
			// The grant knows whose session this is, and every owner-scoped
			// Store call downstream depends on it — resolved here, on the
			// Principal, rather than deferred to ownerMiddleware.
			ctx := withPrincipal(r.Context(), Principal{OwnerID: scope.OwnerID, Kind: PrincipalAgentGrant, Grant: g})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if ro.publicRead(r) {
			ctx := withPrincipal(r.Context(), Principal{OwnerID: ro.cfg.Public.OwnerID, Kind: PrincipalPublic, ReadOnly: true})
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

// publicRead reports whether r is the one thing a public instance answers
// without a credential: an anonymous read of the library it publishes.
//
// It is reached only after all three credentials have failed to resolve, so
// "anonymous" here means "presented nothing this instance recognizes" — a
// request holding a valid token or session took an earlier branch and is
// attributed to whoever it named, public mode or not.
func (ro *Router) publicRead(r *http.Request) bool {
	return ro.cfg.Public.Enabled && publicReadable(r.Method, r.URL.EscapedPath())
}

// publicReadable is the entire reach of an anonymous visitor on a public
// instance, written as a deny-by-default allowlist for the same reason
// agentScopeAllows is: a route added to the API later must not become public
// because nobody thought about it.
//
// It is exactly the library and one artifact in it:
//
//	GET /api/artifacts        — the published library
//	GET /api/artifacts/{id}   — one artifact's metadata and source
//
// Method first, and only GET: no configuration makes a mutation anonymous, so
// that check is the outermost one rather than a property of the paths listed.
//
// Everything deeper is refused, and the sub-resources are why the rule is a
// prefix *exclusion* rather than a prefix match. `/state` is the owner's own
// data — the thing publishing a library must not publish (the render surface
// makes the same call for the same reason, av-wmp6). `/transcripts` is the
// owner's conversations with an agent. `/widget` is source, not a tile; the
// tile a public card shows comes from the render origin under its own token.
// The rest of the API — collections, tags, shares, agent, the BYO provider key
// — is never reached at all, and a public *page* needing tag names renders them
// server-side rather than opening this door wider.
func publicReadable(method, escapedPath string) bool {
	if method != http.MethodGet {
		return false
	}
	rest, ok := artifactsSubPath(escapedPath)
	if !ok {
		return false
	}
	// "" and "/" are the list route; "/{id}" is one artifact. A second
	// separator means a sub-resource, which is not public.
	return !strings.Contains(strings.TrimPrefix(rest, "/"), "/")
}

// artifactsSubPath is the one piece publicReadable and agentScopeAllows
// genuinely share: both are deny-by-default allowlists over paths under
// /api/artifacts, and both need the same first step — strip that prefix, and
// refuse anything where what follows isn't empty or its own path segment (a
// route that merely starts with the same characters, e.g.
// /api/artifactsomething, is not under it). What each does with the
// remainder differs by design — one asks "is this the list or one artifact,
// read-only", the other "is this the session's own artifact, and one of its
// allowlisted sub-resources" — so only this shared prefix step is factored
// out; unifying the two policies themselves would blur two different
// questions into one, harder-to-audit table.
func artifactsSubPath(escapedPath string) (rest string, ok bool) {
	rest, ok = strings.CutPrefix(escapedPath, "/api/artifacts")
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return "", false
	}
	return rest, true
}

// matchesServiceToken reports whether candidate is exactly the operator's
// static bearer token. The comparison is constant-time: authorizeEventStream
// (agent.go) always compared this way because its token can arrive as a URL
// query parameter, but authMiddleware's Authorization-header comparison used
// to be a plain ==. One function for both closes that gap rather than leaving
// it as a difference nobody chose. An empty AuthToken never matches — that is
// app auth being off entirely, handled by authMiddleware's own pass-through
// branch, not this function's to decide.
func (ro *Router) matchesServiceToken(candidate string) bool {
	if ro.cfg.AuthToken == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(ro.cfg.AuthToken)) == 1
}

// hasServiceToken reports whether r carries the operator's static token — the
// full-authority API/CLI credential, and the only credential a single-user
// instance has.
//
// It is one function rather than a comparison at each site because two places
// now ask the question and they must agree: authMiddleware, to admit the
// request at all, and adminRequest (admin.go), to decide it may act on the
// instance's accounts. An empty AuthToken is app auth being off entirely, which
// is emphatically not "every request holds the token" — that branch is handled
// downstream, on its own terms.
func (ro *Router) hasServiceToken(r *http.Request) bool {
	return ro.cfg.AuthToken != "" && bearerToken(r) == ro.cfg.AuthToken
}

// bearerToken pulls the Authorization bearer value, or "" when absent.
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	// The auth-scheme token is case-insensitive per RFC 7235 §2.1; several
	// HTTP client libraries send "bearer" rather than "Bearer".
	if len(auth) < 7 || !strings.EqualFold(auth[:7], "Bearer ") {
		return ""
	}
	return auth[7:]
}

func (ro *Router) resolveAgentGrant(token string) *agentscope.Grant {
	if ro.cfg.AgentCredentials == nil {
		return nil
	}
	return ro.cfg.AgentCredentials.Resolve(token)
}

// agentSubResources are the sub-paths of an artifact an agent session may
// touch on its *own* artifact — one entry per tool the extension exposes
// (state: av-lvi1, widget: av-fafu, assets: av-20fk), with the methods that
// tool uses.
//
// It is spelled as an allowlist so a new artifact sub-route is out of an
// agent's reach until someone adds it here deliberately. Notably absent:
// `refetch` (re-pulls a remote page into the body), `collections`/`tags`
// (library organisation, not the artifact), `transcripts` (other sessions'
// conversations), and `widget/generate` (an agent spawning another agent).
//
// `assets` is GET only, and the asymmetry is the point. The agent needs to
// know which URLs the page fetches are already served from stored copies —
// otherwise it reads a bare `fetch('https://cdn…/app.wasm')` in the body and
// "fixes" an origin that is not actually contacted. Deleting one is a
// different act: it is the owner's escape hatch for a payload whose feature
// they edited away (architecture §3.1), it is irreversible, and it is decided
// on grounds the model cannot see. The route also serves metadata only, never
// bytes, so this grant cannot put a vendored payload into a context window.
var agentSubResources = map[string][]string{
	"state":  {http.MethodGet, http.MethodPut, http.MethodDelete},
	"widget": {http.MethodGet, http.MethodPut, http.MethodDelete},
	"assets": {http.MethodGet},
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
// urlParamID returns the decoded path parameter named key, so store lookups
// resolve the same canonical id that agentScopeAllows already authorized.
// chi routes off the escaped path (RoutePath falls back to r.URL.RawPath),
// so chi.URLParam returns the raw, still-percent-encoded segment — exactly
// what agentScopeAllows unescapes before comparing against scope.ArtifactID.
// A caller that skipped this and used chi.URLParam directly could authorize
// against one decoding of an id and look up another. A malformed escape is
// returned unchanged: it won't match any stored id, so the lookup fails
// closed as a 404 rather than erroring.
func urlParamID(r *http.Request, key string) string {
	raw := chi.URLParam(r, key)
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

func agentScopeAllows(scope agentscope.Scope, method, escapedPath string) bool {
	rest, ok := artifactsSubPath(escapedPath)
	if !ok {
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

// ownerMiddleware supplies the single-user default owner for requests no
// upstream credential check attributed to somebody. It never overwrites a
// Principal already resolved upstream, which is what lets it sit under both
// credential paths: authMiddleware for the API group, sessionGate for the
// page group (av-syug).
//
// It used to also special-case a public visitor (av-wmp6) — resolving
// PUBLIC_OWNER_ID here rather than in authMiddleware, on the theory that
// doing so kept "may this request proceed" and "whose library does it read"
// in the two middlewares that already asked those questions. av-o5cf
// collapsed that: authMiddleware's public branch now resolves the owner too,
// as one complete Principal, so this middleware is a pure backstop — the
// single-user default, and nothing else — with no Kind of its own to decide.
//
// Every route that reads library data runs this, so a handler that reaches
// ownerIDFromCtx always finds an owner that was decided rather than assumed.
func (ro *Router) ownerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principalResolved(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := withPrincipal(r.Context(), Principal{OwnerID: defaultOwnerID, Kind: PrincipalNone})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// noOwner is the owner id of a request nobody attributed. It matches no row —
// owner ids are AUTOINCREMENT and start at 1 — so a scoped Store call made with
// it returns the empty set rather than somebody's library.
const noOwner int64 = 0

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

// sessionGate protects the server-rendered pages once this instance has a
// login, sending an unauthenticated visitor to /auth/login and back to the page
// they asked for.
//
// "Has a login" means either configured path (av-q30x): an identity provider,
// or a local credential. Keying it on the provider alone — as it was — meant a
// self-hoster who set a username and password would still be serving every
// gallery page to anyone who could reach the app origin.
//
// With neither configured it is a pass-through — an unconfigured instance has
// no login to send anyone to, and its pages stay exactly as open as they have
// always been.
//
// A request it admits on a session resolves one Principal, Kind
// PrincipalSession, which answers two questions downstream that are
// complementary rather than alternatives:
//
//   - **Who it is** — OwnerID, which every owner-scoped Store call reads
//     through ownerIDFromCtx. The page routes sit outside the API's auth
//     group (their JS authenticates separately), so this gate is the only
//     place a page request meets its user; discarding the id here and
//     propagating only a boolean is what served owner 1's library to everyone
//     who logged in (av-syug). authMiddleware resolves the same value the
//     same way for the API group.
//   - **That it is** — Kind == PrincipalSession, read via sessionAuthed(),
//     which decides what credential the page may embed (av-5imk,
//     pagecredential.go). The gate has already paid for the lookup, so the
//     render downstream does not repeat it.
//
// One answers whose data the page shows, the other what authority it hands its
// own scripts; neither substitutes for the other.
func (ro *Router) sessionGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ro.loginEnabled() || isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if ownerID, ok := ro.sessionUser(r); ok {
			ctx := withPrincipal(r.Context(), Principal{OwnerID: ownerID, Kind: PrincipalSession})
			next.ServeHTTP(w, r.WithContext(ctx))
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
		if returnTo := safeNext(r.URL.RequestURI()); returnTo != "" {
			dest += "?next=" + url.QueryEscape(returnTo)
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
