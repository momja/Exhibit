package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
)

// Owner-issued library credentials are managed only by a signed-in owner, or
// the operator on a no-login instance. The agent allowlist excludes these routes.

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

// mayManageAgentGrants reports whether this request is a person who may
// manage connections to their library. Issuance also requires MCPURL.
//
// A signed-in session may. So may the page credential of an instance with no
// login, which is the static token or nothing at all: there the person at the
// page and the operator are the same one, and the token already carries every
// route. On an instance with a login the static token is the operator's CLI
// credential, and issuing a standing credential over somebody's artifact is
// that person's call, so it is refused. An agent never may: a session steered
// by text Exhibit did not author must not be able to extend its own reach in
// time. (agentScopeAllows already refuses it; this says so again here.)
func (ro *Router) mayManageAgentGrants(r *http.Request) bool {
	p := principalFromCtx(r.Context())
	if p.ReadOnly {
		return false
	}
	switch p.Kind {
	case PrincipalSession:
		return true
	case PrincipalServiceToken:
		return !ro.loginEnabled()
	case PrincipalNone:
		return !ro.loginEnabled() && (!ro.cfg.Public.Enabled || ro.matchesServiceToken(bearerToken(r)))
	default:
		return false
	}
}

func normalizeAgentGrantLabel(label string) string {
	label = strings.TrimSpace(label)
	if r := []rune(label); len(r) > agentGrantLabelMax {
		label = string(r[:agentGrantLabelMax])
	}
	if label == "" {
		return defaultAgentGrantLabel
	}
	return label
}

func (ro *Router) createAgentGrant(w http.ResponseWriter, r *http.Request) {
	if ro.cfg.MCPURL == "" {
		http.NotFound(w, r)
		return
	}
	var req createAgentGrantRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !ro.mayManageAgentGrants(r) {
		writeError(w, http.StatusForbidden, "agent grants are issued from a signed-in session")
		return
	}
	token, digest, err := agentscope.NewPersistedToken()
	if err != nil {
		serverError(w, r, "mint agent grant token", err)
		return
	}
	createGrant := ro.cfg.Store.CreateAgentGrant
	if principalFromCtx(r.Context()).Kind == PrincipalSession {
		createGrant = ro.cfg.Store.CreateAgentGrantForUser
	}
	g, err := createGrant(r.Context(), ownerIDFromCtx(r.Context()),
		normalizeAgentGrantLabel(req.Label), "", digest)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		serverError(w, r, "create agent grant", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, createAgentGrantResponse{Grant: g, Token: token})
}

func (ro *Router) listAgentGrants(w http.ResponseWriter, r *http.Request) {
	if !ro.mayManageAgentGrants(r) {
		writeError(w, http.StatusForbidden, "agent grants require a signed-in session")
		return
	}
	grants, err := ro.cfg.Store.ListAgentGrants(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		serverError(w, r, "list agent grants", err)
		return
	}
	writeJSON(w, http.StatusOK, listAgentGrantsResponse{Grants: grants})
}

// deleteAgentGrant revokes one grant. The next request made with its token is
// a 401, because nothing resolves it any more.
func (ro *Router) deleteAgentGrant(w http.ResponseWriter, r *http.Request) {
	if !ro.mayManageAgentGrants(r) {
		writeError(w, http.StatusForbidden, "read-only visitor may not mutate")
		return
	}
	err := ro.cfg.Store.DeleteAgentGrant(r.Context(), ownerIDFromCtx(r.Context()),
		urlParamID(r, "grantID"))
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

// agentCredential describes the already authenticated grant; no extra lookup is
// needed. Ordinary owner credentials are deliberately not agent credentials.
func (ro *Router) agentCredential(w http.ResponseWriter, r *http.Request) {
	g := agentGrantFromCtx(r.Context())
	if g == nil {
		writeError(w, http.StatusForbidden, "an agent credential is required")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	scope := g.Scope()
	if scope.Library {
		writeJSON(w, http.StatusOK, map[string]string{
			"scope": "library", "grant_id": strings.TrimPrefix(g.SessionID(), "grant:"), "label": g.Prompt(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"scope": "artifact", "artifact_id": scope.ArtifactID})
}
