package api

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/logging"
	"github.com/momja/Exhibit/internal/render"
	"github.com/momja/Exhibit/internal/rendertoken"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

// Config holds the dependencies and configuration for the API router.
type Config struct {
	Store        store.Store
	Blob         blob.Store
	AppOrigin    string
	RenderOrigin string
	AuthToken    string
	// Agent chat support (Exh-yvhp). Agent is nil when the pi harness is
	// unavailable; Secrets seals the BYO provider keys at rest.
	// AgentCredentials resolves the per-session scoped tokens agent sidecars
	// authenticate with (av-e0yj) — the same registry the manager issues
	// from. Nil means no agent credential is accepted at all.
	Agent            *agent.Manager
	AgentCredentials *agentscope.Registry
	Secrets          *secrets.Box
	MockEnabled      bool
	// Identity delegates login to an identity provider (av-30rj). Nil — the
	// default — is a single-user instance: no /auth routes, no login gate,
	// the static token and owner 1 exactly as before. Non-nil is the only
	// thing that changes, and swapping one provider for another changes
	// nothing but which constructor filled this field.
	Identity auth.IdentityProvider
	// LocalCredential is the instance's own username and password (av-q30x),
	// the second login path. Nil — the default — is the same single-user
	// instance Identity's nil describes; set, it arms the same session gate
	// and lands the same session, so an instance can be secured with one env
	// var pair and no identity server anywhere.
	//
	// It is not an IdentityProvider and is not meant to become one: see
	// internal/auth/local.go for why a form post does not fit a redirect seam.
	//
	// Since av-rzvf it is one account's password rather than the only one
	// there is: accounts live in the users table, and this is the bootstrap
	// and break-glass credential kept in the environment. cmd/server states
	// the role; auth.go implements the precedence it depends on.
	LocalCredential *auth.Credential
	// LocalUsers reports whether the users table already holds an account
	// with a password (av-rzvf) — the other thing that makes local login a
	// real login path on this instance, and what keeps the gate armed for
	// operator-provisioned accounts when no env credential is set.
	//
	// It is configuration read once at startup rather than a query on the
	// request path: the gate consults it on every request including static
	// assets, and the routes it decides are registered once at construction.
	// The consequence is deliberate — provisioning the *first* account with
	// the CLI on an already-running server takes a restart to arm the gate.
	// Every later account is created on an instance whose gate is already up.
	LocalUsers bool
	// SessionTTL bounds how long a login lasts; zero means
	// DefaultSessionTTL. Logout revokes sooner, server-side.
	SessionTTL time.Duration
	// Public opts the instance into serving a read-only gallery to anonymous
	// visitors (av-4ac9). The zero value — a private instance — is the
	// default, and carrying the configuration here rather than in a table is
	// what lets the server-rendered gallery consult it with no per-request
	// database round trip. See publicmode.go.
	Public PublicMode
}

// Router wraps chi.Mux and holds the config.
type Router struct {
	*chi.Mux
	cfg Config
	// tokens signs the render-origin credentials this surface mints for every
	// frame and link it points at RENDER_ORIGIN (av-c5aq). Held on the Router
	// rather than passed around so a page render mints in memory, with no I/O
	// per card.
	tokens *rendertoken.Signer
	// logins throttles failed local logins (av-t21v). In-process and in-memory
	// on purpose: one binary and one SQLite file is this project's deployment
	// contract, and attempt counters do not earn a table — see
	// loginratelimit.go, which also holds the reasoning for what it keys on.
	logins *loginLimiter
}

// compressibleTypes is the explicit set of response content types worth
// compressing. It is spelled out rather than left to chi's default list for
// two reasons: `text/event-stream` must never appear here — the agent surface
// streams SSE and buffering it would stall the stream — and the types that
// dominate our bytes (an artifact render document is `text/html`) should be a
// visible, deliberate choice. Already-compressed payloads are absent on
// purpose: gzipping a PNG, a woff2 or a wasm binary spends CPU to add bytes.
var compressibleTypes = []string{
	"text/html",
	"text/css",
	"text/plain",
	"text/javascript",
	"application/javascript",
	"application/json",
	"image/svg+xml",
}

// compressionLevel is deliberately mid-range. A render document is recomposed
// and recompressed on every view (it carries inlined state and a per-artifact
// CSP, so it cannot be cached), which makes compression CPU a per-request cost
// rather than a one-off. Level 5 keeps nearly all of the size win for a
// fraction of level 9's time.
const compressionLevel = 5

