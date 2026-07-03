package api

import (
	"net/http"

	"github.com/artifact-viewer/artifact-viewer/internal/blob"
	"github.com/artifact-viewer/artifact-viewer/internal/render"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Config holds the dependencies and configuration for the API router.
type Config struct {
	Store        store.Store
	Blob         blob.Store
	AppOrigin    string
	RenderOrigin string
	AuthToken    string
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
	ro.Use(middleware.Logger)
	ro.Use(middleware.Recoverer)
	ro.Use(loggingMiddleware)

	// Gallery UI — no auth header required (token embedded in page JS)
	ro.Get("/", ro.galleryIndex)
	ro.Get("/artifacts/{artifactID}", ro.galleryDetail)
	ro.Get("/artifacts/{artifactID}/edit", ro.galleryEdit)

	// Embedded static assets (client JS islands, e.g. the CodeMirror editor)
	ro.Handle("/assets/*", assetsHandler())

	// Public share route — no auth required
	ro.Get("/s/{shareID}", ro.serveShare)

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
				r.Get("/state", ro.getState)
				r.Put("/state", ro.setState)
				// Artifact-centric collection membership routes
				r.Post("/collections/{collectionID}", ro.addArtifactToCollection)
				r.Delete("/collections/{collectionID}", ro.removeArtifactFromCollection)
				// Artifact-centric tag routes
				r.Post("/tags/{tagID}", ro.addArtifactTag)
				r.Delete("/tags/{tagID}", ro.removeArtifactTag)
			})
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
	r.Use(middleware.Logger)
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
