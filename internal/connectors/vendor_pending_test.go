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
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedCompletionQuotaFailureRevokesUnsavedAccount(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	ctx := context.Background()
	session, err := s.BeginVendorBound(ctx, 1, "notes-cloud", Limits{Vendor: 1}, "browser")
	require.NoError(t, err)
	_, ready, err := s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", Limits{Vendor: 0})
	require.ErrorIs(t, err, store.ErrConnectionLimit)
	assert.False(t, ready)
	assert.Equal(t, int32(1), f.deletes.Load())
	connections, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, connections)
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, pending)
	_, _, err = s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", UnlimitedLimits())
	assert.ErrorIs(t, err, ErrInvalid, "a retired authorization cannot be adopted by a later retry")
}

func TestFailedManagedCleanupSurvivesRestartAndRetries(t *testing.T) {
	s, path := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	ctx := context.Background()
	session, err := s.BeginVendorBound(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser")
	require.NoError(t, err)
	f.change(func(v *composioFixtureState) { v.RevokeHTTP = http.StatusServiceUnavailable })
	_, _, err = s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", Limits{Vendor: 0})
	require.ErrorIs(t, err, store.ErrConnectionLimit)
	assert.Zero(t, f.deletes.Load())
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Contains(t, pending[0].RequestJSON, "ca_fixture")
	assert.NotContains(t, pending[0].RequestJSON, "composio-operator-private-key")

	db, err := store.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	config := s.cfg
	config.Store = db
	restarted := New(config)
	f.change(func(v *composioFixtureState) { v.RevokeHTTP = http.StatusOK })
	count, err := restarted.PruneIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, int32(1), f.deletes.Load())
	pending, err = db.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestAbandonedManagedAuthorizationIsCleanedAfterRestart(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	ctx := context.Background()
	_, err := s.BeginVendorBound(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser")
	require.NoError(t, err)
	config := s.cfg
	config.Now = func() time.Time { return time.Now().Add(time.Hour) }
	count, err := New(config).PruneIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, int32(1), f.deletes.Load())
}

func TestStaleManagedRepairCleansOnlyItsReplacement(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	ctx := context.Background()
	original, err := s.saveVendor(ctx, 1, "Notes", "notes-cloud", "tenant:exhibit:1", "ca_old", s.cfg.Vendor.Providers["notes-cloud"], UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: original.ID})
	require.NoError(t, err)
	session, err := s.BeginVendorFor(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser", SetupContext{ArtifactID: "app", ConnectionID: original.ID})
	require.NoError(t, err)
	record, err := s.cfg.Store.GetConnection(ctx, 1, original.ID)
	require.NoError(t, err)
	sealed, err := s.cfg.Box.Encrypt(`{"vendor_connection_id":"ca_newer"}`)
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.RepairConnection(ctx, record, sealed, "ca_newer"))
	_, _, err = s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", UnlimitedLimits())
	require.ErrorIs(t, err, ErrInvalid)
	record, err = s.cfg.Store.GetConnection(ctx, 1, original.ID)
	require.NoError(t, err)
	assert.Equal(t, "ca_newer", record.ExternalRef)
	grants, err := s.Grants(ctx, 1, "app")
	require.NoError(t, err)
	assert.Len(t, grants, 1)
	assert.Equal(t, int32(1), f.deletes.Load())
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Contains(t, f.operations, "POST /api/v3.1/connected_accounts/ca_fixture/revoke")
	assert.NotContains(t, f.operations, "POST /api/v3.1/connected_accounts/ca_newer/revoke")
}

