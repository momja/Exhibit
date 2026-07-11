package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The key round trip: PUT stores it encrypted, GET returns only a masked
// hint, DELETE unsets. The plaintext key must never come back and must not
// be stored in the clear.
func TestAgentKeyLifecycle(t *testing.T) {
	r := newTestRouter(t)

	w := doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"configured":false`)

	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-5",
		"api_key":  "sk-ant-supersecret-1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "supersecret")

	w = doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Configured bool   `json:"configured"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		KeyHint    string `json:"key_hint"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Configured)
	assert.Equal(t, "anthropic", got.Provider)
	assert.Equal(t, "claude-sonnet-4-5", got.Model)
	assert.Equal(t, "sk-…1234", got.KeyHint)
	assert.NotContains(t, w.Body.String(), "supersecret")

	// The stored ciphertext must not contain the plaintext key.
	k, err := r.cfg.Store.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, k)
	assert.NotContains(t, k.KeyCiphertext, "supersecret")

	w = doJSON(t, r, "DELETE", "/api/agent/key", nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = doJSON(t, r, "GET", "/api/agent/key", nil)
	assert.Contains(t, w.Body.String(), `"configured":false`)
}

func TestAgentKeyRejectsUnknownProvider(t *testing.T) {
	r := newTestRouter(t)
	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "definitely-not-a-provider",
		"api_key":  "k",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Without an agent manager configured (no pi binary), session routes degrade
// to 503 rather than panicking.
func TestAgentSessionsUnavailableWithoutManager(t *testing.T) {
	r := newTestRouter(t)
	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// The SSE route authenticates via ?token= because EventSource can't set
// headers; a wrong or missing token must 401.
func TestAgentEventsQueryTokenAuth(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/api/agent/sessions/nope/events", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	req = httptest.NewRequest("GET", "/api/agent/sessions/nope/events?token=secret", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Authenticated but no manager -> 503 (not 401).
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAgentPageServes(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/agent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "Agent API key"))
}
