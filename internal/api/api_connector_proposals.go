package api

import (
	"net/http"

	"github.com/momja/Exhibit/internal/connectors"
)

func (ro *Router) agentProposeAPIConnector(w http.ResponseWriter, r *http.Request) {
	if ro.connectionRequestStore(w) == nil {
		return
	}
	var spec connectors.APIProposalSpec
	if !decodeStrictJSON(w, r, &spec) {
		return
	}
	proposal, err := ro.cfg.Connections.ProposeAPI(r.Context(), ownerIDFromCtx(r.Context()), urlParamID(r, "artifactID"), spec)
	if err != nil {
		connectionError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]any{"proposal": proposal})
}
