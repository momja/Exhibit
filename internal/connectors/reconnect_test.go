package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconnectNativePreservesAccessAndInvalidatesCachedCredentials(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Personal", Recipe: "bearer", Origin: "https://api.example.test", Secret: "old-secret", DailyLimit: 7, CacheMinutes: 60}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.ReserveConnectionRequest(ctx, c.ID, 7, time.Now()))
	require.NoError(t, s.cfg.Store.PutConnectionCache(ctx, c.ID, "cached", "old-cache", time.Now().Add(time.Hour)))
	handle, err := s.Standin(ctx, 1, "app", c.ID)
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.PutRelaySchedule(ctx, store.RelaySchedule{ID: "refresh", ArtifactID: "app", ViewerID: 1, URL: "https://api.example.test/data", StateKey: "result", IntervalSeconds: 86400, NextRunAt: time.Now().Unix(), Enabled: true}, 10))
	before, err := s.cfg.Store.GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
	_, err = s.Reconnect(ctx, 2, c.ID, ReconnectRequest{Secret: "other-user"})
	assert.ErrorIs(t, err, store.ErrNotFound)
	updated, err := s.Reconnect(ctx, 1, c.ID, ReconnectRequest{Secret: "new-secret"})
	require.NoError(t, err)
	assert.Equal(t, c.ID, updated.ID)
	assert.Equal(t, c.CreatedAt, updated.CreatedAt)
	assert.Equal(t, c.Name, updated.Name)
	assert.Equal(t, 7, updated.DailyLimit)
	assert.Equal(t, 1, updated.RequestsToday)
	after, err := s.cfg.Store.GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, before.RecipeJSON, after.RecipeJSON)
	_, secret, err := s.readCredential(after)
	require.NoError(t, err)
	assert.Equal(t, "new-secret", secret.Value)
	grants, err := s.Grants(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.Equal(t, c.ID, grants[0].Connection.ID)
	cache, err := s.cfg.Store.GetConnectionCache(ctx, c.ID, "cached", time.Now())
	require.ErrorIs(t, err, store.ErrNotFound)
	assert.Empty(t, cache)
	s.mu.Lock()
	_, alive := s.standins[handle.Value]
	s.mu.Unlock()
	assert.False(t, alive)
	schedules, err := s.cfg.Store.ListRelaySchedules(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, schedules, 1)
	// A stale completion cannot overwrite the repair or bring a deleted account back.
	err = s.cfg.Store.RepairConnection(ctx, before, before.EncryptedSecret, "")
	assert.ErrorIs(t, err, store.ErrNotFound)
	s.cfg.CustomEnabled = false
	_, err = s.Reconnect(ctx, 1, c.ID, ReconnectRequest{Secret: "disabled"})
	assert.ErrorIs(t, err, ErrDisabled)
}

func TestReconnectBasicKeepsPrivateServicePath(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Basic", Recipe: "basic", AccessURL: "https://user:old@api.example.test/private/base"}, UnlimitedLimits())
	require.NoError(t, err)
	for _, raw := range []string{"https://user:new@other.example.test/private/base", "https://user:new@api.example.test/different"} {
		_, err = s.Reconnect(ctx, 1, c.ID, ReconnectRequest{AccessURL: raw})
		assert.ErrorIs(t, err, ErrInvalid)
	}
	updated, err := s.Reconnect(ctx, 1, c.ID, ReconnectRequest{AccessURL: "https://user:new@api.example.test/private/base"})
	require.NoError(t, err)
	assert.Equal(t, c.ID, updated.ID)
}

