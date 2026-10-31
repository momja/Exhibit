package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// The account panel lists only the current owner's durable connections.
// Newly issued grant tokens are never part of a page or fragment response.
type agentGrantsPanelView struct {
	MCPURL         string
	RefreshHeaders string
	Grants         []agentGrantRowView
}

type agentGrantRowView struct {
	ID       string
	Label    string
	Client   string
	Created  string
	LastUsed string
}

func (ro *Router) agentGrantsPanel(r *http.Request) (*agentGrantsPanelView, error) {
	if !ro.mayManageAgentGrants(r) {
		return nil, nil
	}
	grants, err := ro.cfg.Store.ListAgentGrants(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		return nil, err
	}
	if ro.cfg.MCPURL == "" && len(grants) == 0 {
		return nil, nil
	}
	// The htmx fragment needs the same request-derived authority as apiFetch.
	// Sessions use their cookie; only a no-login operator page gets the static
	// token. Scope the header to this region rather than all htmx requests.
	headers := map[string]string{}
	if token := ro.pageCredentials(r).Token; token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	encodedHeaders, err := json.Marshal(headers)
	if err != nil {
		return nil, err
	}
	view := &agentGrantsPanelView{MCPURL: ro.cfg.MCPURL, RefreshHeaders: string(encodedHeaders)}
	for _, grant := range grants {
		row := agentGrantRowView{
			ID: grant.ID, Label: grant.Label, Client: grant.Client,
			Created: grant.CreatedAt.UTC().Format(time.DateOnly), LastUsed: "Never used",
		}
		if row.Client == "" {
			row.Client = "Pasted token"
		}
		if grant.LastUsedAt != nil {
			row.LastUsed = grant.LastUsedAt.UTC().Format("2006-01-02 15:04 UTC")
		}
		view.Grants = append(view.Grants, row)
	}
	return view, nil
}

func (ro *Router) agentGrantsPanelPartial(w http.ResponseWriter, r *http.Request) {
	view, err := ro.agentGrantsPanel(r)
	if err != nil {
		fragmentServerError(w, r, "agent grants partial list", err)
		return
	}
	if view == nil {
		if !ro.mayManageAgentGrants(r) {
			fragmentNotFound(w)
			return
		}
		// Removing the last grant on a disabled instance removes the entire
		// section, rather than leaving a stale row after a failed list refresh.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("HX-Retarget", "#agent-grants-panel")
		w.Header().Set("HX-Reswap", "outerHTML")
		w.WriteHeader(http.StatusOK)
		return
	}
	fragment, err := renderPage("agentGrantsPanelBody", view)
	if err != nil {
		fragmentServerError(w, r, "agent grants partial render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, fragment)
}
