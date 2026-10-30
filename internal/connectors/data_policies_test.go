package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetainedDataProvenanceDoesNotRestrictApprovedNetworkAccess(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var standaloneCalls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"balance":"12345.67"}`) }))
	defer provider.Close()
	standalone := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		standaloneCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		fmt.Fprint(w, "independently approved API")
	}))
	defer standalone.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Financial API", Recipe: "bearer", Origin: provider.URL, Secret: "private-financial-secret"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	base := []string{provider.URL, standalone.URL}
	gate := testGate(t, base...)
	response, err := doRequest(t, s, 1, gate, provider.URL+"/balance")
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.SetStateAsViewer(ctx, store.ViewerID(1), "app", "balance", string(response.Body)))
	require.NoError(t, s.Ungrant(ctx, 1, "app", c.ID))
	_, err = doRequest(t, s, 1, gate, standalone.URL+"/public")
	require.NoError(t, err)
	require.NoError(t, s.Revoke(ctx, 1, c.ID))
	allowed, err := s.AllowedEgress(ctx, 1, "app", base)
	require.NoError(t, err)
	assert.Equal(t, base, allowed)
	_, err = doRequest(t, s, 1, gate, standalone.URL+"/public")
	require.NoError(t, err)
	assert.Equal(t, int32(2), standaloneCalls.Load())
	assert.ErrorIs(t, s.ValidateArtifactSharing(ctx, 1, "app"), store.ErrConnectionSharing)
	require.NoError(t, s.ClearData(ctx, 1, "app"))
	state, err := s.cfg.Store.GetStateAsViewer(ctx, store.ViewerID(1), "app")
	require.NoError(t, err)
	assert.Empty(t, state)
	allowed, err = s.AllowedEgress(ctx, 1, "app", base)
	require.NoError(t, err)
	assert.Equal(t, base, allowed)
	_, err = doRequest(t, s, 1, gate, standalone.URL+"/public")
	require.NoError(t, err)
	assert.Equal(t, int32(3), standaloneCalls.Load())
}

func TestRevokedProvenanceDoesNotRestrictApprovedRedirects(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var destinationCalls atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
	}))
	defer destination.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		http.Redirect(w, r, destination.URL+"/public", http.StatusFound)
	}))
	defer redirector.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Private API", Recipe: "bearer", Origin: redirector.URL, Secret: "secret"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, s.Revoke(ctx, 1, c.ID))
	_, err = doRequest(t, s, 1, testGate(t, redirector.URL, destination.URL), redirector.URL+"/no-credential")
	require.NoError(t, err)
	assert.Equal(t, int32(1), destinationCalls.Load())
	_, err = doRequest(t, s, 1, testGate(t, redirector.URL), redirector.URL+"/no-credential")
	var denied *egress.DeniedError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, destination.URL, denied.Origin)
	assert.Equal(t, int32(1), destinationCalls.Load(), "redirects still need the artifact's independent approval")
}

func TestRetainedProvenanceCannotChangeIndependentOriginApprovals(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	for _, input := range []struct{ origin, permitted string }{
		{"https://a.example.test", "https://common.example.test"},
		{"https://b.example.test", "https://common.example.test"},
	} {
		c, err := s.Create(ctx, 1, CreateRequest{Name: "Personal API", Recipe: "bearer", Origin: input.origin, Secret: "secret"}, UnlimitedLimits())
		require.NoError(t, err)
		_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID, PermittedEgress: []string{input.permitted}})
		require.NoError(t, err)
		require.NoError(t, s.Revoke(ctx, 1, c.ID))
	}
	base := []string{"https://a.example.test", "https://b.example.test", "https://standalone.example.test", "https://cdn.example.test"}
	allowed, err := s.AllowedEgress(ctx, 1, "app", base)
	require.NoError(t, err)
	assert.Equal(t, base, allowed)
	assert.NotContains(t, allowed, "https://common.example.test", "legacy permitted_egress metadata must not approve an origin")
	_, err = doRequest(t, s, 1, testGate(t, base...), "https://common.example.test/unapproved")
	var denied *egress.DeniedError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, "https://common.example.test", denied.Origin)
	policies, err := s.DataPolicies(ctx, 1, "app")
	require.NoError(t, err)
	assert.Len(t, policies, 2)
	// Independent approvals require no connector engines or provenance store.
	policyOnly := New(Config{})
	allowed, err = policyOnly.AllowedEgress(ctx, 1, "app", base)
	require.NoError(t, err)
	assert.Equal(t, base, allowed)
	allowed[0] = "https://changed.example.test"
	assert.Equal(t, "https://a.example.test", base[0], "callers must not be able to mutate the approved-origin slice through the result")
	allowed, err = policyOnly.AllowedEgress(ctx, 1, "app", nil)
	require.NoError(t, err)
	assert.Empty(t, allowed, "no approvals remain no approvals, regardless of connection origins")
}

func TestSharedConnectionRecipientRetainsPolicyAfterOwnerReset(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer shared-financial-secret", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"balance":"12345.67"}`)
	}))
	defer provider.Close()
	viewerID := int64(2)
	// In single-user test stores recipient shares still require a real users
	// row. Give the recipient an account without changing the owner principal.
	_, err := s.cfg.Store.UpsertUser(ctx, "owner", "owner@example.test")
	require.NoError(t, err)
	viewer, err := s.cfg.Store.UpsertUser(ctx, "recipient", "recipient@example.test")
	require.NoError(t, err)
	require.Equal(t, viewerID, viewer.ID)
	_, err = s.cfg.Store.CreateGrant(ctx, 1, "app", viewerID)
	require.NoError(t, err)
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Shared financial API", Recipe: "bearer", Origin: provider.URL, Secret: "shared-financial-secret"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID, ShareWithRecipients: true})
	require.NoError(t, err)
	response, err := doRequest(t, s, viewerID, testGate(t, provider.URL), provider.URL+"/balance")
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.SetStateAsViewer(ctx, store.ViewerID(viewerID), "app", "balance", string(response.Body)))
	policies, err := s.DataPolicies(ctx, viewerID, "app")
	require.NoError(t, err)
	require.Len(t, policies, 2, "recipient records custody before the response can be saved")
	// Active shared grants keep their saved-data provenance during a reset.
	require.NoError(t, s.ClearData(ctx, viewerID, "app"))
	policies, err = s.DataPolicies(ctx, viewerID, "app")
	require.NoError(t, err)
	require.Len(t, policies, 2)
	require.NoError(t, s.cfg.Store.SetStateAsViewer(ctx, store.ViewerID(viewerID), "app", "balance", string(response.Body)))
	require.NoError(t, s.Revoke(ctx, 1, c.ID))
	require.NoError(t, s.ClearData(ctx, 1, "app"))
	policies, err = s.DataPolicies(ctx, viewerID, "app")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, viewerID, policies[0].ViewerID)
	allowed, err := s.AllowedEgress(ctx, viewerID, "app", []string{provider.URL, "https://elsewhere.example.test"})
	require.NoError(t, err)
	assert.Equal(t, []string{provider.URL, "https://elsewhere.example.test"}, allowed)
	state, err := s.cfg.Store.GetStateAsViewer(ctx, store.ViewerID(viewerID), "app")
	require.NoError(t, err)
	assert.Contains(t, state["balance"], "12345.67")
	require.NoError(t, s.ClearData(ctx, viewerID, "app"))
	policies, err = s.DataPolicies(ctx, viewerID, "app")
	require.NoError(t, err)
	assert.Empty(t, policies)
}
