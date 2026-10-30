package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func relayFixture(t *testing.T, handler http.Handler) (*Router, string, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	ro := newTestRouter(t)
	ro.cfg.Relay = RelayConfig{Enabled: true, SchedulesEnabled: true, GateFactory: egress.NewForTests}
	id := createTestArtifact(t, ro, "Connected example")
	require.NoError(t, ro.cfg.Store.SetOriginDecision(context.Background(), 1, id, upstream.URL, store.DecisionAllow, "test"))
	return ro, id, upstream
}

func relayCall(t *testing.T, ro *Router, id string, input relayRequest, policy string) *httptest.ResponseRecorder {
	t.Helper()
	if policy == "" {
		a, err := ro.cfg.Store.GetArtifact(context.Background(), 1, id)
		require.NoError(t, err)
		policy, err = ro.relayPolicy(context.Background(), 1, a)
		require.NoError(t, err)
	}
	body, err := json.Marshal(input)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/api/artifacts/"+id+"/relay", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Exhibit-Relay-Policy", policy)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

func TestRelayRequiresSeparateReadAndWriteConsentAndSanitizesHeaders(t *testing.T) {
	var calls atomic.Int64
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		for _, h := range []string{"Cookie", "Authorization", "Origin", "Referer", "X-Forwarded-For"} {
			assert.Empty(t, r.Header.Get(h), h)
		}
		assert.Equal(t, "Exhibit-Relay/1 (+http://app.test)", r.UserAgent())
		w.Header().Set("Set-Cookie", "session=secret")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<script>window.appOriginCompromised=true</script>`)
	}))
	input := relayRequest{URL: upstream.URL + "/data?private=query", Method: "GET", Headers: map[string]string{"Cookie": "session=forged", "Authorization": "Bearer forged", "Origin": "https://attacker.test", "Referer": "https://attacker.test", "X-Forwarded-For": "127.0.0.1"}}
	w := relayCall(t, ro, id, input, "")
	require.Equal(t, 403, w.Code)
	assert.Contains(t, w.Body.String(), "relay_consent_required")
	assert.Zero(t, calls.Load())
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(context.Background(), 1, id, store.RelayGrant{Allowed: true}))
	w = relayCall(t, ro, id, input, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var result relayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 200, result.Status)
	assert.Empty(t, result.Headers["Set-Cookie"])
	assert.NotContains(t, w.Body.String(), "<script>")
	body, err := base64.StdEncoding.DecodeString(result.BodyBase64)
	require.NoError(t, err)
	assert.Contains(t, string(body), "appOriginCompromised")
	// One consent covers every method: a POST may only read, and a GET may
	// change things, so the relay does not pretend to tell them apart.
	input.Method = "POST"
	input.BodyBase64 = base64.StdEncoding.EncodeToString([]byte("data"))
	w = relayCall(t, ro, id, input, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.EqualValues(t, 2, calls.Load())
	activity, err := s.ListRelayActivity(context.Background(), 1, id)
	require.NoError(t, err)
	require.Len(t, activity, 2)
	raw, _ := json.Marshal(activity)
	assert.NotContains(t, string(raw), "private")
	assert.NotContains(t, string(raw), "query")
}

func TestRelayStalePolicyAndUpstreamBackoff(t *testing.T) {
	var calls atomic.Int64
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	}))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(context.Background(), 1, id, store.RelayGrant{Allowed: true}))
	a, err := ro.cfg.Store.GetArtifact(context.Background(), 1, id)
	require.NoError(t, err)
	old, err := ro.relayPolicy(context.Background(), 1, a)
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetOriginDecision(context.Background(), 1, id, "https://another.example.test", store.DecisionAllow, "test"))
	input := relayRequest{URL: upstream.URL, Method: "GET"}
	w := relayCall(t, ro, id, input, old)
	assert.Equal(t, 409, w.Code)
	assert.Zero(t, calls.Load())
	w = relayCall(t, ro, id, input, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"status":429`)
	w = relayCall(t, ro, id, input, "")
	assert.Equal(t, 429, w.Code)
	assert.EqualValues(t, 1, calls.Load())
}

