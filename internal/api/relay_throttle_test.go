package api

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayThrottleReturnsRetryAfterBeforeProviderContact(t *testing.T) {
	var calls atomic.Int64
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(context.Background(), 1, id, store.RelayGrant{Allowed: true}))
	ro.relayRuntime.buckets["artifact:1:"+id] = relayBucket{updated: time.Now()}
	w := relayCall(t, ro, id, relayRequest{URL: upstream.URL, Method: "GET"}, "")
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Equal(t, "1", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), `"code":"relay_rate_limited"`)
	assert.Zero(t, calls.Load())

	// Once a token refills, an ordinary request can succeed with the same
	// consent and policy. The limiter is retained rather than bypassed.
	ro.relayRuntime.buckets["artifact:1:"+id] = relayBucket{updated: time.Now().Add(-time.Second)}
	w = relayCall(t, ro, id, relayRequest{URL: upstream.URL, Method: "GET"}, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, calls.Load())
}

func TestRelayThrottleRefusalsDoNotSpendOtherBudgets(t *testing.T) {
	runtime := newRelayRuntime()
	runtime.buckets["artifact:1:busy"] = relayBucket{updated: time.Now()}
	for range 150 {
		delay := runtime.reserveRequest(1, "busy", "https://api.example.test")
		require.Positive(t, delay)
		assert.LessOrEqual(t, delay, time.Second)
	}
	assert.Equal(t, 120.0, runtime.buckets["owner:1"].tokens)
	assert.Equal(t, 300.0, runtime.buckets["target:https://api.example.test"].tokens)
	assert.Zero(t, runtime.reserveRequest(1, "other", "https://api.example.test"))
}

func TestRelayThrottleKeepsUpstreamBackoffGuidance(t *testing.T) {
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(context.Background(), 1, id, store.RelayGrant{Allowed: true}))
	first := relayCall(t, ro, id, relayRequest{URL: upstream.URL}, "")
	require.Equal(t, http.StatusOK, first.Code, "a provider 429 remains the artifact's response")
	assert.Contains(t, first.Body.String(), `"status":429`)
	second := relayCall(t, ro, id, relayRequest{URL: upstream.URL}, "")
	require.Equal(t, http.StatusTooManyRequests, second.Code)
	assert.Equal(t, "120", second.Header().Get("Retry-After"))
}
