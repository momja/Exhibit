package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetSafetyFixture(t *testing.T) (*Router, *connectors.Service, string, string) {
	t.Helper()
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	c, err := service.Create(t.Context(), 1, connectors.CreateRequest{Name: "Account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(t.Context(), 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetStateAsViewer(t.Context(), 1, id, "balance", "PRIVATE-OLD-CACHE"))
	require.NoError(t, service.Ungrant(t.Context(), 1, id, c.ID))
	a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
	require.NoError(t, err)
	policy, err := ro.relayPolicy(t.Context(), 1, a)
	require.NoError(t, err)
	return ro, service, id, policy
}

func stateFrameRequest(ro *Router, id, method, suffix, body, policy string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/artifacts/"+id+"/state"+suffix, strings.NewReader(body))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Exhibit-Relay-Policy", policy)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

func TestConnectionDataResetRefusesOldFrameStateMutations(t *testing.T) {
	ro, _, id, policy := resetSafetyFixture(t)
	w := doJSON(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-data", nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	for _, test := range []struct{ method, suffix, body string }{
		{http.MethodPut, "", `{"key":"balance","value":"PRIVATE-OLD-CACHE"}`},
		{http.MethodDelete, "?key=balance", ""},
		{http.MethodPost, "/import?replace=true", `{"balance":"PRIVATE-OLD-CACHE"}`},
	} {
		w = stateFrameRequest(ro, id, test.method, test.suffix, test.body, policy)
		assert.Equal(t, http.StatusConflict, w.Code, test.method+": "+w.Body.String())
	}
	state, err := ro.cfg.Store.GetStateAsViewer(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, state, "a released data policy must not leave an old frame able to restore its private cache")
}

type blockedFrameStateStore struct {
	store.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockedFrameStateStore) SetStateAsViewer(ctx context.Context, viewer store.ViewerID, artifact, key, value string, version ...int) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.Store.SetStateAsViewer(ctx, viewer, artifact, key, value, version...)
}

func TestConnectionDataResetCannotOvertakeAnAlreadyValidatedFrameWrite(t *testing.T) {
	ro, _, id, policy := resetSafetyFixture(t)
	blocked := &blockedFrameStateStore{Store: ro.cfg.Store, entered: make(chan struct{}), release: make(chan struct{})}
	ro.cfg.Store = blocked
	writeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		writeDone <- stateFrameRequest(ro, id, http.MethodPut, "", `{"key":"balance","value":"PRIVATE-OLD-CACHE"}`, policy)
	}()
	select {
	case <-blocked.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("frame write did not reach the mutation")
	}
	resetDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { resetDone <- doJSON(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-data", nil) }()
	select {
	case <-resetDone:
		t.Fatal("data reset overtook a validated frame mutation")
	case <-time.After(30 * time.Millisecond):
	}
	close(blocked.release)
	require.Equal(t, http.StatusNoContent, (<-writeDone).Code)
	require.Equal(t, http.StatusNoContent, (<-resetDone).Code)
	state, err := blocked.GetStateAsViewer(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, state)
}

func TestRetainedProvenanceDoesNotRestrictApprovedRelayWithAcquisitionDisabled(t *testing.T) {
	var hits int
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		assert.Empty(t, r.Header.Get("Authorization"))
		fmt.Fprint(w, "standalone response")
	}))
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	c, err := service.Create(t.Context(), 1, connectors.CreateRequest{Name: "Private account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(t.Context(), 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, service.Ungrant(t.Context(), 1, id, c.ID))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(t.Context(), 1, id, store.RelayGrant{Allowed: true}))
	for _, enabled := range []bool{false, true} {
		if enabled {
			ro.cfg.Connections = service
		}
		w := relayCall(t, ro, id, relayRequest{URL: upstream.URL + "/public", Method: http.MethodGet}, "")
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	assert.Equal(t, 2, hits, "retained metadata must not block independently approved traffic")
}

func TestConnectionDataResetDiscardsAnInFlightScheduledResponse(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		fmt.Fprint(w, `{"balance":"PRIVATE-SCHEDULED-RESPONSE"}`)
	}))
	defer upstream.Close()
	ro := newTestRouter(t)
	ro.cfg.Relay = RelayConfig{Enabled: true, SchedulesEnabled: true, GateFactory: egress.NewForTests}
	id := createOwnArtifact(t, ro)
	require.NoError(t, ro.cfg.Store.SetOriginDecision(t.Context(), 1, id, upstream.URL, store.DecisionAllow, "test"))
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	ro.cfg.Connections = service
	c, err := service.Create(t.Context(), 1, connectors.CreateRequest{Name: "Account", Recipe: "bearer", Origin: upstream.URL, Secret: "secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(t.Context(), 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, service.Ungrant(t.Context(), 1, id, c.ID))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(t.Context(), 1, id, store.RelayGrant{Allowed: true}))
	done := make(chan string, 1)
	go func() {
		_, reason := ro.refreshSchedule(t.Context(), store.RelaySchedule{ArtifactID: id, ViewerID: 1, URL: upstream.URL, StateKey: "balance"})
		done <- reason
	}()
	select {
	case <-started:
	case reason := <-done:
		t.Fatalf("scheduled request stopped before reaching the provider: %s", reason)
	case <-time.After(3 * time.Second):
		t.Fatal("scheduled request did not reach the provider")
	}
	w := doJSON(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-data", nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	finish()
	select {
	case reason := <-done:
		assert.Contains(t, reason, "changed")
	case <-time.After(3 * time.Second):
		t.Fatal("scheduled response did not finish")
	}
	state, err := ro.cfg.Store.GetStateAsViewer(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, state, "a response captured before reset cannot restore private data after its policy is released")
}