// gzipWriterPool reuses gzip.Writer instances across requests instead of
// allocating one per compressed response.
var gzipWriterPool = sync.Pool{
	New: func() any {
		zw, _ := gzip.NewWriterLevel(io.Discard, compressionLevel)
		return zw
	},
}

// compressor returns the response-compression middleware shared by the app and
// render routers. gzip only: it is stdlib, every client supports it, and brotli
// would mean a new dependency for a modest further gain.
//
// This is a small hand-rolled negotiator rather than chi's
// middleware.Compress: that middleware matches Accept-Encoding with
// strings.Contains, which does not parse quality values — "gzip;q=0" (the
// header a client sends to explicitly refuse gzip) contains "gzip" as a
// substring and so would be compressed anyway. Negotiating correctly needs
// actual quality-value parsing (acceptsGzip below).
func compressor() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
				next.ServeHTTP(w, r)
				return
			}
			cw := &gzipResponseWriter{ResponseWriter: w}
			defer cw.Close()
			next.ServeHTTP(cw, r)
		})
	}
}

// acceptsGzip reports whether the given Accept-Encoding header value permits
// a gzip response, per RFC 9110 §12.5.3: a coding is acceptable if it has an
// explicit entry with q>0, or no explicit entry but a "*" entry with q>0;
// it is unacceptable if its explicit entry has q=0, or "*;q=0" applies with
// no more specific gzip entry. An empty header means the client sent no
// Accept-Encoding at all, which this codebase treats as "don't compress"
// rather than the identity-only default the RFC technically allows.
func acceptsGzip(header string) bool {
	if header == "" {
		return false
	}
	var gzipQ, starQ float64 = -1, -1
	for _, part := range strings.Split(header, ",") {
		name, q := parseAcceptEncoding(part)
		switch name {
		case "gzip":
			gzipQ = q
		case "*":
			starQ = q
		}
	}
	if gzipQ >= 0 {
		return gzipQ > 0
	}
	return starQ > 0
}

// parseAcceptEncoding splits one Accept-Encoding list element (e.g.
// "gzip;q=0.5") into its lowercased coding name and quality value, which
// defaults to 1 when absent or unparsable.
func parseAcceptEncoding(part string) (name string, q float64) {
	q = 1
	fields := strings.Split(part, ";")
	name = strings.ToLower(strings.TrimSpace(fields[0]))
	for _, param := range fields[1:] {
		val, ok := strings.CutPrefix(strings.TrimSpace(param), "q=")
		if !ok {
			continue
		}
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			q = parsed
		}
	}
	return name, q
}

// gzipResponseWriter wraps an http.ResponseWriter, compressing the body with
// gzip when the response's Content-Type is one of compressibleTypes.
// Compressibility is only knowable once the handler sets Content-Type, so
// the decision is made lazily in WriteHeader like chi's compressResponseWriter.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
	compressing bool
}

func (cw *gzipResponseWriter) isCompressible() bool {
	contentType, _, _ := strings.Cut(cw.Header().Get("Content-Type"), ";")
	for _, t := range compressibleTypes {
		if t == contentType {
			return true
		}
	}
	return false
}

func (cw *gzipResponseWriter) WriteHeader(code int) {
	if cw.wroteHeader {
		cw.ResponseWriter.WriteHeader(code)
		return
	}
	cw.wroteHeader = true

	if cw.Header().Get("Content-Encoding") == "" && cw.isCompressible() {
		cw.compressing = true
		zw := gzipWriterPool.Get().(*gzip.Writer)
		zw.Reset(cw.ResponseWriter)
		cw.gz = zw
		cw.Header().Set("Content-Encoding", "gzip")
		cw.Header().Add("Vary", "Accept-Encoding")
		// The content-length after compression is unknown.
		cw.Header().Del("Content-Length")
	}
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *gzipResponseWriter) Write(p []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}
	if cw.compressing {
		return cw.gz.Write(p)
	}
	return cw.ResponseWriter.Write(p)
}

