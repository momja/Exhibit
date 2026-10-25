package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentKeyThinkingPreference(t *testing.T) {
	r := newTestRouter(t)
	save := func(body map[string]string, want int) {
		t.Helper()
		w := doJSON(t, r, "PUT", "/api/agent/key", body)
		require.Equal(t, want, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "private-key")
	}
	readLevel := func(want string) {
		t.Helper()
		w := doJSON(t, r, "GET", "/api/agent/key", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var got struct {
			ThinkingLevel string `json:"thinking_level"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, want, got.ThinkingLevel)
	}
	save(map[string]string{
		"provider": "anthropic", "api_key": "private-key-1234", "thinking_level": " high ",
	}, http.StatusOK)
	readLevel("high")
	before, err := r.cfg.Store.GetAgentKey(t.Context(), defaultOwnerID)
	require.NoError(t, err)

	// Changing thinking keeps the masked credential, just like changing model.
	save(map[string]string{"provider": "anthropic", "thinking_level": "off"}, http.StatusOK)
	readLevel("off")
	after, err := r.cfg.Store.GetAgentKey(t.Context(), defaultOwnerID)
	require.NoError(t, err)
	assert.Equal(t, before.KeyCiphertext, after.KeyCiphertext)

	// Older clients and key rotation may omit thinking; neither resets it.
	save(map[string]string{"provider": "anthropic", "model": "claude-sonnet-4-5"}, http.StatusOK)
	readLevel("off")
	save(map[string]string{"provider": "openai", "api_key": "replacement-private-key"}, http.StatusOK)
	readLevel("off")

	save(map[string]string{"provider": "openai", "thinking_level": "ultra"}, http.StatusBadRequest)
	readLevel("off")
	// An explicit empty value returns to the environment baseline.
	save(map[string]string{"provider": "openai", "thinking_level": ""}, http.StatusOK)
	readLevel("")
}

func TestAgentSessionThinkingPreferenceIsBYOKOnly(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.Agent = &agent.Manager{}
	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic", "api_key": "owner-private-key", "thinking_level": "off",
	})
	require.Equal(t, http.StatusOK, w.Code)
	request := withOwnerResolved(r, httptest.NewRequest("POST", "/api/agent/sessions", nil))
	options, ok := r.agentSessionOpts(httptest.NewRecorder(), request)
	require.True(t, ok)
	assert.Equal(t, "off", options.ThinkingLevel)
	assert.False(t, options.PlatformPaid)

	enablePlatformMode(r)
	options, ok = r.agentSessionOpts(httptest.NewRecorder(), request)
	require.True(t, ok)
	assert.Empty(t, options.ThinkingLevel, "hosted sessions must not use a saved personal preference")
	assert.True(t, options.PlatformPaid)
	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic", "thinking_level": "max",
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
	k, err := r.cfg.Store.GetAgentKey(t.Context(), defaultOwnerID)
	require.NoError(t, err)
	assert.Equal(t, "off", k.ThinkingLevel, "hosted mode leaves personal settings untouched")

	r.cfg.PlatformAgentKey = nil
	options, ok = r.agentSessionOpts(httptest.NewRecorder(), request)
	require.True(t, ok)
	assert.Equal(t, "off", options.ThinkingLevel)
}
