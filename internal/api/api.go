package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/momja/Exhibit/internal/agent"
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

	// Gallery UI — no auth header required (token embedded in page JS)
	ro.Get("/", ro.galleryIndex)
	ro.Get("/artifacts/{artifactID}", ro.galleryDetail)
	ro.Get("/artifacts/{artifactID}/edit", ro.galleryEdit)

	// Embedded static assets (client JS islands, e.g. the CodeMirror editor)
	ro.Handle("/assets/*", assetsHandler())

	// Public share route — no auth required
	ro.Get("/s/{shareID}", ro.serveShare)

	// Agent chat UI (token embedded in page JS, like the gallery) and the
	// SSE event stream (EventSource can't set headers; the handler checks
	// the same bearer token passed as ?token=).
	ro.Get("/agent", ro.agentPage)
	ro.Get("/api/agent/sessions/{sessionID}/events", ro.agentEvents)

	// Authenticated API routes
	ro.Group(func(r chi.Router) {
		r.Use(authMiddleware(ro.cfg.AuthToken))
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
				// (the sandboxed iframe bridges writes via postMessage).
				r.Get("/state", ro.getState)
				r.Put("/state", ro.setState)
				// Per-origin network decisions (exhibit-fr7): decided one
				// origin at a time, so the runtime permission prompt never
				// has to restate the artifact's whole allowlist.
				r.Get("/origins", ro.listOriginDecisions)
				r.Post("/origins", ro.setOriginDecision)
				r.Delete("/origins", ro.deleteOriginDecision)
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
