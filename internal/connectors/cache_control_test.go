package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionCacheHonorsRequestCacheControl(t *testing.T) {
	for _, tt := range []struct {
		name      string
		control   []string
		finalBody string
	}{
		{"refresh", []string{"no-cache"}, "3"},
		{"refresh multiple fields", []string{"max-age=120", "No-CaChe"}, "3"},
		{"no storage", []string{"max-age=120, no-store"}, "1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := testService(t)
			ctx := context.Background()
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, calls.Add(1))
			}))
			defer provider.Close()
			connection, err := s.Create(ctx, 1, CreateRequest{Name: "Cache directives", Recipe: "bearer", Origin: provider.URL, Secret: "cache-control-test-secret", CacheMinutes: 5}, UnlimitedLimits())
			require.NoError(t, err)
			_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
			require.NoError(t, err)
			gate := testGate(t, provider.URL)
			response, err := doRequest(t, s, 1, gate, provider.URL)
			require.NoError(t, err)
			assert.Equal(t, "1", string(response.Body))
			for i := 2; i <= 3; i++ {
				req, err := http.NewRequest(http.MethodGet, provider.URL, nil)
				require.NoError(t, err)
				req.Header["Cache-Control"] = tt.control
				response, err = s.Do(ctx, 1, "app", req, gate)
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprint(i), string(response.Body), "each explicit refresh reaches the provider")
			}
			response, err = doRequest(t, s, 1, gate, provider.URL)
			require.NoError(t, err)
			assert.Equal(t, tt.finalBody, string(response.Body), "no-cache updates the entry; no-store leaves it alone")
			assert.EqualValues(t, 3, calls.Load())
		})
	}
}

func TestConnectionCacheHonorsResponseCacheControl(t *testing.T) {
	for _, control := range []string{"no-store", "public, No-StOrE", "no-cache", `no-cache="Authorization, X-Account"`} {
		t.Run(control, func(t *testing.T) {
			s, _ := testService(t)
			ctx := context.Background()
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) > 1 {
					w.Header().Add("Cache-Control", "max-age=120")
					w.Header().Add("Cache-Control", control)
				}
				fmt.Fprint(w, calls.Load())
			}))
			defer provider.Close()
			connection, err := s.Create(ctx, 1, CreateRequest{Name: "Response directives", Recipe: "bearer", Origin: provider.URL, Secret: "response-cache-control-secret", CacheMinutes: 5}, UnlimitedLimits())
			require.NoError(t, err)
			_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
			require.NoError(t, err)
			gate := testGate(t, provider.URL)
			// A provider can withdraw permission to cache an existing response
			// during a forced refresh; the old entry must then disappear too.
			for i := 1; i <= 4; i++ {
				req, err := http.NewRequest(http.MethodGet, provider.URL, nil)
				require.NoError(t, err)
				if i == 2 {
					req.Header.Set("Cache-Control", "no-cache")
				}
				response, err := s.Do(ctx, 1, "app", req, gate)
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprint(i), string(response.Body))
			}
			assert.EqualValues(t, 4, calls.Load())
		})
	}
}

func TestConnectionCacheDiscardsPreviouslyStoredNoStoreResponses(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var calls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, "fresh")
	}))
	defer provider.Close()
	connection, err := s.Create(ctx, 1, CreateRequest{Name: "Legacy entry", Recipe: "bearer", Origin: provider.URL, Secret: "legacy-cache-control-secret", CacheMinutes: 5}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	old, err := encodeConnectionCache(&egress.Response{Status: http.StatusOK, Header: http.Header{"Cache-Control": {"no-store"}}, Body: []byte("obsolete"), URL: provider.URL}, credential{})
	require.NoError(t, err)
	sealed, err := s.cfg.Box.Encrypt(string(old))
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, provider.URL, nil)
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.PutConnectionCache(ctx, connection.ID, requestCacheKey(req), sealed, s.cfg.Now(), s.cfg.Now().Add(time.Hour)))
	response, err := s.Do(ctx, 1, "app", req, testGate(t, provider.URL))
	require.NoError(t, err)
	assert.Equal(t, "fresh", string(response.Body))
	assert.EqualValues(t, 1, calls.Load())
}
