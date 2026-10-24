package api

import "net/http"

// Revoking credentials never releases the restrictions protecting saved data.
// This separate human action erases that data and releases only old policies;
// current grants continue to apply. It also works with acquisition disabled.
func (ro *Router) clearConnectionData(w http.ResponseWriter, r *http.Request) {
	if !ro.relayHumanRequest(w, r) {
		return
	}
	policy := ro.connectionPolicy()
	if policy == nil {
		writeError(w, http.StatusServiceUnavailable, "Connection policy is unavailable.")
		return
	}
	ro.connectionDataMu.Lock()
	defer ro.connectionDataMu.Unlock()
	if err := policy.ClearData(r.Context(), ownerIDFromCtx(r.Context()), urlParamID(r, "artifactID")); err != nil {
		connectionError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
