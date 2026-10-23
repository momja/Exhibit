package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectorCostCountersAreOperatorOnlyAndSurviveDisabledEngines(t *testing.T) {
	in := newAdminInstance(t)
	s := in.st.(*store.SQLiteStore)
	now := time.Now().UTC()
	require.NoError(t, s.RecordVendorCall(context.Background(), "nango", "operator-provider", "management", now))
	require.NoError(t, s.RecordVendorCall(context.Background(), "composio", "other-provider", "forward", now))
	const path = "/api/admin/connector-usage"
	w := in.do(t, http.MethodGet, path, in.memberCookie, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "operator-provider")
	w = in.do(t, http.MethodGet, path, in.adminCookie, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var report struct {
		Start string              `json:"period_start"`
		Usage []store.VendorUsage `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
	assert.Equal(t, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), report.Start)
	require.Len(t, report.Usage, 2)
	assert.Equal(t, int64(1), report.Usage[0].ForwardCalls)
	assert.Equal(t, int64(1), report.Usage[1].ManagementCalls)
	// Operator reporting is available even with relay and engines disabled.
	assert.Nil(t, in.ro.cfg.Connections)
	in.ro.cfg.Relay.Enabled = true
	w = in.do(t, http.MethodGet, "/connections", in.adminCookie, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "operator-provider")
	assert.Contains(t, w.Body.String(), "Operator only")
	w = in.do(t, http.MethodGet, "/connections", in.memberCookie, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "operator-provider")
	assert.NotContains(t, w.Body.String(), "Operator only")
}
