package api

import (
	"context"
	"net/http"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

type vendorUsageStore interface {
	VendorUsage(context.Context, time.Time) ([]store.VendorUsage, error)
}

// The ledger contains aggregate attempt counts, not provider payloads or
// credentials. It remains visible to the operator after an engine is disabled.
func (ro *Router) connectorUsage(w http.ResponseWriter, r *http.Request) {
	s, ok := ro.cfg.Store.(vendorUsageStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "connector accounting is unavailable")
		return
	}
	now := time.Now().UTC()
	usage, err := s.VendorUsage(r.Context(), now)
	if err != nil {
		serverError(w, r, "connector accounting", err)
		return
	}
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"period_start": start.Format(time.RFC3339), "period_end": now.Format(time.RFC3339), "usage": usage})
}
