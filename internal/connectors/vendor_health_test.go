package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nangoHealthFixture struct {
	provider      *httptest.Server
	managed       *httptest.Server
	calls         atomic.Int32
	lookups       atomic.Int32
	accountStatus atomic.Int32
	foreignOwner  atomic.Bool
	deletes       atomic.Int32
}

func newNangoHealthFixture(t *testing.T, s *Service, handler http.HandlerFunc) (*nangoHealthFixture, *Connection) {
	t.Helper()
	f := &nangoHealthFixture{}
	f.accountStatus.Store(http.StatusOK)
	f.provider = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		assert.Equal(t, "Bearer nango-health-access-4913", r.Header.Get("Authorization"))
		handler(w, r)
	}))
	t.Cleanup(f.provider.Close)
	f.managed = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lookups.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if f.accountStatus.Load() != http.StatusOK {
			w.WriteHeader(int(f.accountStatus.Load()))
			fmt.Fprint(w, `{"error":"private vendor detail"}`)
			return
		}
		assert.Equal(t, "/connections/health-account", r.URL.Path)
		if r.Method == http.MethodDelete {
			f.deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		owner := "tenant:exhibit:1"
		if f.foreignOwner.Load() {
			owner = "tenant:exhibit:2"
		}
		fmt.Fprintf(w, `{"connection_id":"health-account","provider_config_key":"notes","tags":{"exhibit_owner":%q},"end_user":{"id":%q},"credentials":{"type":"OAUTH2","access_token":"nango-health-access-4913","refresh_token":"nango-health-refresh-3097"}}`, owner, owner)
	}))
	t.Cleanup(f.managed.Close)
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{BaseURL: f.managed.URL, SecretKey: "nango-operator-private-key", ComposioAPIKey: "second-operator-private-key", UserNamespace: "tenant", Providers: map[string]VendorProvider{
		"notes": {Name: "Notes", Engine: "nango", Origins: []string{f.provider.URL}, CacheMinutes: 5},
	}}
	connection, err := s.Create(context.Background(), 1, CreateRequest{Name: "Managed notes", Recipe: "vendor", VendorProvider: "notes", VendorConnectionID: "health-account"}, UnlimitedLimits())
	require.NoError(t, err)
	return f, connection
}

