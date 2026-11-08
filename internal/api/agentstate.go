package api

import (
	"net/http"
)

// Agents and saved data (Exh-dabd). An agent runs on an inference provider,
// so a state tool is a way for what a person typed into their tools to leave
// the instance. Whether it may is the owner's decision, made once for their
// library here and optionally per artifact through PATCH agent_state_access.
// Enforcement is in the state routes (state.go: artifactExists).

type agentStateAccessDefault struct {
	Allowed *bool `json:"allowed"`
}

func (ro *Router) getAgentStateAccessDefault(w http.ResponseWriter, r *http.Request) {
	allowed, err := ro.cfg.Store.AgentStateAccessDefault(r.Context(), ownerIDFromCtx(r.Context()))
	if err != nil {
		serverError(w, r, "get agent state access default", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"allowed": allowed})
}

func (ro *Router) putAgentStateAccessDefault(w http.ResponseWriter, r *http.Request) {
	// agentScopeAllows already refuses this route to every agent credential.
	// Checked again here because this is the one switch an agent must never
	// reach, and a route allowlist is a rule a later edit can widen.
	if principalFromCtx(r.Context()).Kind == PrincipalAgentGrant {
		writeError(w, http.StatusForbidden, "agent access to saved data is set by the owner, not by an agent")
		return
	}
	var req agentStateAccessDefault
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Allowed == nil {
		writeError(w, http.StatusBadRequest, "allowed must be a boolean")
		return
	}
	if err := ro.cfg.Store.SetAgentStateAccessDefault(r.Context(), ownerIDFromCtx(r.Context()), *req.Allowed); err != nil {
		serverError(w, r, "set agent state access default", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"allowed": *req.Allowed})
}
