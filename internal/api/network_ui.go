package api

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
)

type networkApprovalOrigin struct {
	Origin  string
	Blocked bool
}

type networkApprovalView struct {
	ID       string
	New      bool
	Origins  []networkApprovalOrigin
	Warnings []scanner.Warning
}

func (ro *Router) networkUIArtifact(w http.ResponseWriter, r *http.Request) (*store.Artifact, string) {
	artifact, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), r.URL.Query().Get("artifact"))
	if err != nil {
		fragmentServerError(w, r, "load network review artifact", err)
		return nil, ""
	}
	if artifact == nil {
		fragmentNotFound(w)
		return nil, ""
	}
	body, err := ro.readBlobString(r.Context(), artifact.SourceBlobID)
	if err != nil {
		fragmentServerError(w, r, "read network review source", err)
		return nil, ""
	}
	return artifact, body
}

func writeNetworkFragment(w http.ResponseWriter, r *http.Request, name string, data any) {
	fragment, err := renderPage(name, data)
	if err != nil {
		fragmentServerError(w, r, "render network review", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, fragment)
}

func (ro *Router) networkWarningsPartial(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("artifact") == "" {
		writeNetworkFragment(w, r, "networkWarnings", []scanner.Warning{})
		return
	}
	artifact, body := ro.networkUIArtifact(w, r)
	if artifact != nil {
		writeNetworkFragment(w, r, "networkWarnings", scanner.Warnings(body, artifact.SourceURL))
	}
}

func (ro *Router) networkApprovalPartial(w http.ResponseWriter, r *http.Request) {
	artifact, body := ro.networkUIArtifact(w, r)
	if artifact == nil {
		return
	}
	decisions, err := ro.cfg.Store.ListOriginDecisions(r.Context(), ownerIDFromCtx(r.Context()), artifact.ID)
	if err != nil {
		fragmentServerError(w, r, "load network review decisions", err)
		return
	}
	approved, blocked := make(map[string]bool), make(map[string]bool)
	for _, decision := range decisions {
		approved[decision.Origin] = decision.Decision == store.DecisionAllow
		blocked[decision.Origin] = decision.Decision == store.DecisionBlock
	}
	view := networkApprovalView{ID: artifact.ID, New: r.URL.Query().Get("mode") == "new", Warnings: scanner.Warnings(body, artifact.SourceURL)}
	origins := networkFootprint(body, ro.cfg.RenderOrigin)
	sort.Strings(origins)
	for _, origin := range origins {
		if !approved[origin] {
			view.Origins = append(view.Origins, networkApprovalOrigin{Origin: origin, Blocked: blocked[origin]})
		}
	}
	writeNetworkFragment(w, r, "networkApproval", view)
}

type agentConnectionRequestsView struct {
	Requests  []connectionRequestView
	ManageURL string
}

func (ro *Router) agentConnectionRequestsPartial(w http.ResponseWriter, r *http.Request) {
	view := agentConnectionRequestsView{}
	artifactID := r.URL.Query().Get("artifact")
	requests, ok := ro.cfg.Store.(store.ConnectionRequestStore)
	if artifactID == "" || !ok || ro.cfg.Connections == nil || !ro.cfg.Relay.Enabled {
		writeNetworkFragment(w, r, "agentConnectionRequests", view)
		return
	}
	artifact, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), artifactID)
	if err != nil {
		fragmentServerError(w, r, "load connection request artifact", err)
		return
	}
	if artifact == nil {
		fragmentNotFound(w)
		return
	}
	pending, err := requests.ListArtifactConnectionRequests(r.Context(), ownerIDFromCtx(r.Context()), artifactID)
	if err != nil {
		fragmentServerError(w, r, "load connection requests", err)
		return
	}
	page, err := ro.connectionPageView(r)
	if err != nil {
		fragmentServerError(w, r, "load connection request actions", err)
		return
	}
	for _, request := range pending {
		view.Requests = append(view.Requests, page.requestView(request))
	}
	view.ManageURL = "/connections?artifact=" + url.QueryEscape(artifactID)
	writeNetworkFragment(w, r, "agentConnectionRequests", view)
}