func TestRelayCacheRespectsRequestIdentityAndCurrentPolicy(t *testing.T) {
	var calls atomic.Int64
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/private" {
			w.Header().Set("Cache-Control", "private, max-age=120")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=120")
		}
		fmt.Fprint(w, r.Header.Get("Accept-Language"))
	}))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(context.Background(), 1, id, store.RelayGrant{Allowed: true}))
	input := relayRequest{URL: upstream.URL + "/public", Headers: map[string]string{"Accept-Language": "en"}}
	for range 2 {
		require.Equal(t, 200, relayCall(t, ro, id, input, "").Code)
	}
	assert.EqualValues(t, 1, calls.Load(), "a fresh public response is reused")
	input.Headers["Accept-Language"] = "fr"
	require.Equal(t, 200, relayCall(t, ro, id, input, "").Code)
	assert.EqualValues(t, 2, calls.Load(), "negotiated representations have separate keys")
	input.Cache = "reload"
	require.Equal(t, 200, relayCall(t, ro, id, input, "").Code)
	assert.EqualValues(t, 3, calls.Load(), "reload bypasses the cache")
	input.Cache = ""
	require.NoError(t, ro.cfg.Store.SetOriginDecision(context.Background(), 1, id, "https://another.example.test", store.DecisionAllow, "test"))
	require.Equal(t, 200, relayCall(t, ro, id, input, "").Code)
	assert.EqualValues(t, 4, calls.Load(), "a different render policy cannot reuse cached bytes")
	input.URL = upstream.URL + "/private"
	for range 2 {
		require.Equal(t, 200, relayCall(t, ro, id, input, "").Code)
	}
	assert.EqualValues(t, 6, calls.Load(), "private responses are never retained")
}

func TestRelayBackoffIsEnforcedBeforeFollowingARedirect(t *testing.T) {
	var targetCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(target.Close)
	ro, id, redirect := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/limited", http.StatusFound)
	}))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(context.Background(), 1, id, store.RelayGrant{Allowed: true}))
	require.NoError(t, ro.cfg.Store.SetOriginDecision(context.Background(), 1, id, target.URL, store.DecisionAllow, "test"))
	require.Equal(t, 200, relayCall(t, ro, id, relayRequest{URL: target.URL}, "").Code)
	w := relayCall(t, ro, id, relayRequest{URL: redirect.URL}, "")
	assert.Equal(t, 429, w.Code, w.Body.String())
	assert.EqualValues(t, 1, targetCalls.Load(), "a redirect must not bypass target backoff")
}

func TestRelayScheduledRefreshUsesCurrentConsentAndKeepsPreviousDataOnFailure(t *testing.T) {
	var calls atomic.Int64
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, `{"balance":42}`) }))
	s, _ := ro.relayStore()
	ctx := context.Background()
	require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Allowed: true}))
	now := time.Now()
	job := store.RelaySchedule{ID: "refresh", ArtifactID: id, ViewerID: 1, URL: upstream.URL, StateKey: "accounts", IntervalSeconds: 3600, NextRunAt: now.Unix(), Enabled: true}
	require.NoError(t, s.PutRelaySchedule(ctx, job, 10))
	ro.runRelaySchedules(ctx, now)
	state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, `{"balance":42}`, state["accounts"])
	require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{}))
	ro.runRelaySchedules(ctx, now.Add(2*time.Hour))
	assert.EqualValues(t, 1, calls.Load())
	state, err = ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, `{"balance":42}`, state["accounts"])
	jobs, err := s.ListRelaySchedules(ctx, 1, id)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.NotEmpty(t, jobs[0].LastError)
}

func TestRelayPagesKeepSecretsOutAndDefaultDisabled(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Example")
	for _, path := range []string{"/connections", "/artifacts/" + id} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "data-relay-src")
	}
	ro.cfg.Relay.Enabled = true
	req := httptest.NewRequest("GET", "/artifacts/"+id, nil)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "data-relay-src")
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "/a/"+id)
	// A frame's forged window-message request cannot choose a policy. Only
	// the app controller's page value is sent in this header.
	req = httptest.NewRequest("POST", "/api/artifacts/"+id+"/relay", strings.NewReader(`{"url":"https://api.example.test"}`))
	req.Header.Set("Authorization", authHeader())
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, 409, w.Code)
}