func TestOAuthSetupRepairsExistingIdentityAndBindsToolContext(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"new-token","token_type":"bearer","expires_in":3600}`)
	}))
	defer provider.Close()
	s.cfg.OAuthEnabled = true
	s.cfg.GitHub = OAuthProvider{ClientID: "client", ClientSecret: "secret", AuthURL: provider.URL + "/auth", TokenURL: provider.URL + "/token", APIOrigin: provider.URL}
	original, err := s.save(ctx, 1, "GitHub", "github", "oauth", recipe{Origins: []string{provider.URL}, Header: "Authorization", Scheme: "Bearer "}, credential{OAuthToken: json.RawMessage(`{"access_token":"old-token","token_type":"bearer"}`)}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: original.ID})
	require.NoError(t, err)
	setup := SetupContext{ArtifactID: "app", ConnectionID: original.ID}
	begin := func() string {
		destination, err := s.BeginOAuthFor(ctx, 1, "browser", "ignored rename", "https://app.example.test/callback", setup)
		require.NoError(t, err)
		u, err := url.Parse(destination)
		require.NoError(t, err)
		return u.Query().Get("state")
	}
	_, bound, err := s.CompleteOAuthFor(ctx, 2, "browser", begin(), "code", UnlimitedLimits())
	require.Error(t, err)
	assert.Nil(t, bound)
	_, bound, err = s.CompleteOAuthFor(ctx, 1, "browser", begin(), "", UnlimitedLimits())
	require.Error(t, err)
	require.NotNil(t, bound)
	assert.Equal(t, setup, *bound)
	repaired, bound, err := s.CompleteOAuthFor(ctx, 1, "browser", begin(), "code", Limits{})
	require.NoError(t, err)
	assert.Equal(t, setup, *bound)
	assert.Equal(t, original.ID, repaired.ID)
	assert.Equal(t, original.Name, repaired.Name)
	grants, err := s.Grants(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	// A tool removed after consent began cannot complete setup or return its context.
	state := begin()
	_, err = s.cfg.Store.DeleteArtifact(ctx, 1, "app")
	require.NoError(t, err)
	_, bound, err = s.CompleteOAuthFor(ctx, 1, "browser", state, "code", UnlimitedLimits())
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Nil(t, bound)
}

func TestVendorReconnectPreservesIdentityAndPinnedProviderPolicy(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	fixture := newComposioFixture(t, s, "https://api.example.test")
	spec := s.cfg.Vendor.Providers["notes-cloud"].defaults("notes-cloud")
	original, err := s.saveVendor(ctx, 1, "Notes", "notes-cloud", "tenant:exhibit:1", "ca_old", spec, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: original.ID})
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.MarkVendorConnectionReconnect(ctx, 1, original.ID))
	// Changed catalog settings do not silently redirect an existing grant.
	s.cfg.Vendor.Providers["notes-cloud"] = VendorProvider{Name: "Changed", Engine: "nango", Origins: []string{"https://different.example.test"}}
	setup := SetupContext{ArtifactID: "app", ConnectionID: original.ID}
	session, err := s.BeginVendorFor(ctx, 1, "notes-cloud", Limits{}, "browser", setup)
	require.NoError(t, err)
	assert.Equal(t, "composio", session.Engine)
	_, _, _, err = s.CompleteVendorFor(ctx, 1, "different-browser", session.ID, "rename", Limits{})
	require.Error(t, err)
	repaired, ready, bound, err := s.CompleteVendorFor(ctx, 1, "browser", session.ID, "rename", Limits{})
	require.NoError(t, err)
	require.True(t, ready)
	assert.Equal(t, setup, *bound)
	assert.Equal(t, original.ID, repaired.ID)
	assert.Equal(t, original.Name, repaired.Name)
	assert.Equal(t, original.Origins, repaired.Origins)
	assert.Equal(t, "active", repaired.Status)
	assert.Equal(t, int32(1), fixture.revokes.Load())
	assert.Equal(t, int32(1), fixture.deletes.Load())
	grants, err := s.Grants(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	again, ready, _, err := s.CompleteVendorFor(ctx, 1, "browser", session.ID, "ignored", Limits{})
	require.NoError(t, err)
	require.True(t, ready)
	assert.Equal(t, repaired.ID, again.ID)
	assert.Equal(t, int32(1), fixture.deletes.Load())
}
