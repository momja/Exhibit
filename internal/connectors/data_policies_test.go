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

func TestSharedToolRecipientsUseOwnAccountsAndCache(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	var ownerCalls, recipientCalls, anonymousCalls atomic.Int32
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer owner-financial-secret":
			ownerCalls.Add(1)
			fmt.Fprint(w, `{"balance":"owner result"}`)
		case "Bearer recipient-financial-secret":
			recipientCalls.Add(1)
			fmt.Fprint(w, `{"balance":"recipient result"}`)
		default:
			anonymousCalls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer provider.Close()
	_, err := s.cfg.Store.UpsertUser(ctx, "owner", "owner@example.test")
	require.NoError(t, err)
	viewer, err := s.cfg.Store.UpsertUser(ctx, "recipient", "recipient@example.test")
	require.NoError(t, err)
	_, err = s.cfg.Store.CreateGrant(ctx, 1, "app", viewer.ID)
	require.NoError(t, err)
	owner, err := s.Create(ctx, 1, CreateRequest{Name: "Owner account", Recipe: "bearer", Origin: provider.URL, Secret: "owner-financial-secret", CacheMinutes: 60}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: owner.ID})
	require.NoError(t, err)
	gate := testGate(t, provider.URL)
	response, err := doRequest(t, s, 1, gate, provider.URL+"/balance")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "owner result")
	grants, err := s.Grants(ctx, viewer.ID, "app")
	require.NoError(t, err)
	assert.Empty(t, grants)
	_, err = s.Standin(ctx, viewer.ID, "app", owner.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
	response, err = doRequest(t, s, viewer.ID, gate, provider.URL+"/balance")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, response.Status)
	assert.NotContains(t, string(response.Body), "owner result", "recipients cannot use the owner's cached response")
	personal, err := s.Create(ctx, viewer.ID, CreateRequest{Name: "Recipient account", Recipe: "bearer", Origin: provider.URL, Secret: "recipient-financial-secret", CacheMinutes: 60}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, viewer.ID, "app", GrantRequest{ConnectionID: personal.ID})
	require.NoError(t, err)
	for range 2 {
		response, err = doRequest(t, s, viewer.ID, gate, provider.URL+"/balance")
		require.NoError(t, err)
		assert.Contains(t, string(response.Body), "recipient result")
	}
	assert.Equal(t, int32(1), ownerCalls.Load())
	assert.Equal(t, int32(1), recipientCalls.Load())
	assert.Equal(t, int32(1), anonymousCalls.Load())
	// Saved results and personal account use remain separate in shared state.
	require.NoError(t, s.cfg.Store.UpdateArtifact(ctx, 1, "app", map[string]any{"share_state_mode": store.ShareStateShared}))
	response, err = doRequest(t, s, viewer.ID, gate, provider.URL+"/balance")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "recipient result")
	require.NoError(t, s.cfg.Store.SetStateAsViewer(ctx, store.ViewerID(viewer.ID), "app", "balance", string(response.Body)))
	state, err := s.cfg.Store.GetStateAsViewer(ctx, store.ViewerID(1), "app")
	require.NoError(t, err)
	assert.Contains(t, state["balance"], "recipient result", "shared results follow the tool's saved-data setting")
	require.NoError(t, s.Revoke(ctx, 1, owner.ID))
	response, err = doRequest(t, s, viewer.ID, gate, provider.URL+"/balance")
	require.NoError(t, err)
	assert.Contains(t, string(response.Body), "recipient result", "disconnecting the owner's account cannot revoke the recipient's")
}