// Flush satisfies http.Flusher so streaming handlers (e.g. the agent SSE
// route) still function when passed through this writer, even though SSE's
// content type is never in compressibleTypes and so is never actually
// compressed.
func (cw *gzipResponseWriter) Flush() {
	if cw.compressing {
		cw.gz.Flush()
	}
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (cw *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := cw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("api: http.Hijacker is unavailable on the underlying ResponseWriter")
}

func (cw *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}

// Close flushes and returns the pooled gzip.Writer, if one was used. It must
// run after the wrapped handler returns (the middleware defers it) so any
// buffered tail bytes reach the client.
func (cw *gzipResponseWriter) Close() error {
	if !cw.compressing {
		return nil
	}
	err := cw.gz.Close()
	gzipWriterPool.Put(cw.gz)
	cw.gz = nil
	return err
}

// NewRouter constructs the chi router with all routes registered.
func NewRouter(cfg Config) *Router {
	r := &Router{
		Mux:    chi.NewRouter(),
		cfg:    cfg,
		tokens: renderSigner(cfg.Secrets),
		logins: newLoginLimiter(nil),
	}
	r.setupRoutes()
	return r
}

// renderSigner derives the render-token signing key from the same server secret
// that seals agent provider keys, domain-separated so the two never share key
// material. Deriving rather than configuring keeps the operator's contract at
// one secret (EXHIBIT_SECRET or the generated data/secret.key).
//
// With no Box at all — a process constructed without secrets, which in practice
// means a test — an ephemeral key is generated instead. Tokens then work end to
// end but do not survive a restart, which is the strict answer; the permissive
// one would be an unauthenticated render origin.
func renderSigner(box *secrets.Box) *rendertoken.Signer {
	if box == nil {
		return rendertoken.NewRandomSigner()
	}
	key := box.DeriveKey(rendertoken.KeyPurpose)
	return rendertoken.NewSigner(key)
}

func (ro *Router) setupRoutes() {
	// RequestMiddleware is outermost so that panic recovery (Recoverer)
	// happens inside the wrapped writer and the final structured request
	// log still records the 500 status.
	ro.Use(logging.RequestMiddleware)
	ro.Use(middleware.Recoverer)
	ro.Use(compressor())
	// Login gate for the server-rendered pages (av-30rj). A pass-through
	// unless an identity provider is configured, so a single-user instance
	// is unaffected.
	ro.Use(ro.sessionGate)

	// The login flow — registered only when a provider is configured
	// (internal/api/auth.go).
	ro.setupAuthRoutes(ro)

	// Server-rendered pages (av-syug). They carry no Authorization header —
	// a page's own JS authenticates its API calls separately, see
	// pagecredential.go — but they read the library server-side, so they need
	// the one thing the API group's chain supplies beyond authentication:
	// whose library this is.
	//
	// ownerMiddleware alone is what that takes. The credential is already
	// resolved upstream by sessionGate, which puts the session's owner in the
	// context and never reaches here for a visitor who has none (it redirects
	// to /auth/login first); this group only backstops the instances that
	// issue no sessions at all, where the answer is the single-user default.
	//
	// Membership of this group is also the declaration that a route is
	// owner-scoped, which is what av-syug's route walk holds new page routes
	// to: a page that renders artifacts belongs in here, not beside the
	// static assets below.
	ro.Group(func(r chi.Router) {
		r.Use(ro.ownerMiddleware)

		// Gallery UI
		r.Get("/", ro.galleryIndex)
		r.Get("/new", ro.galleryNew)
		r.Get("/artifacts/{artifactID}", ro.galleryDetail)
		r.Get("/artifacts/{artifactID}/edit", ro.galleryEdit)
		// "Open in new tab": mints a fresh render token and redirects to
		// RENDER_ORIGIN/a/:id (av-c5aq). Links go through here rather than
		// carrying a token in the markup, which would be stale by the time
		// anyone clicked it.
		r.Get("/artifacts/{artifactID}/open", ro.openArtifact)

		// Agent chat UI — token embedded in page JS, like the gallery.
		r.Get("/agent", ro.agentPage)

		// The administration surface (av-utap). It sits in the page group
		// like every other page — the group is what supplies an owner — and
		// then behind adminOnly, because carrying an owner is not carrying
		// authority. To anyone who is not an admin this route answers the
		// same 404 an unrouted path does; admin.go says why that, and not a
		// 403, is the right refusal.
		r.With(ro.adminOnly).Get("/admin/users", ro.adminUsersPage)

		// A person's own account (av-qo05). The same page group and the same
		// page furniture as the route above, and deliberately not the same
		// guard: this handler reads ownerIDFromCtx and nothing else, so a
		// session is the whole authorization. Registering the two beside each
		// other is where that difference is visible.
		r.Get("/profile", ro.profilePage)

		// Server-rendered fragments, swapped into a live page by htmx
		// (av-6m3e). They render the same template partials the full page
		// render uses, and carry no authority the page they belong to
		// doesn't already have — so they sit with the pages, outside the
		// API's auth group and under the same owner resolution.
		r.Get("/partials/agent-preview", ro.agentPreviewPartial)
		r.Get("/partials/card-widget", ro.cardWidgetPartial)
	})

	// Embedded static assets (client JS islands, e.g. the CodeMirror editor)
	ro.Handle("/assets/*", assetsHandler())

	// Web app manifest (av-fdcx) — app origin only, public and static like
	// the favicon.
	ro.Get("/manifest.json", ro.manifest)

	// Public share route — no auth required
	ro.Get("/s/{shareID}", ro.serveShare)

	// The instance's public identity (av-4ac9). Registered here, outside the
	// authenticated API group, because a visitor with no credential is
	// precisely who needs it; a private instance answers 404 rather than
	// naming itself. See publicmode.go for that choice.
	ro.Get("/api/settings/public", ro.publicSettings)

	// The agent SSE event stream. EventSource can't set headers, so the
	// handler checks the same bearer token passed as ?token= (or the session
	// cookie) itself rather than going through the API's auth middleware. It
	// resolves a session by id and streams its events; it reads nothing
	// owner-scoped, which is why it stays out of the page group above.
	ro.Get("/api/agent/sessions/{sessionID}/events", ro.agentEvents)

	// Authenticated API routes
	ro.Group(func(r chi.Router) {
		r.Use(ro.authMiddleware)
		r.Use(ro.ownerMiddleware)

		r.Route("/api/artifacts", func(r chi.Router) {
			r.Get("/", ro.listArtifacts)
			r.Post("/", ro.createArtifact)
			r.Route("/{artifactID}", func(r chi.Router) {
				r.Get("/", ro.getArtifact)
				r.Patch("/", ro.updateArtifact)
				r.Post("/refetch", ro.refetchArtifact)
				r.Delete("/", ro.deleteArtifact)
				// State: written by the host frame on the artifact's behalf
				// (the sandboxed iframe bridges writes via postMessage), and
				// by the edit page's state inspector (av-hg5f). DELETE is the
				// row-removal contract the shim's removeItem/clear fixes
				// consume too. One route serves both deletes, discriminated
				// by whether a `key` query parameter is present — the key is
				// deliberately NOT a path segment, because a key of ".." is
				// resolved away by the browser before the request is sent and
				// lands on the artifact delete above (av-hh1o).
				r.Get("/state", ro.getState)
				r.Put("/state", ro.setState)
				r.Delete("/state", ro.deleteState)
				// The artifact's gallery-card widget (av-fafu) — a second
				// document under the artifact's own security envelope, so it
				// hangs off the artifact rather than being a resource of its
				// own. widget.go explains why it carries no allowlist.
				r.Get("/widget", ro.getWidget)
				r.Put("/widget", ro.putWidget)
				r.Delete("/widget", ro.deleteWidget)
				// Starts a one-shot agent session that writes the widget and
				// returns its id; the caller watches the session's existing
				// SSE stream for the result rather than holding this request
				// open for a whole agent turn.
				r.Post("/widget/generate", ro.generateWidget)
				// Artifact-centric collection membership routes
				r.Post("/collections/{collectionID}", ro.addArtifactToCollection)
				r.Delete("/collections/{collectionID}", ro.removeArtifactFromCollection)
				// Artifact-centric tag routes
				r.Post("/tags/{tagID}", ro.addArtifactTag)
				r.Delete("/tags/{tagID}", ro.removeArtifactTag)
				// Agent conversations persisted with this artifact
				r.Get("/transcripts", ro.listTranscripts)
			})
		})

		r.Route("/api/agent", func(r chi.Router) {
			r.Get("/key", ro.getAgentKey)
			r.Put("/key", ro.putAgentKey)
			r.Delete("/key", ro.deleteAgentKey)
			r.Post("/sessions", ro.createAgentSession)
			r.Post("/sessions/{sessionID}/prompt", ro.agentPrompt)
			r.Post("/sessions/{sessionID}/abort", ro.agentAbort)
			r.Delete("/sessions/{sessionID}", ro.closeAgentSession)
		})

		r.Route("/api/collections", func(r chi.Router) {
			r.Get("/", ro.listCollections)
			r.Post("/", ro.createCollection)
			r.Post("/{collectionID}/artifacts/{artifactID}", ro.addArtifactToCollection)
			r.Delete("/{collectionID}/artifacts/{artifactID}", ro.removeArtifactFromCollection)
		})

		r.Route("/api/tags", func(r chi.Router) {
			r.Get("/", ro.listTags)
			r.Post("/", ro.createTag)
			r.Patch("/{tagID}", ro.updateTag)
			r.Delete("/{tagID}", ro.deleteTag)
			r.Post("/{tagID}/artifacts/{artifactID}", ro.addArtifactTag)
			r.Delete("/{tagID}/artifacts/{artifactID}", ro.removeArtifactTag)
		})

		// Administration of other people's accounts (av-utap). Every route
		// in here acts on somebody else, so the group carries adminOnly on
		// top of the API group's own authentication — a session is not
		// authorization for any of it. Routes a person performs on their
		// *own* account are av-g2dx's and belong outside this group.
		r.Route("/api/admin/users", func(r chi.Router) {
			r.Use(ro.adminOnly)
			r.Get("/", ro.listAdminUsers)
			r.Post("/", ro.createAdminUser)
			// One PATCH for password / disabled / is_admin, each optional:
			// the fields are unrelated, and a route per verb would multiply
			// the surface that has to be guarded without splitting anything
			// that is actually separate.
			r.Patch("/{userID}", ro.updateAdminUser)
		})

		// The caller's own account (av-4wyq, epic av-g2dx). Deliberately not
		// inside the group above: that one acts on *other* accounts and
		// carries adminOnly for it, while this route cannot name an account
		// at all — it takes no id, from path or body, and erases whatever the
		// request's own session resolved to. That is the whole authority
		// argument, and registering the two apart is where it is visible.
		r.Delete("/api/account", ro.deleteAccount)

		r.Route("/api/shares", func(r chi.Router) {
			r.Post("/", ro.createShare)
			r.Delete("/{shareID}", ro.deleteShare)
		})
	})

	// Registered last on purpose: chi copies the not-found handler into every
	// subrouter that hasn't set one of its own at the moment NotFound is
	// called, so an earlier registration would leave the /api/* subrouters on
	// chi's default. The handler itself keeps /api/* on the plain-text
	// fallback and renders the styled page for everything else (gallery.go).
	ro.NotFound(ro.notFound)
}

// RenderHandler returns an http.Handler for the render origin.
// It is read-only and serves artifacts in sandboxed iframes.
func (ro *Router) RenderHandler() http.Handler {
	renderer := render.New(render.Config{
		Store:        ro.cfg.Store,
		Blob:         ro.cfg.Blob,
		AppOrigin:    ro.cfg.AppOrigin,
		RenderOrigin: ro.cfg.RenderOrigin,
		// The same Signer the app surface mints with: one process, one key,
		// stateless verification — no shared table, no round trip.
		Tokens: ro.tokens,
	})

	r := chi.NewRouter()
	r.Use(logging.RequestMiddleware)
	r.Use(middleware.Recoverer)
	// This is the surface compression matters most on: a render document is
	// composed per request and served no-store, so every view pays its full
	// size over the wire with no cache to amortise it.
	r.Use(compressor())
	// Every response this mux emits — rendered document, 404 on a rejected
	// token, unrouted path — withholds its Referer, because the URL that
	// produced it carries a render token (av-nr0p).
	r.Use(render.NoReferrer)

	// Serve a rendered artifact by id
	r.Get("/a/{artifactID}", renderer.ServeArtifact)
	// Serve an artifact's gallery-card widget (av-fafu) — same origin, same
	// per-artifact CSP, narrower preamble.
	r.Get("/w/{artifactID}", renderer.ServeWidget)
	// Serve share via render origin
	r.Get("/s/{shareID}", renderer.ServeShare)

	return r
}

// serveShare handles public share links on the app origin,
// redirecting to the render origin.
func (ro *Router) serveShare(w http.ResponseWriter, r *http.Request) {
	shareID := chi.URLParam(r, "shareID")
	http.Redirect(w, r, ro.cfg.RenderOrigin+"/s/"+shareID, http.StatusFound)
}
