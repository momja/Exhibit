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
)

// agentPageData feeds the "agent" template. ArtifactJSON is pre-marshaled
// JSON (or the literal "null"), injected verbatim into the bootstrap
// <script> as a template.JS value - json.Marshal HTML-escapes '<', '>' and
// '&' by default, which is what keeps this safe to embed in a <script>
// block despite the title coming from user-authored artifact data.
type agentPageData struct {
	Token        string
	RenderOrigin string
	ArtifactJSON template.JS
	MockEnabled  bool
	AgentEnabled bool
}

// agentPage serves the agent chat surface (Exh-jlbt): a build/modify-with-AI
// chat on the left, a live sandboxed preview of the session's artifact on the
// right. `?artifact=<id>` opens the page in modify mode bound to that
// artifact. Like the rest of the gallery it is one server-rendered document
// with vanilla-JS islands; streaming arrives over SSE from the session's Pi
// sidecar.
func (ro *Router) agentPage(w http.ResponseWriter, r *http.Request) {
	artifactJSON := "null"
	if id := r.URL.Query().Get("artifact"); id != "" {
		if a, err := ro.cfg.Store.GetArtifact(r.Context(), id); err == nil && a != nil {
			j, _ := json.Marshal(map[string]string{"id": a.ID, "title": a.Title})
			artifactJSON = string(j)
		}
	}
	page, err := renderPage("agent", agentPageData{
		Token:        ro.cfg.AuthToken,
		RenderOrigin: ro.cfg.RenderOrigin,
		ArtifactJSON: template.JS(artifactJSON),
		MockEnabled:  ro.cfg.MockEnabled,
		AgentEnabled: ro.cfg.Agent != nil,
	})
	if err != nil {
		serverError(w, r, "agent page render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page)
}
