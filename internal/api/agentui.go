// The agent chat page is an html/template file, templates/agent.tmpl (its
// handler and view model live here, matching the rest of the gallery -
// gallery.go); this file just holds the handler and the view model the
// template consumes.
package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

// agentPageData feeds the "agent" template. ArtifactJSON is pre-marshaled
// JSON (or the literal "null"), injected verbatim into the bootstrap
// <script> as a template.JS value - json.Marshal HTML-escapes '<', '>' and
// '&' by default, which is what keeps this safe to embed in a <script>
// block despite the title coming from user-authored artifact data.
type agentPageData struct {
	Token        string
	ArtifactJSON template.JS
	MockEnabled  bool
	AgentEnabled bool
	BackURL      string
	Preview      agentPreviewData
}

// agentPreviewData feeds the "agentPreview" partial - the preview pane's
// contents, rendered both into the full page and, after every agent save, into
// the htmx fragment response (av-6m3e).
//
// FrameURL carries a per-render cache-busting stamp. The render document is
// served Cache-Control: no-store, but a *stable* src is never re-requested at
// all: the browser only reloads a frame whose src changed. The stamp is what
// turns "the agent saved a new body" into "the visitor sees it".
//
// Widget is the artifact's gallery tile (av-fafu), shown as a strip above the
// live preview so a set_widget call lands somewhere visible — and so the
// default tile is visible too, which is what "this artifact has no widget yet"
// looks like. It carries the same stamp for the same reason.
type agentPreviewData struct {
	HasArtifact bool
	Title       string
	FrameURL    string
	OpenURL     string
	DetailURL   string
	Widget      widgetView
}

// newAgentPreviewData builds the pane's view model for one artifact; the zero
// value (no artifact) renders the empty state.
func (ro *Router) newAgentPreviewData(a *store.Artifact) agentPreviewData {
	if a == nil {
		return agentPreviewData{}
	}
	renderURL := ro.cfg.RenderOrigin + "/a/" + a.ID
	stamp := "?r=" + strconv.FormatInt(time.Now().UnixNano(), 10)
	widget := newWidgetView(a, ro.cfg.RenderOrigin)
	if widget.URL != "" {
		widget.URL += stamp
	}
	return agentPreviewData{
		HasArtifact: true,
		Title:       a.Title,
		FrameURL:    renderURL + stamp,
		OpenURL:     renderURL,
		DetailURL:   "/artifacts/" + a.ID,
		Widget:      widget,
	}
}

// agentPreviewPartial serves the preview pane as a standalone fragment. The
// agent page's htmx wiring re-fetches it whenever the session reports a saved
// artifact, so a create_artifact/update_artifact tool call re-renders the pane
// from the same template the initial page render used - no second, JS-side
// definition of this markup, and no full page reload (which would drop the
// chat transcript and the SSE stream).
//
// Unauthenticated, like /agent itself: this is page chrome for a surface whose
// token lives in its own bootstrap script, and it exposes only a title the
// requester must already know the artifact id to ask for.
func (ro *Router) agentPreviewPartial(w http.ResponseWriter, r *http.Request) {
	var artifact *store.Artifact
	if id := r.URL.Query().Get("artifact"); id != "" {
		a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id)
		if err != nil {
			serverError(w, r, "agent preview lookup", err)
			return
		}
		if a == nil {
			// A fragment 404 is plain text on purpose: htmx leaves the pane
			// untouched on an error response, so the visitor keeps the preview
			// they had rather than watching it blank out.
			http.Error(w, "artifact not found", http.StatusNotFound)
			return
		}
		artifact = a
	}
	fragment, err := renderPage("agentPreview", ro.newAgentPreviewData(artifact))
	if err != nil {
		serverError(w, r, "agent preview render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}

// agentPage serves the agent chat surface (Exh-jlbt): a build/modify-with-AI
// chat on the left, a live sandboxed preview of the session's artifact on the
// right. `?artifact=<id>` opens the page in modify mode bound to that
// artifact. Like the rest of the gallery it is one server-rendered document
// with vanilla-JS islands; streaming arrives over SSE from the session's Pi
// sidecar.
//
// BackURL mirrors the two ways this page is reached rather than hardcoding
// the gallery: the detail page's "Modify with agent" link (?artifact=<id>)
// expects the back link to return to that artifact, and the add-artifact
// page's "Build with agent" tile (bare /agent) expects it back on /new -
// otherwise "add artifact -> agent -> back" strands the visitor on the
// gallery instead of where they came from.
func (ro *Router) agentPage(w http.ResponseWriter, r *http.Request) {
	artifactJSON := "null"
	backURL := "/new"
	var artifact *store.Artifact
	if id := r.URL.Query().Get("artifact"); id != "" {
		if a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), id); err == nil && a != nil {
			j, _ := json.Marshal(map[string]string{"id": a.ID, "title": a.Title})
			artifactJSON = string(j)
			backURL = "/artifacts/" + a.ID
			artifact = a
		}
	}
	page, err := renderPage("agent", agentPageData{
		Token:        ro.cfg.AuthToken,
		ArtifactJSON: template.JS(artifactJSON),
		MockEnabled:  ro.cfg.MockEnabled,
		AgentEnabled: ro.cfg.Agent != nil,
		BackURL:      backURL,
		Preview:      ro.newAgentPreviewData(artifact),
	})
	if err != nil {
		serverError(w, r, "agent page render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}
