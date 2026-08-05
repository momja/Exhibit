package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/logging"
	"github.com/momja/Exhibit/internal/render"
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
	Agent       *agent.Manager
	Secrets     *secrets.Box
	MockEnabled bool
	// Identity delegates login to an identity provider (av-30rj). Nil — the
	// default — is a single-user instance: no /auth routes, no login gate,
	// the static token and owner 1 exactly as before. Non-nil is the only
	// thing that changes, and swapping one provider for another changes
	// nothing but which constructor filled this field.
	Identity auth.IdentityProvider
	// SessionTTL bounds how long a login lasts; zero means
	// DefaultSessionTTL. Logout revokes sooner, server-side.
	SessionTTL time.Duration
}

// Router wraps chi.Mux and holds the config.
type Router struct {
	*chi.Mux
	cfg Config
}

// NewRouter constructs the chi router with all routes registered.
func NewRouter(cfg Config) *Router {
	r := &Router{
		Mux: chi.NewRouter(),
		cfg: cfg,
	}
	r.setupRoutes()
	return r
}

func (ro *Router) setupRoutes() {
	// RequestMiddleware is outermost so that panic recovery (Recoverer)
	// happens inside the wrapped writer and the final structured request
	// log still records the 500 status.
	ro.Use(logging.RequestMiddleware)
	ro.Use(middleware.Recoverer)
	// Login gate for the server-rendered pages (av-30rj). A pass-through
	// unless an identity provider is configured, so a single-user instance
	// is unaffected.
	ro.Use(ro.sessionGate)

	// The login flow — registered only when a provider is configured
	// (internal/api/auth.go).
	ro.setupAuthRoutes(ro)

	// Gallery UI — no auth header required (token embedded in page JS)
	ro.Get("/", ro.galleryIndex)
	ro.Get("/new", ro.galleryNew)
	ro.Get("/artifacts/{artifactID}", ro.galleryDetail)
	ro.Get("/artifacts/{artifactID}/edit", ro.galleryEdit)

	// Embedded static assets (client JS islands, e.g. the CodeMirror editor)
	ro.Handle("/assets/*", assetsHandler())

	// Web app manifest (av-fdcx) — app origin only, public and static like
	// the favicon.
	ro.Get("/manifest.json", ro.manifest)

	// Public share route — no auth required
	ro.Get("/s/{shareID}", ro.serveShare)

	// Agent chat UI (token embedded in page JS, like the gallery) and the
	// SSE event stream (EventSource can't set headers; the handler checks
	// the same bearer token passed as ?token=).
	ro.Get("/agent", ro.agentPage)
	ro.Get("/api/agent/sessions/{sessionID}/events", ro.agentEvents)

	// Server-rendered fragments, swapped into a live page by htmx (av-6m3e).
	// They render the same template partials the full page render uses, and
	// carry no authority the page they belong to doesn't already have — so
	// they sit with the page routes, outside the API's auth group.
	ro.Get("/partials/agent-preview", ro.agentPreviewPartial)
	ro.Get("/partials/card-widget", ro.cardWidgetPartial)

	// Authenticated API routes
	ro.Group(func(r chi.Router) {
		r.Use(ro.authMiddleware)
		r.Use(ownerMiddleware)

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
	})

	r := chi.NewRouter()
	r.Use(logging.RequestMiddleware)
	r.Use(middleware.Recoverer)

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
