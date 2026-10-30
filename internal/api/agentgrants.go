package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
)

// Owner-issued agent grants (Exh-l2xj): the credential a person hands to an
// agent of their own (Claude Code, Cursor, ...) so it can edit one artifact
// through the API, with the reach a built-in Pi session has and no more.
//
// The three routes are the owner's and nobody else's. A recipient's lookup of
// the artifact fails and answers 404, as for an artifact that does not exist,
// and an agent never reaches them: `agent-grants` is not in agentSubResources,
// so a grant cannot mint, list or revoke grants.

// agentGrantLabelMax bounds a label. It is shown in a list and recorded on
// every version the grant writes.
const agentGrantLabelMax = 100

// defaultAgentGrantLabel names a grant its owner did not label.
const defaultAgentGrantLabel = "External agent"

type createAgentGrantRequest struct {
	Label string `json:"label"`
}

// createAgentGrantResponse carries the token, which is the only time it ever
// leaves the server: the store keeps a digest.
type createAgentGrantResponse struct {
	Grant *store.AgentGrant `json:"grant"`
	Token string            `json:"token"`
}

type listAgentGrantsResponse struct {
	Grants []store.AgentGrant `json:"grants"`
}

// mayIssueAgentGrant reports whether this request is a person who may hand
// out authority over their artifact.
//
// A signed-in session may. So may the page credential of an instance with no
// login, which is the static token or nothing at all: there the person at the
// page and the operator are the same one, and the token already carries every
// route. On an instance with a login the static token is the operator's CLI
// credential, and issuing a standing credential over somebody's artifact is
// that person's call, so it is refused. An agent never may: a session steered
// by text Exhibit did not author must not be able to extend its own reach in
// time. (agentScopeAllows already refuses it; this says so again here.)
func (ro *Router) mayIssueAgentGrant(r *http.Request) bool {
	p := principalFromCtx(r.Context())
	if p.ReadOnly {
		return false
	}
	switch p.Kind {
	case PrincipalSession:
		return true
	case PrincipalServiceToken, PrincipalNone:
		return !ro.loginEnabled()
	default:
		return false
	}
}

func normalizeAgentGrantLabel(label string) string {
	label = strings.Join(strings.Fields(label), " ")
	if r := []rune(label); len(r) > agentGrantLabelMax {
		label = string(r[:agentGrantLabelMax])
	}
	if label == "" {
		return defaultAgentGrantLabel
	}
	return label
}

func (ro *Router) createAgentGrant(w http.ResponseWriter, r *http.Request) {
	var req createAgentGrantRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !ro.mayIssueAgentGrant(r) {
		writeError(w, http.StatusForbidden, "agent grants are issued from a signed-in session")
		return
	}
	token, digest, err := agentscope.NewPersistedToken()
	if err != nil {
		serverError(w, r, "mint agent grant token", err)
		return
	}
	g, err := ro.cfg.Store.CreateAgentGrant(r.Context(), ownerIDFromCtx(r.Context()),
		urlParamID(r, "artifactID"), normalizeAgentGrantLabel(req.Label), digest)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, r, "create agent grant", err)
		return
	}
	writeJSON(w, http.StatusCreated, createAgentGrantResponse{Grant: g, Token: token})
}

func (ro *Router) listAgentGrants(w http.ResponseWriter, r *http.Request) {
	ownerID := ownerIDFromCtx(r.Context())
	id := urlParamID(r, "artifactID")
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "list agent grants artifact lookup", err)
		return
	}
	if a == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	grants, err := ro.cfg.Store.ListAgentGrants(r.Context(), ownerID, id)
	if err != nil {
		serverError(w, r, "list agent grants", err)
		return
	}
	writeJSON(w, http.StatusOK, listAgentGrantsResponse{Grants: grants})
}

// deleteAgentGrant revokes one grant. The next request made with its token is
// a 401, because nothing resolves it any more.
func (ro *Router) deleteAgentGrant(w http.ResponseWriter, r *http.Request) {
	if principalFromCtx(r.Context()).ReadOnly {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}
	err := ro.cfg.Store.DeleteAgentGrant(r.Context(), ownerIDFromCtx(r.Context()),
		urlParamID(r, "artifactID"), urlParamID(r, "grantID"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, r, "delete agent grant", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// agentCredentialPath is where an agent credential reads its own scope.
const agentCredentialPath = "/api/agent/credential"

type agentCredentialResponse struct {
	ArtifactID string `json:"artifact_id"`
}

// getAgentCredential tells an agent credential which artifact it reaches
// (Exh-q6xo), so a client configured with a URL and a token can find its
// target without being told the id separately. It reports the scope and
// nothing else; anybody but an agent gets 404.
func (ro *Router) getAgentCredential(w http.ResponseWriter, r *http.Request) {
	g := agentGrantFromCtx(r.Context())
	if g == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, agentCredentialResponse{ArtifactID: g.Scope().ArtifactID})
}

// --- the detail page's External agents panel --------------------------------

// agentGrantsPanelView is the owner's list of grants over one artifact.
type agentGrantsPanelView struct {
	ArtifactID string
	// MayIssue hides the issue form from a principal that would be refused.
	MayIssue bool
	// AppOrigin is what an agent's config names as the instance.
	AppOrigin string
	Grants    []agentGrantRowView
}

type agentGrantRowView struct {
	ID       string
	Label    string
	Created  string
	LastUsed string
}

func (ro *Router) agentGrantsPanel(r *http.Request, a *store.Artifact) (agentGrantsPanelView, error) {
	grants, err := ro.cfg.Store.ListAgentGrants(r.Context(), a.OwnerID, a.ID)
	if err != nil {
		return agentGrantsPanelView{}, err
	}
	view := agentGrantsPanelView{ArtifactID: a.ID, MayIssue: ro.mayIssueAgentGrant(r), AppOrigin: ro.cfg.AppOrigin}
	now := time.Now()
	for _, g := range grants {
		row := agentGrantRowView{ID: g.ID, Label: g.Label, Created: g.CreatedAt.Format("Jan 2, 2006"), LastUsed: "never used"}
		if g.LastUsedAt != nil {
			row.LastUsed = "used " + sinceLabel(now.Sub(*g.LastUsedAt))
		}
		view.Grants = append(view.Grants, row)
	}
	return view, nil
}

// sinceLabel says how long ago something was, to the minute.
func sinceLabel(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	default:
		return plural(int(d/(24*time.Hour)), "day") + " ago"
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// agentGrantsPanelPartial re-renders the panel's list after an issue or a
// revoke, for the reason sharePanelPartial exists: the detail page holds a
// live artifact frame that a reload would restart.
func (ro *Router) agentGrantsPanelPartial(w http.ResponseWriter, r *http.Request) {
	a, err := ro.cfg.Store.GetArtifact(r.Context(), ownerIDFromCtx(r.Context()), r.URL.Query().Get("artifact"))
	if err != nil {
		fragmentServerError(w, r, "agent grants partial lookup", err)
		return
	}
	if a == nil {
		fragmentNotFound(w)
		return
	}
	view, err := ro.agentGrantsPanel(r, a)
	if err != nil {
		fragmentServerError(w, r, "agent grants partial list", err)
		return
	}
	fragment, err := renderPage("agentGrantsPanelBody", view)
	if err != nil {
		fragmentServerError(w, r, "agent grants partial render", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, fragment)
}
