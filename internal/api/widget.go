package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/store"
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
	Body           string `json:"body,omitempty"`
	WidgetRevision string `json:"widget_revision"`
	// NetworkFootprint is every origin the widget body references. Unapproved
	// lists the subset the artifact's allowlist does not cover — the ones the
	// browser will block. Both are transparency, not policy.
	NetworkFootprint []string `json:"network_footprint"`
	Unapproved       []string `json:"unapproved_origins"`
	WidgetURL        string   `json:"widget_url"`
}

type putWidgetRequest struct {
	Edits                  json.RawMessage `json:"edits"`
	Body                   *string         `json:"body"`
	ExpectedWidgetRevision json.RawMessage `json:"expected_widget_revision"`
}

// getWidget returns the widget's source. A widget-less artifact is a 404: it
// has no widget document, and callers (the edit page, the agent's get_widget)
// need to tell "no widget" apart from "an empty one".
func (ro *Router) getWidget(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "artifact has no widget", "body": "", "widget_revision": a.WidgetRevision})
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
	footprint := networkFootprint(string(body), ro.cfg.RenderOrigin)
	writeJSON(w, http.StatusOK, widgetResponse{
		Body:             string(body),
		WidgetRevision:   a.WidgetRevision,
		NetworkFootprint: footprint,
		Unapproved:       diffOrigins(footprint, a.NetworkAllowlist),
		WidgetURL:        ro.cfg.RenderOrigin + "/w/" + id,
	})
}

// putWidget stores (or replaces) an artifact's widget document.
//
// A save is a new version of the artifact, with a blob of its own: the widget a
// version had is never overwritten, so restoring that version brings its tile
// back with it. Saving a widget byte-identical to the current one records
// nothing — there is nothing to return to.
func (ro *Router) putWidget(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")

	var req putWidgetRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	body := ""
	if req.Body != nil {
		body = *req.Body
	}
	if body == "" && len(req.Edits) == 0 {
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

	fields := map[string]any{}
	if len(req.ExpectedWidgetRevision) > 0 {
		var value any
		if err := json.Unmarshal(req.ExpectedWidgetRevision, &value); err != nil {
			writeError(w, http.StatusBadRequest, "invalid expected_widget_revision")
			return
		}
		fields["expected_widget_revision"] = value
	}
	patching := len(req.Edits) > 0
	if patching {
		if req.Body != nil {
			fields["body"] = body
		}
		var edits any
		if err := json.Unmarshal(req.Edits, &edits); err != nil {
			writeError(w, http.StatusBadRequest, "invalid edits")
			return
		}
		fields["edits"] = edits
		if !ro.expandSourceEdits(w, r, a, fields, true) {
			return
		}
		body = fields["body"].(string)
	}
	expected, ok := contentPrecondition(w, r, fields, "expected_widget_revision", a.WidgetRevision, true)
	if !ok {
		return
	}
	current := ""
	if a.WidgetBlobID != "" {
		// A missing widget can be repaired; other storage failures abort the save.
		var err error
		current, err = ro.readBlobString(r.Context(), a.WidgetBlobID)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			serverError(w, r, "read previous widget body", err)
			return
		}
	}
	version, err := ro.storeVersionContent(r.Context(), ownerID, id, body, true, store.VersionChange{
		Provenance: versionProvenance(r, store.VersionEdit), ExpectedWidgetRevision: expected,
		CheckOnly: a.WidgetBlobID != "" && current == body,
	})
	if err != nil {
		writeArtifactError(w, r, "save widget version", err)
		return
	}

	footprint := networkFootprint(body, ro.cfg.RenderOrigin)
	unapproved := diffOrigins(footprint, a.NetworkAllowlist)
	slog.InfoContext(r.Context(), "widget saved",
		slog.String("artifact_id", id),
		slog.Int("body_bytes", len(body)),
		slog.Any("footprint", footprint),
		slog.Any("unapproved", unapproved),
	)

	responseBody := body
	if patching {
		responseBody = ""
	}
	writeJSON(w, http.StatusOK, widgetResponse{
		Body:             responseBody,
		WidgetRevision:   store.ContentRevision(version.WidgetBlobID),
		NetworkFootprint: footprint,
		Unapproved:       unapproved,
		WidgetURL:        ro.cfg.RenderOrigin + "/w/" + id,
	})
}

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
	id := urlParamID(r, "artifactID")
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
	opts.WidgetOnly = true

	s, err := ro.cfg.Agent.Create(r.Context(), opts)
	if err != nil {
		writeAgentCreateError(w, r, "create widget agent session", err)
		return
	}
	if err := s.Prompt(r.Context(), agent.GenerateWidgetPrompt, nil, nil); err != nil {
		// The session is useless without its prompt; don't leave the
		// subprocess running until the idle reaper notices.
		ro.cfg.Agent.Close(s.OwnerID, s.ID)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// The caller watches this session's SSE stream, which authenticates with
	// a ticket rather than the service token (av-rgp1), so mint the first
	// one here — the button is one action, not two round trips.
	ticket, err := ro.sseTickets.Issue(s.ID, opts.OwnerID)
	if err != nil {
		// The caller never learns this session's id, so leaving it running
		// would strand a subprocess only the idle reaper could ever reclaim.
		ro.cfg.Agent.Close(s.OwnerID, s.ID)
		serverError(w, r, "issue sse ticket", err)
		return
	}

	slog.InfoContext(r.Context(), "widget generation started",
		slog.String("artifact_id", id), slog.String("session_id", s.ID))
	writeJSON(w, http.StatusAccepted, map[string]any{"session_id": s.ID, "sse_ticket": ticket})
}

// widgetGenerateAvailability reports whether the edit page's "Generate widget"
// button can run, and why not when it can't. The button is rendered disabled
// with the reason rather than hidden: a missing affordance is harder to
// diagnose than a disabled one that says what it needs.
func (ro *Router) widgetGenerateAvailability(r *http.Request) (bool, string) {
	if ro.cfg.Agent == nil {
		return false, "Agent support is off on this server (no pi binary), so widgets must be written by hand."
	}
	// Platform mode supplies the credential, so there is no key to be missing
	// — and, more to the point, no way to add one: the reason below would
	// point at a screen this instance does not render (av-siqf).
	if ro.platformMode() {
		return true, ""
	}
	k, err := ro.cfg.Store.GetAgentKey(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil || k == nil {
		return false, "Add an agent API key to generate widgets."
	}
	return true, ""
}

// deleteWidget removes the artifact's widget; the card falls back to the
// default tile. Like a save it is a new version, so restoring an earlier one
// brings the tile back. The bytes stay: the version that had the widget still
// names them.
func (ro *Router) deleteWidget(w http.ResponseWriter, r *http.Request) {
	id := urlParamID(r, "artifactID")
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
	fields := map[string]any{}
	if token := r.Header.Get("If-Match"); token != "" {
		fields["expected_widget_revision"] = token
	}
	expected, ok := contentPrecondition(w, r, fields, "expected_widget_revision", a.WidgetRevision, true)
	if !ok {
		return
	}
	none := ""
	if _, err := ro.cfg.Store.CommitVersion(r.Context(), ownerID, id, store.VersionChange{
		Provenance: versionProvenance(r, store.VersionEdit), WidgetBlobID: &none,
		ExpectedWidgetRevision: expected, CheckOnly: a.WidgetBlobID == "",
	}); err != nil {
		writeArtifactError(w, r, "remove widget", err)
		return
	}
	w.Header().Set("X-Widget-Revision", store.ContentRevision(""))
	w.WriteHeader(http.StatusNoContent)
}
