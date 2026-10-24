package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayStandaloneCacheSurvivesUnrelatedConnectionChanges(t *testing.T) {
	var calls atomic.Int64
	ro, id, public := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Cache-Control", "public, max-age=120")
		fmt.Fprint(w, "public response")
	}))
	ctx := t.Context()
	require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, "https://other.example.test", store.DecisionAllow, "test"))
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Allowed: true}))
	request := relayRequest{URL: public.URL}
	for range 2 {
		w := relayCall(t, ro, id, request, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	c, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{
		Name: "Unrelated API", Recipe: "bearer", Origin: "https://other.example.test", Secret: "fixture-only-secret",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	w := relayCall(t, ro, id, request, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, ro.cfg.Connections.Revoke(ctx, 1, c.ID))
	w = relayCall(t, ro, id, request, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, calls.Load(), "unrelated grants must neither disable nor invalidate public response reuse")
}

func TestConnectionsAddCredentialsWithoutNarrowingApprovedTraffic(t *testing.T) {
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer fixture-first", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-API-Key"))
		fmt.Fprint(w, `{"source":"first"}`)
	}))
	defer first.Close()
	second := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "fixture-second", r.Header.Get("X-API-Key"))
		assert.Empty(t, r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"source":"second"}`)
	}))
	defer second.Close()
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-API-Key"))
		fmt.Fprint(w, `{"source":"public"}`)
	}))
	defer public.Close()
	unapproved := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("an unapproved origin must never be contacted")
	}))
	defer unapproved.Close()

	ro := newTestRouter(t)
	ro.cfg.Relay = RelayConfig{Enabled: true, SchedulesEnabled: true, GateFactory: egress.NewForTests}
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	id := createOwnArtifact(t, ro)
	ctx := t.Context()
	base := []string{first.URL, second.URL, public.URL}
	for _, target := range base {
		require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, target, store.DecisionAllow, "test"))
	}
	c1, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{
		Name: "First", Recipe: "bearer", Origin: first.URL, Secret: "fixture-first",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	c2, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{
		Name: "Second", Recipe: "api-key", Header: "X-API-Key", Origin: second.URL, Secret: "fixture-second",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	for _, c := range []*connectors.Connection{c1, c2} {
		// Old clients may still send permitted_egress. It supplies no network
		// authority and cannot narrow other origins or expand credential scope.
		w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/connections", map[string]any{
			"connection_id": c.ID, "permitted_egress": []string{unapproved.URL},
		})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}
	allowed, err := ro.connectionOrigins(ctx, 1, id, base)
	require.NoError(t, err)
	assert.ElementsMatch(t, base, allowed)
	a := mustArtifact(t, ro, id)
	before, err := ro.relayPolicy(ctx, 1, a)
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c1.ID})
	require.NoError(t, err)
	after, err := ro.relayPolicy(ctx, 1, a)
	require.NoError(t, err)
	assert.Equal(t, before, after, "changing only legacy destination metadata cannot invalidate the host")
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Allowed: true}))
	for _, target := range base {
		w := relayCall(t, ro, id, relayRequest{URL: target, Headers: map[string]string{
			"Authorization": "Bearer forged", "X-API-Key": "forged",
		}}, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response relayResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, http.StatusOK, response.Status)
		body, err := base64.StdEncoding.DecodeString(response.BodyBase64)
		require.NoError(t, err)
		assert.Contains(t, string(body), "source")
		assert.NotContains(t, w.Body.String(), "fixture-first")
		assert.NotContains(t, w.Body.String(), "fixture-second")
	}
	w := relayCall(t, ro, id, relayRequest{URL: unapproved.URL}, "")
	assert.Equal(t, http.StatusForbidden, w.Code, "legacy egress metadata cannot approve a new origin")

	// A new standalone approval works without editing either connection.
	w = doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]string{
		"origin": "https://cdn.example.test", "decision": "allow", "source": "runtime",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	now := time.Now()
	require.NoError(t, s.PutRelaySchedule(ctx, store.RelaySchedule{
		ID: "second-refresh", ArtifactID: id, ViewerID: 1, URL: second.URL, StateKey: "second-data",
		IntervalSeconds: 3600, NextRunAt: now.Unix(), Enabled: true,
	}, 10))
	ro.runRelaySchedules(ctx, now)
	state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, `{"source":"second"}`, state["second-data"])

	// Old retained records require no migration or saved-data deletion to
	// allow the other connector and the standalone endpoint to keep working.
	require.NoError(t, ro.cfg.Connections.Revoke(ctx, 1, c1.ID))
	for _, target := range []string{second.URL, public.URL} {
		w = relayCall(t, ro, id, relayRequest{URL: target}, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	ro.cfg.Connections = nil
	w = relayCall(t, ro, id, relayRequest{URL: public.URL}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = relayCall(t, ro, id, relayRequest{URL: second.URL}, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "connections_disabled", "only a matching connection needs the disabled engine")
	state, err = ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, `{"source":"second"}`, state["second-data"])
}