func TestManagedRepairCanRetryRetiringPreviousAccount(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	ctx := context.Background()
	original, err := s.saveVendor(ctx, 1, "Notes", "notes-cloud", "tenant:exhibit:1", "ca_old", s.cfg.Vendor.Providers["notes-cloud"], UnlimitedLimits())
	require.NoError(t, err)
	session, err := s.BeginVendorFor(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser", SetupContext{ConnectionID: original.ID})
	require.NoError(t, err)
	f.change(func(v *composioFixtureState) { v.RevokeHTTP = http.StatusServiceUnavailable })
	_, _, err = s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", UnlimitedLimits())
	assertComposioFailure(t, err, VendorUpstreamError)
	assert.Zero(t, f.deletes.Load())
	f.change(func(v *composioFixtureState) { v.RevokeHTTP = http.StatusOK })
	repaired, ready, err := s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", UnlimitedLimits())
	require.NoError(t, err)
	require.True(t, ready)
	assert.Equal(t, original.ID, repaired.ID)
	assert.Equal(t, int32(1), f.deletes.Load())
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.NotContains(t, f.operations, "POST /api/v3.1/connected_accounts/ca_fixture/revoke")
}

func TestManagedCleanupDoesNotRevokeAnAdoptedReference(t *testing.T) {
	s, _ := testService(t)
	f := newComposioFixture(t, s, "https://api.example.test")
	ctx := context.Background()
	session, err := s.BeginVendorBound(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser")
	require.NoError(t, err)
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	c, ready, err := s.CompleteVendor(ctx, 1, "browser", session.ID, "Notes", UnlimitedLimits())
	require.NoError(t, err)
	require.True(t, ready)
	// Simulate a crash between committing the connection and removing its
	// durable pending record. The new process has no in-memory flow result.
	require.NoError(t, s.cfg.Store.CreatePendingVendorConnection(ctx, pending[0]))
	require.NoError(t, New(s.cfg).CleanupVendorConnections(ctx, 1))
	assert.Zero(t, f.deletes.Load())
	_, err = s.cfg.Store.GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
}

func TestIndeterminateManagedStartRemainsAvailableForReconciliation(t *testing.T) {
	s, _ := testService(t)
	newComposioFixture(t, s, "https://api.example.test")
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.HasSuffix(r.URL.Path, "/link"))
		w.Write([]byte(`{"missing":"connected_account_id"}`))
	}))
	defer upstream.Close()
	s.cfg.Vendor.ComposioBaseURL = upstream.URL
	ctx := context.Background()
	_, err := s.BeginVendorBound(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser")
	require.Error(t, err)
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	err = New(s.cfg).CleanupVendorConnections(ctx, 1)
	require.ErrorContains(t, err, "operator reconciliation")
	pending, err = s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	require.Len(t, pending, 1)
}

func TestRejectedManagedStartDoesNotLeaveCleanupWork(t *testing.T) {
	s, _ := testService(t)
	newComposioFixture(t, s, "https://api.example.test")
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()
	s.cfg.Vendor.ComposioBaseURL = upstream.URL
	ctx := context.Background()
	for range 3 {
		_, err := s.BeginVendorBound(ctx, 1, "notes-cloud", UnlimitedLimits(), "browser")
		require.Error(t, err)
		assert.Empty(t, s.vendorFlowLocks.held, "rejected attempts do not retain a mutex per setup request")
	}
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, pending)
	require.NoError(t, s.CleanupVendorConnections(ctx, 1))
}

func TestAbandonedNangoFlowRetainsOwnershipUntilRemoteSessionExpires(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	started := time.Now().Truncate(time.Second)
	expires := started.Add(2 * time.Hour)
	var flowID atomic.Value
	var exists atomic.Bool
	var deletes atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/connect/sessions":
			var body struct {
				Tags map[string]string `json:"tags"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			flowID.Store(body.Tags["exhibit_flow"])
			fmt.Fprintf(w, `{"data":{"token":"link-token","expires_at":%q}}`, expires.Format(time.RFC3339))
		case "/connections":
			if !exists.Load() {
				fmt.Fprint(w, `{"connections":[]}`)
				return
			}
			fmt.Fprintf(w, `{"connections":[{"connection_id":"late-account","provider_config_key":"notes","tags":{"exhibit_flow":%q,"exhibit_owner":"tenant:exhibit:1","end_user_id":"tenant:exhibit:1"}}]}`, flowID.Load().(string))
		case "/connections/late-account":
			if r.Method == http.MethodDelete {
				deletes.Add(1)
				exists.Store(false)
				fmt.Fprint(w, `{}`)
				return
			}
			fmt.Fprintf(w, `{"connection_id":"late-account","provider_config_key":"notes","tags":{"exhibit_flow":%q,"exhibit_owner":"tenant:exhibit:1"},"end_user":{"id":"tenant:exhibit:1"}}`, flowID.Load().(string))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer provider.Close()
	s.cfg.VendorEnabled = true
	s.cfg.Vendor = VendorConfig{BaseURL: provider.URL, SecretKey: "operator-key", UserNamespace: "tenant", Providers: map[string]VendorProvider{
		"notes": {Engine: "nango", Origins: []string{"https://api.example.test"}},
	}}
	_, err := s.BeginVendorBound(ctx, 1, "notes", UnlimitedLimits(), "browser")
	require.NoError(t, err)
	config := s.cfg
	clock := started.Add(31 * time.Minute)
	config.Now = func() time.Time { return clock }
	restarted := New(config)
	_, err = restarted.PruneIdle(ctx)
	require.ErrorContains(t, err, "still pending")
	exists.Store(true) // Consent finishes after the local browser flow expired.
	_, err = restarted.PruneIdle(ctx)
	require.ErrorContains(t, err, "still pending")
	assert.Equal(t, int32(1), deletes.Load())
	pending, err := s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	exists.Store(true) // Another late completion must still have an owner.
	clock = expires.Add(time.Minute)
	count, err := restarted.PruneIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, int32(2), deletes.Load())
	pending, err = s.cfg.Store.ListPendingVendorConnections(ctx, 1, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, pending)
}
