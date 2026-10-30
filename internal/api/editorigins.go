package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/momja/Exhibit/internal/store"
)

type editOriginsView struct {
	ID         string   `json:"-"`
	Allowlist  []string `json:"allowlist"`
	Blocked    []string `json:"blocked"`
	Unapproved []string `json:"unapproved"`
}

func newEditOriginsView(footprint []string, decisions []store.OriginDecision) editOriginsView {
	view := editOriginsView{Allowlist: []string{}, Blocked: []string{}}
	for _, decision := range decisions {
		switch decision.Decision {
		case store.DecisionAllow:
			view.Allowlist = append(view.Allowlist, decision.Origin)
		case store.DecisionBlock:
			view.Blocked = append(view.Blocked, decision.Origin)
		}
	}
	view.Unapproved = diffOrigins(footprint, view.Allowlist, view.Blocked)
	return view
}

// Render a working copy without persisting it. The editor includes its draft
// so a server-rendered refresh cannot replace edits made during a save. Without
// a draft, return the saved decisions and source footprint, like the full page.
func (ro *Router) editOriginsPartial(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	id := r.URL.Query().Get("artifact")
	artifact, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "edit origins lookup", err)
		return
	}
	if artifact == nil {
		ro.notFound(w, r)
		return
	}
	var view editOriginsView
	if draft := r.URL.Query().Get("draft"); r.URL.Query().Has("draft") {
		if err := json.Unmarshal([]byte(draft), &view); err != nil {
			http.Error(w, "invalid origin draft", http.StatusBadRequest)
			return
		}
	} else {
		decisions, err := ro.cfg.Store.ListOriginDecisions(r.Context(), ownerID, id)
		if err != nil {
			serverError(w, r, "edit origins decisions", err)
			return
		}
		source, err := ro.cfg.Blob.Get(r.Context(), artifact.SourceBlobID)
		if err != nil {
			serverError(w, r, "edit origins source", err)
			return
		}
		defer source.Close()
		body, err := io.ReadAll(source)
		if err != nil {
			serverError(w, r, "edit origins read source", err)
			return
		}
		view = newEditOriginsView(networkFootprint(string(body), ro.cfg.RenderOrigin), decisions)
	}
	view.ID = artifact.ID
	page, err := renderPage("editOriginLists", view)
	if err != nil {
		serverError(w, r, "edit origins render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}
