package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetainedDataPoliciesEnforceEgressAfterUngrantAndRevoke(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var exfiltrationCalls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"balance":"12345.67"}`) }))
	defer provider.Close()
	exfiltration := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exfiltrationCalls.Add(1)
		fmt.Fprint(w, "external destination")
	}))
	defer exfiltration.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Financial API", Recipe: "bearer", Origin: provider.URL, Secret: "private-financial-secret"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL, exfiltration.URL)
	response, err := doRequest(t, s, 1, gate, provider.URL+"/balance")
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.SetStateAsViewer(ctx, store.ViewerID(1), "app", "balance", string(response.Body)))
	require.NoError(t, s.Ungrant(ctx, 1, "app", c.ID))
	_, err = doRequest(t, s, 1, gate, exfiltration.URL+"/collect")
	assert.ErrorIs(t, err, ErrEgress)
	require.NoError(t, s.Revoke(ctx, 1, c.ID))
	allowed, err := s.AllowedEgress(ctx, 1, "app", []string{provider.URL, exfiltration.URL})
	require.NoError(t, err)
	assert.Equal(t, []string{provider.URL}, allowed)
	_, err = doRequest(t, s, 1, gate, exfiltration.URL+"/collect")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Zero(t, exfiltrationCalls.Load(), "revoking credentials cannot release saved financial data")
	assert.ErrorIs(t, s.ValidateArtifactSharing(ctx, 1, "app"), store.ErrConnectionSharing)
	require.NoError(t, s.ClearData(ctx, 1, "app"))
	state, err := s.cfg.Store.GetStateAsViewer(ctx, store.ViewerID(1), "app")
	require.NoError(t, err)
	assert.Empty(t, state)
	allowed, err = s.AllowedEgress(ctx, 1, "app", []string{provider.URL, exfiltration.URL})
	require.NoError(t, err)
	assert.Equal(t, []string{provider.URL, exfiltration.URL}, allowed)
	_, err = doRequest(t, s, 1, gate, exfiltration.URL+"/collect")
	require.NoError(t, err)
	assert.Equal(t, int32(1), exfiltrationCalls.Load())
}

func TestRetainedPolicyIntersectionAppliesToEveryRedirect(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var forbiddenCalls atomic.Int32
	forbidden := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forbiddenCalls.Add(1) }))
	defer forbidden.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, forbidden.URL+"/collect", http.StatusFound)
	}))
	defer redirector.Close()
	c, err := s.Create(ctx, 1, CreateRequest{Name: "Private API", Recipe: "bearer", Origin: redirector.URL, Secret: "secret"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, s.Revoke(ctx, 1, c.ID))
	_, err = doRequest(t, s, 1, testGate(t, redirector.URL, forbidden.URL), redirector.URL+"/no-credential")
	assert.ErrorIs(t, err, ErrEgress)
	assert.Zero(t, forbiddenCalls.Load())
}

func TestRetainedPoliciesIntersectAcrossRevokedConnections(t *testing.T) {
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
	allowed, err := s.AllowedEgress(ctx, 1, "app", []string{"https://a.example.test", "https://b.example.test", "https://common.example.test", "https://elsewhere.example.test"})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://common.example.test"}, allowed)
	policies, err := s.DataPolicies(ctx, 1, "app")
	require.NoError(t, err)
	assert.Len(t, policies, 2)
	// Data protection does not depend on the operational engines or seal key.
	policyOnly := New(Config{Store: s.cfg.Store})
	allowed, err = policyOnly.AllowedEgress(ctx, 1, "app", []string{"https://common.example.test", "https://elsewhere.example.test"})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://common.example.test"}, allowed)
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
	require.NoError(t, s.cfg.Store.CreateShare(ctx, 1, &store.Share{ID: "named-recipient", ArtifactID: "app", RecipientID: &viewerID}))
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
	// An active shared grant remains eligible after a recipient reset. Keep
	// their custody row too, so an in-flight response cannot be saved without
	// protection and later released by the owner's reset.
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
	assert.Equal(t, []string{provider.URL}, allowed)
	state, err := s.cfg.Store.GetStateAsViewer(ctx, store.ViewerID(viewerID), "app")
	require.NoError(t, err)
	assert.Contains(t, state["balance"], "12345.67")
	require.NoError(t, s.ClearData(ctx, viewerID, "app"))
	policies, err = s.DataPolicies(ctx, viewerID, "app")
	require.NoError(t, err)
	assert.Empty(t, policies)
}
