package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shortSecretScrubber() (*Service, *store.ConnectionRecord, recipe, credential, string) {
	s := New(Config{StandinsEnabled: true})
	token := standinPrefix + strings.Repeat("q", 48)
	s.standins[token] = standinBinding{viewerID: 1, connectionID: "short", expires: time.Now().Add(time.Hour)}
	return s, &store.ConnectionRecord{ID: "short", Provider: "basic"}, recipe{Header: "Authorization", Scheme: "Basic "}, credential{Username: "q", Password: "z"}, token
}

func TestResponseScrubbingRejectsExpansionBeforeAllocatingItsOutput(t *testing.T) {
	s, rec, policy, secret, _ := shortSecretScrubber()
	response := &egress.Response{Status: 200, Body: bytes.Repeat([]byte{'q'}, maxConnectionResponseBytes-1)}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := s.scrubResponse(rec, 1, policy, secret, response)
	runtime.ReadMemStats(&after)
	require.ErrorIs(t, err, ErrResponseSize)
	// The input string copy is bounded by the original 4 MiB response. The
	// projected 250 MiB replacement must never be allocated, even transiently.
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(8<<20))
	assert.Len(t, response.Body, maxConnectionResponseBytes-1)
}

func TestResponseScrubbingAlsoBoundsMetadataAndPreservesCompleteStandins(t *testing.T) {
	s, rec, policy, secret, token := shortSecretScrubber()
	response := &egress.Response{Status: 200, Body: []byte(`{"echo":"q:z","header":"Basic cTp6"}`), URL: "https://api.example/echo"}
	require.NoError(t, s.scrubResponse(rec, 1, policy, secret, response))
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body, &body))
	assert.Equal(t, token, body["echo"])
	assert.Equal(t, "Basic "+token, body["header"])
	assert.NotContains(t, string(response.Body), "cTp6")
	assert.NotContains(t, string(response.Body), "q:z")
	for _, response := range []*egress.Response{
		{Body: []byte("ok"), Header: http.Header{"Content-Language": {strings.Repeat("q", 16<<10)}}},
		{Body: []byte("ok"), URL: "https://api.example/" + strings.Repeat("q", 16<<10)},
		{Body: []byte("ok"), StatusText: strings.Repeat("q", 16<<10)},
		{Body: []byte("ok"), Header: http.Header{"Content-Language": {strings.Repeat("q", 700), strings.Repeat("q", 700)}}},
	} {
		assert.ErrorIs(t, s.scrubResponse(rec, 1, policy, secret, response), ErrResponseSize)
	}
}

func TestCachedResponseChecksDecodedBodyLimitBeforeDecoding(t *testing.T) {
	for _, size := range []int{maxConnectionResponseBytes, maxConnectionResponseBytes + 1} {
		raw, err := json.Marshal(egress.Response{Status: 200, Body: bytes.Repeat([]byte{'s'}, size)})
		require.NoError(t, err)
		response, err := decodeCachedResponse(string(raw))
		if size > maxConnectionResponseBytes {
			assert.ErrorIs(t, err, ErrResponseSize)
			assert.Nil(t, response)
		} else {
			require.NoError(t, err)
			assert.Len(t, response.Body, size)
		}
	}
	raw, err := json.Marshal(egress.Response{Status: 200, StatusText: "OK", URL: strings.Repeat("x", maxConnectionResponseMetadataBytes)})
	require.NoError(t, err)
	_, err = decodeCachedResponse(string(raw))
	assert.ErrorIs(t, err, ErrResponseSize)
	_, err = decodeCachedResponse(strings.Repeat("x", maxConnectionCacheJSONBytes+1))
	assert.ErrorIs(t, err, ErrResponseSize)
}

func TestOversizedScrubbedProviderResponseIsNeverReturnedOrCached(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("q", maxConnectionResponseBytes-1))
	}))
	defer provider.Close()
	c, err := s.save(ctx, 1, "Short Basic secret", "basic", "native", recipe{Origins: []string{provider.URL}, Header: "Authorization", Scheme: "Basic ", Sensitivity: "personal", CacheSeconds: 60}, credential{Username: "q", Password: "z"}, UnlimitedLimits())
	require.NoError(t, err)
	_, err = s.Grant(ctx, 1, "app", GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	response, err := doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/data")
	assert.ErrorIs(t, err, ErrResponseSize)
	assert.Nil(t, response)
	req, err := http.NewRequest(http.MethodGet, provider.URL+"/data", nil)
	require.NoError(t, err)
	_, err = s.cfg.Store.GetConnectionCache(ctx, c.ID, requestCacheKey(req), s.cfg.Now())
	assert.ErrorIs(t, err, store.ErrNotFound)
	// A cache created by an earlier version must meet the same decoded limit.
	raw, err := json.Marshal(egress.Response{Status: 200, Body: bytes.Repeat([]byte{'s'}, maxConnectionResponseBytes+1)})
	require.NoError(t, err)
	sealed, err := s.cfg.Box.Encrypt(string(raw))
	require.NoError(t, err)
	require.NoError(t, s.cfg.Store.PutConnectionCache(ctx, c.ID, requestCacheKey(req), sealed, s.cfg.Now().Add(time.Hour)))
	response, err = doRequest(t, s, 1, testGate(t, provider.URL), provider.URL+"/data")
	assert.ErrorIs(t, err, ErrResponseSize)
	assert.Nil(t, response)
}
