package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/scanner"
)

// An artifact's widget (av-fafu) is a second self-contained HTML document: the
// glanceable tile its gallery card renders, reading the same server-persisted
// state as the artifact and rendering under the same per-artifact CSP. These
// three routes are its whole write surface, and like every other mutation they
// are the single write path (architecture §4.1) — nothing writes the widget
// blob without passing through here.
//
// The widget carries no allowlist of its own, by design. Its network reach is
// exactly the owning artifact's, so a widget can never contact an origin the
// artifact wasn't already approved for. What PUT does report is the widget's
// *footprint*: origins the widget references that are not on that allowlist
// will simply be blocked at render, and the caller (edit page, agent) is told
// so rather than discovering it as a silent blank tile. As everywhere else, a
// scan never seeds the allowlist (spec §6.2).

type widgetResponse struct {
	Body string `json:"body"`
	// NetworkFootprint is every origin the widget body references. Unapproved
	// lists the subset the artifact's allowlist does not cover — the ones the
	// browser will block. Both are transparency, not policy.
	NetworkFootprint []string `json:"network_footprint"`
	Unapproved       []string `json:"unapproved_origins"`
	WidgetURL        string   `json:"widget_url"`
}

type putWidgetRequest struct {
	Body string `json:"body"`
}

// getWidget returns the widget's source. A widget-less artifact is a 404: it
// has no widget document, and callers (the edit page, the agent's get_widget)
// need to tell "no widget" apart from "an empty one".
func (ro *Router) getWidget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
	if err != nil {
		serverError(w, r, "get widget artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if a.WidgetBlobID == "" {
		http.Error(w, "artifact has no widget", http.StatusNotFound)
		return
	}
	rc, err := ro.cfg.Blob.Get(r.Context(), a.WidgetBlobID)
	if err != nil {
		http.Error(w, "widget body not found", http.StatusNotFound)
		return
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		serverError(w, r, "read widget body", err)
		return
	}
	footprint := scanner.Scan(string(body))
	writeJSON(w, http.StatusOK, widgetResponse{
		Body:             string(body),
		NetworkFootprint: footprint,
		Unapproved:       diffOrigins(footprint, a.NetworkAllowlist),
		WidgetURL:        ro.cfg.RenderOrigin + "/w/" + id,
	})
}

// putWidget stores (or replaces) an artifact's widget document.
//
// The blob id is minted once and reused on every later save, so a widget's
// render URL is stable across edits — the gallery card's iframe src never has
// to change, and no blob is orphaned per revision.
func (ro *Router) putWidget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")

	var req putWidgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Body == "" {
		http.Error(w, "body is required", http.StatusBadRequest)
		return
	}

	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "put widget artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	blobID := a.WidgetBlobID
	if blobID == "" {
		blobID = uuid.New().String()
	}
	if err := ro.cfg.Blob.Put(r.Context(), blobID, bytes.NewReader([]byte(req.Body))); err != nil {
		serverError(w, r, "store widget body", err)
		return
	}
	if a.WidgetBlobID == "" {
		if err := ro.cfg.Store.UpdateArtifact(r.Context(), ownerID, id, map[string]any{"widget_blob_id": blobID}); err != nil {
			writeArtifactError(w, r, "attach widget", err)
			return
		}
	}

	footprint := scanner.Scan(req.Body)
	unapproved := diffOrigins(footprint, a.NetworkAllowlist)
	slog.InfoContext(r.Context(), "widget saved",
		slog.String("artifact_id", id),
		slog.Int("body_bytes", len(req.Body)),
		slog.Any("footprint", footprint),
		slog.Any("unapproved", unapproved),
	)

	writeJSON(w, http.StatusOK, widgetResponse{
		Body:             req.Body,
		NetworkFootprint: footprint,
		Unapproved:       unapproved,
		WidgetURL:        ro.cfg.RenderOrigin + "/w/" + id,
	})
}

// generateWidgetPrompt is the entire user turn the "Generate widget" button
// sends. It is a server-side constant on purpose: the button takes no input, so
// there is nothing a caller can put into the model's context through this
// route. Everything that makes the result a *widget* — read the artifact
// first, follow the tile contract, touch nothing but set_widget — is in the
// session's system prompt (agent.CreateOpts.WidgetOnly), not here.
const generateWidgetPrompt = "Build the gallery widget for this artifact."

// generateWidget starts a one-shot agent session that writes this artifact's
// widget, and returns its session id immediately.
//
// It deliberately does not wait for the result. An agent turn runs for tens of
// seconds, and holding the request open for it would turn every slow model into
// an indistinguishable hang. Instead the caller subscribes to the session's
// existing SSE stream (GET /api/agent/sessions/:id/events) and watches for the
// exhibit_widget_saved event the set_widget tool already emits — so this adds a
// route, not a second streaming mechanism, and the edit page's preview swap is
// driven by the same event the chat surface uses.
func (ro *Router) generateWidget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
	if err != nil {
		serverError(w, r, "generate widget artifact lookup", err)
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "artifact not found")
		return
	}

	opts, ok := ro.agentSessionOpts(w, r)
	if !ok {
		return // agentSessionOpts wrote the reason (no pi binary, no key, …)
	}
	opts.ArtifactID = id
	opts.ArtifactTitle = a.Title
	opts.WidgetOnly = true

	s, err := ro.cfg.Agent.Create(r.Context(), opts)
	if err != nil {
		serverError(w, r, "create widget agent session", err)
		return
	}
	if err := s.Prompt(r.Context(), generateWidgetPrompt, nil); err != nil {
		// The session is useless without its prompt; don't leave the
		// subprocess running until the idle reaper notices.
		ro.cfg.Agent.Close(s.ID)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	slog.InfoContext(r.Context(), "widget generation started",
		slog.String("artifact_id", id), slog.String("session_id", s.ID))
	writeJSON(w, http.StatusAccepted, map[string]any{"session_id": s.ID})
}

// widgetGenerateAvailability reports whether the edit page's "Generate widget"
// button can run, and why not when it can't. The button is rendered disabled
// with the reason rather than hidden: a missing affordance is harder to
// diagnose than a disabled one that says what it needs.
func (ro *Router) widgetGenerateAvailability(r *http.Request) (bool, string) {
	if ro.cfg.Agent == nil {
		return false, "Agent support is off on this server (no pi binary), so widgets must be written by hand."
	}
	k, err := ro.cfg.Store.GetAgentKey(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil || k == nil {
		return false, "Add an agent API key to generate widgets."
	}
	return true, ""
}

// deleteWidget detaches the widget; the card falls back to the default tile.
// The blob is left on disk, matching how DeleteArtifact orphans an artifact
// body in v1 (Blob.Store has no Delete).
func (ro *Router) deleteWidget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "artifactID")
	ownerID := ownerIDFromCtx(r.Context())
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "delete widget artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if a.WidgetBlobID != "" {
		if err := ro.cfg.Store.UpdateArtifact(r.Context(), ownerID, id, map[string]any{"widget_blob_id": ""}); err != nil {
			writeArtifactError(w, r, "detach widget", err)
			return
		}
		slog.InfoContext(r.Context(), "widget removed", slog.String("artifact_id", id))
	}
	w.WriteHeader(http.StatusNoContent)
}
