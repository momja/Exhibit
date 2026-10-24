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

func TestDistinctConnectionsAndStandaloneRequestsRemainIndependent(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	const bearerSecret, apiKeySecret = "first-api-bearer-8741", "second-api-key-6913"
	var firstCalls, secondCalls, standaloneCalls atomic.Int32
	second := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"), "the first connection's bearer must never reach the second API")
		if r.URL.Path == "/via-standalone" {
			assert.Empty(t, r.Header.Get("X-Second-Key"), "an unrelated request cannot acquire a credential by redirecting")
		} else {
			assert.Equal(t, apiKeySecret, r.Header.Get("X-Second-Key"))
		}
		fmt.Fprint(w, `{"api":"second"}`)
	}))
	defer second.Close()
	standalone := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		standaloneCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Second-Key"))
		if r.URL.Path == "/redirect-to-second" {
			http.Redirect(w, r, second.URL+"/via-standalone", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "body { color: navy; }")
	}))
	defer standalone.Close()
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls.Add(1)
		assert.Equal(t, "Bearer "+bearerSecret, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Second-Key"))
		switch r.URL.Path {
		case "/redirect-to-second":
			http.Redirect(w, r, second.URL+"/data", http.StatusFound)
		case "/redirect-to-standalone":
			http.Redirect(w, r, standalone.URL+"/style.css", http.StatusFound)
		default:
			fmt.Fprint(w, `{"api":"first"}`)
		}
	}))
	defer first.Close()
	firstConnection, err := s.Create(ctx, 1, CreateRequest{Name: "First API", Recipe: "bearer", Origin: first.URL, Secret: bearerSecret, DailyLimit: 4}, UnlimitedLimits())
	require.NoError(t, err)
	secondConnection, err := s.Create(ctx, 1, CreateRequest{Name: "Second API", Recipe: "api-key", Origin: second.URL, Header: "X-Second-Key", Secret: apiKeySecret, DailyLimit: 1}, UnlimitedLimits())
	require.NoError(t, err)
	for _, c := range []*Connection{firstConnection, secondConnection} {
		grant, err := s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
		assert.Empty(t, grant.PermittedEgress, "coexisting connections do not require reciprocal legacy data-destination consent")
	}
	base := []string{first.URL, second.URL, standalone.URL}
	allowed, err := s.AllowedEgress(ctx, 1, "app", base)
	require.NoError(t, err)
	assert.Equal(t, base, allowed)
	gate := testGate(t, base...)
	for _, target := range []string{first.URL + "/data", second.URL + "/data", standalone.URL + "/style.css"} {
		_, err := doRequest(t, s, 1, gate, target)
		require.NoError(t, err)
	}
	firstStandin, err := s.Standin(ctx, 1, "app", firstConnection.ID)
	require.NoError(t, err)
	secondStandin, err := s.Standin(ctx, 1, "app", secondConnection.ID)
	require.NoError(t, err)
	for _, misuse := range []struct{ target, header, value string }{
		{second.URL + "/data", "Authorization", "Bearer " + firstStandin.Value},
		{standalone.URL + "/style.css", "Authorization", "Bearer " + firstStandin.Value},
		{first.URL + "/data", "X-Second-Key", secondStandin.Value},
	} {
		req, err := http.NewRequest(http.MethodGet, misuse.target, nil)
		require.NoError(t, err)
		req.Header.Set(misuse.header, misuse.value)
		_, err = s.Do(ctx, 1, "app", req, gate)
		assert.ErrorIs(t, err, ErrStandin)
	}
	assert.Equal(t, int32(1), firstCalls.Load())
	assert.Equal(t, int32(1), secondCalls.Load())
	assert.Equal(t, int32(1), standaloneCalls.Load())
	valid, err := http.NewRequest(http.MethodGet, first.URL+"/data", nil)
	require.NoError(t, err)
	valid.Header.Set("Authorization", "Bearer "+firstStandin.Value)
	_, err = s.Do(ctx, 1, "app", valid, gate)
	require.NoError(t, err)
	for _, path := range []string{"/redirect-to-second", "/redirect-to-standalone"} {
		_, err := doRequest(t, s, 1, gate, first.URL+path)
		assert.ErrorIs(t, err, ErrEgress, "a credential-bearing request stays at its initial origin even when the next origin is approved")
	}
	assert.Equal(t, int32(4), firstCalls.Load())
	assert.Equal(t, int32(1), secondCalls.Load())
	assert.Equal(t, int32(1), standaloneCalls.Load())
	_, err = doRequest(t, s, 1, gate, standalone.URL+"/redirect-to-second")
	require.NoError(t, err)
	assert.Equal(t, int32(2), secondCalls.Load())
	_, err = doRequest(t, s, 1, gate, second.URL+"/data")
	assert.ErrorIs(t, err, store.ErrConnectionQuota, "the second connection keeps its independent quota")
	_, err = doRequest(t, s, 1, gate, standalone.URL+"/style.css")
	require.NoError(t, err, "spent connector quotas do not block unrelated approved assets")
	_, err = doRequest(t, s, 1, testGate(t, first.URL, second.URL), standalone.URL+"/style.css")
	var denied *egress.DeniedError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, standalone.URL, denied.Origin, "connection grants never approve unrelated origins")
}