func TestNangoOperatorKeyReflectionUsesNeutralRedactionAndSurvivesCacheKeyRotation(t *testing.T) {
	s, _ := testService(t)
	f, c := newNangoHealthFixture(t, s, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"operator":"nango-operator-private-key","other_operator":"second-operator-private-key","provider":"nango-health-access-4913"}`)
	})
	_, err := s.Grant(context.Background(), 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	handle, err := s.Standin(context.Background(), 1, "app", c.ID)
	require.NoError(t, err)
	gate := testGate(t, f.provider.URL)
	response, err := doRequest(t, s, 1, gate, f.provider.URL+"/data")
	require.NoError(t, err)
	var data map[string]string
	require.NoError(t, json.Unmarshal(response.Body, &data))
	assert.Equal(t, "[credential redacted]", data["operator"])
	assert.Equal(t, "[credential redacted]", data["other_operator"])
	assert.Equal(t, handle.Value, data["provider"])
	before := f.lookups.Load()
	s.cfg.Vendor.SecretKey = "rotated-nango-operator-key"
	s.cfg.Vendor.ComposioAPIKey = "rotated-other-operator-key"
	response, err = doRequest(t, s, 1, gate, f.provider.URL+"/data")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(response.Body, &data))
	assert.Equal(t, "[credential redacted]", data["operator"])
	assert.Equal(t, "[credential redacted]", data["other_operator"])
	assert.Equal(t, handle.Value, data["provider"])
	assert.NotContains(t, string(response.Body), "operator-private-key")
	assert.Equal(t, int32(1), f.calls.Load())
	assert.Equal(t, before, f.lookups.Load(), "cached delivery must not make a management call")
}

func TestManagedDeliveryStatusesNormalizeAndPersistReconnectAcrossBothEngines(t *testing.T) {
	for _, engine := range []string{"nango", "composio"} {
		for _, scenario := range []struct {
			status int
			code   string
		}{{401, VendorNeedsReconnect}, {429, VendorRateLimited}, {403, VendorRefused}, {503, VendorUpstreamError}} {
			t.Run(fmt.Sprintf("%s/%d", engine, scenario.status), func(t *testing.T) {
				s, _ := testService(t)
				var connection *Connection
				var target string
				var calls func() int32
				if engine == "nango" {
					f, c := newNangoHealthFixture(t, s, func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Retry-After", "27")
						w.WriteHeader(scenario.status)
						fmt.Fprint(w, `{"detail":"nango-health-access-4913 must stay private"}`)
					})
					connection, target, calls = c, f.provider.URL+"/data", f.calls.Load
				} else {
					f := newComposioFixture(t, s, "https://api.example.test")
					connection = completeComposio(t, s)
					f.change(func(state *composioFixtureState) {
						state.ProxyStatus = scenario.status
						state.ProxyHeaders = map[string]string{"Retry-After": "27"}
					})
					target, calls = "https://api.example.test/data", f.proxies.Load
				}
				ctx := context.Background()
				_, err := s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
				require.NoError(t, err)
				origin := strings.TrimSuffix(target, "/data")
				gate := testGate(t, origin)
				response, err := doRequest(t, s, 1, gate, target)
				require.Nil(t, response)
				assert.Equal(t, scenario.code, ErrorCode(err))
				var failure *VendorError
				require.ErrorAs(t, err, &failure)
				assert.Equal(t, "27", failure.RetryAfter)
				assert.NotContains(t, err.Error(), "must stay private")
				record, err := s.cfg.Store.GetConnection(ctx, 1, connection.ID)
				require.NoError(t, err)
				if scenario.code == VendorNeedsReconnect {
					assert.Equal(t, "reconnect", record.Status)
					list, err := s.List(ctx, 1)
					require.NoError(t, err)
					require.Len(t, list, 1)
					assert.Equal(t, VendorNeedsReconnect, list[0].Status)
					assert.True(t, list[0].ReconnectRecommended)
					grants, err := s.Grants(ctx, 1, "app")
					require.NoError(t, err)
					require.Len(t, grants, 1)
					assert.Equal(t, VendorNeedsReconnect, grants[0].Connection.Status)
					before := calls()
					_, err = doRequest(t, s, 1, gate, target)
					assert.Equal(t, VendorNeedsReconnect, ErrorCode(err))
					assert.Equal(t, before, calls(), "an unhealthy binding cannot become a public unauthenticated fetch")
					_, err = s.Standin(ctx, 1, "app", connection.ID)
					assert.Equal(t, VendorNeedsReconnect, ErrorCode(err))
					policies, err := s.DataPolicies(ctx, 1, "app")
					require.NoError(t, err)
					require.Len(t, policies, 1)
				} else {
					assert.Equal(t, "active", record.Status)
				}
			})
		}
	}
}

func TestManagedExplicitUnhealthyStatusPersistsReconnectAndStopsCachedDelivery(t *testing.T) {
	for _, engine := range []string{"nango", "composio"} {
		t.Run(engine, func(t *testing.T) {
			s, _ := testService(t)
			var connection *Connection
			var target string
			var makeRevoked func()
			var calls func() int32
			if engine == "nango" {
				f, c := newNangoHealthFixture(t, s, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":true}`) })
				connection, target, calls = c, f.provider.URL+"/data", f.calls.Load
				makeRevoked = func() { f.accountStatus.Store(http.StatusNotFound) }
			} else {
				f := newComposioFixture(t, s, "https://api.example.test")
				p := s.cfg.Vendor.Providers["notes-cloud"]
				p.CacheMinutes = 5
				s.cfg.Vendor.Providers["notes-cloud"] = p
				connection = completeComposio(t, s)
				target, calls = "https://api.example.test/data", f.proxies.Load
				makeRevoked = func() { f.change(func(state *composioFixtureState) { state.Status = "REVOKED" }) }
			}
			ctx := context.Background()
			_, err := s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: connection.ID})
			require.NoError(t, err)
			gate := testGate(t, strings.TrimSuffix(target, "/data"))
			_, err = doRequest(t, s, 1, gate, target)
			require.NoError(t, err)
			makeRevoked()
			status, err := s.VendorStatus(ctx, 1, connection.ID)
			require.NoError(t, err)
			assert.Equal(t, "revoked", status)
			before := calls()
			_, err = doRequest(t, s, 1, gate, target)
			assert.Equal(t, VendorNeedsReconnect, ErrorCode(err))
			assert.Equal(t, before, calls(), "the old cached response must be inaccessible through an unhealthy grant")
			list, err := s.List(ctx, 1)
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, VendorNeedsReconnect, list[0].Status)
			assert.True(t, list[0].ReconnectRecommended)
		})
	}
}

func TestNangoRevokeChecksPinnedOwnerAndHandlesAnAlreadyDeletedRemoteAccount(t *testing.T) {
	for _, scenario := range []string{"owned", "foreign", "deleted"} {
		t.Run(scenario, func(t *testing.T) {
			s, _ := testService(t)
			f, c := newNangoHealthFixture(t, s, func(http.ResponseWriter, *http.Request) {})
			ctx := context.Background()
			_, err := s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
			require.NoError(t, err)
			if scenario == "foreign" {
				f.foreignOwner.Store(true)
			}
			if scenario == "deleted" {
				f.accountStatus.Store(http.StatusNotFound)
			}
			err = s.Revoke(ctx, 1, c.ID)
			if scenario == "foreign" {
				assert.Equal(t, VendorRefused, ErrorCode(err))
				assert.Zero(t, f.deletes.Load(), "a stale reference must not revoke another owner")
				_, err = s.cfg.Store.GetConnection(ctx, 1, c.ID)
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
				_, err = s.cfg.Store.GetConnection(ctx, 1, c.ID)
				assert.ErrorIs(t, err, store.ErrNotFound)
				if scenario == "owned" {
					assert.Equal(t, int32(1), f.deletes.Load())
				} else {
					assert.Zero(t, f.deletes.Load())
				}
			}
			policies, err := s.DataPolicies(ctx, 1, "app")
			require.NoError(t, err)
			require.Len(t, policies, 1)
		})
	}
}

func TestVendorReferencesShareTheDocumented256CharacterBound(t *testing.T) {
	for _, length := range []int{200, 201, 256, 257} {
		ref := "ca_" + strings.Repeat("a", length-3)
		assert.Equal(t, length <= 256, validVendorReference(ref), length)
		assert.Equal(t, length <= 256, composioReference(ref), length)
	}
}
