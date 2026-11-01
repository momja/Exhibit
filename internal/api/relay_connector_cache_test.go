package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayConnectorCacheRespectsBrowserCacheModeAndQuota(t *testing.T) {
	for _, tt := range []struct {
		name           string
		mode           string
		requestControl string
	}{
		{name: "default"},
		{name: "reload", mode: "reload"},
		{name: "no-cache", mode: "no-cache"},
		{name: "no-store", mode: "no-store"},
		{name: "reload preserves explicit no-store", mode: "reload", requestControl: "no-store"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			noStore := tt.mode == "no-store" || tt.requestControl == "no-store"
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if noStore && calls.Load() <= 2 {
					assert.Equal(t, "no-store", r.Header.Get("Cache-Control"))
				}
				fmt.Fprint(w, "connected data")
			}))
			defer upstream.Close()
			ro := newTestRouter(t)
			ro.cfg.Relay = RelayConfig{Enabled: true, GateFactory: egress.NewForTests}
			id := createTestArtifact(t, ro, "Cached connected tool")
			require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, upstream.URL, store.DecisionAllow, "test"))
			ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
			dailyLimit := 3
			if tt.mode == "" {
				dailyLimit = 1
			}
			connection, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{Name: "Cached account", Recipe: "bearer", Origin: upstream.URL, Secret: "browser-cache-test-secret", CacheMinutes: 5, DailyLimit: dailyLimit}, connectors.UnlimitedLimits())
			require.NoError(t, err)
			_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
			require.NoError(t, err)
			s, ok := ro.relayStore()
			require.True(t, ok)
			require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Allowed: true}))
			input := relayRequest{URL: upstream.URL + "/data", Method: http.MethodGet, Cache: tt.mode}
			if tt.requestControl != "" {
				input.Headers = map[string]string{"Cache-Control": tt.requestControl}
			}
			for range 2 {
				response := relayCall(t, ro, id, input, "")
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			}
			wantCalls := int32(2)
			if tt.mode == "" {
				wantCalls = 1
			}
			assert.Equal(t, wantCalls, calls.Load(), "ordinary GETs reuse the response without spending another quota unit")
			input.Cache = ""
			input.Headers = nil
			response := relayCall(t, ro, id, input, "")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			if noStore {
				wantCalls++
			}
			assert.Equal(t, wantCalls, calls.Load(), "reload/no-cache replace the normal cache entry; no-store never populates it")
		})
	}
}
