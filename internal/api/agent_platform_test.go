package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Platform mode (av-siqf): the instance supplies the agent credential, and
// reports nothing about it — not the key, not the provider, not the model.

// enablePlatformMode puts an already-constructed router into platform mode.
// The mode is read per request rather than baked into route registration,
// which is what lets one router answer for both here.
func enablePlatformMode(r *Router) {
	r.cfg.PlatformAgentKey = &PlatformKey{
		Provider: "exhibit-mock",
		Model:    "exhibit-mock-1",
		APIKey:   "platform-secret-key",
	}
}

// The key resource does not exist in platform mode — every method on it, not
// just the read. A 404 rather than a 403 for the reason the rest of the API
// answers 404: a distinct refusal would be the one place the interface admits
// there is a credential it isn't showing.
func TestPlatformModeHidesTheKeyResource(t *testing.T) {
	r := newTestRouter(t)
	enablePlatformMode(r)

	for _, tc := range []struct {
		method string
		body   any
	}{
		{"GET", nil},
		{"PUT", map[string]string{"provider": "anthropic", "api_key": "sk-test"}},
		{"DELETE", nil},
	} {
		w := doJSON(t, r, tc.method, "/api/agent/key", tc.body)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s /api/agent/key", tc.method)
		assert.NotContains(t, w.Body.String(), "exhibit-mock")
		assert.NotContains(t, w.Body.String(), "platform-secret-key")
	}
}

// BYOK is untouched when the variable is unset: the same routes answer exactly
// as they did before platform mode existed.
func TestBYOKUnaffectedWhenPlatformKeyUnset(t *testing.T) {
	r := newTestRouter(t)

	w := doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"configured":false`)

	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic", "model": "claude-sonnet-4-5", "api_key": "sk-ant-1234567890",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"provider":"anthropic"`)

	w = doJSON(t, r, "DELETE", "/api/agent/key", nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// A key an owner entered before the instance switched modes is neither read
// nor deleted: flipping the variable back off restores their BYOK session with
// that key intact.
func TestPlatformModeLeavesStoredKeysAlone(t *testing.T) {
	r := newTestRouter(t)

	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic", "model": "claude-sonnet-4-5", "api_key": "sk-ant-owners-own-key",
	})
	require.Equal(t, http.StatusOK, w.Code)

	enablePlatformMode(r)
	// Whatever the surface does now, the row is still there.
	w = doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
	w = doJSON(t, r, "DELETE", "/api/agent/key", nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	k, err := r.cfg.Store.GetAgentKey(context.Background(), defaultOwnerID)
	require.NoError(t, err)
	require.NotNil(t, k, "the stored key must survive platform mode")
	plain, err := r.cfg.Secrets.Decrypt(k.KeyCiphertext)
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-owners-own-key", plain)

	// Turning it back off restores the owner's key exactly as it was.
	r.cfg.PlatformAgentKey = nil
	w = doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"provider":"anthropic"`)
}

// The page renders no key control at all — absent, not disabled — and names
// neither the provider nor the model the instance runs.
func TestPlatformModeAgentPageHasNoKeyUI(t *testing.T) {
	r := newTestRouter(t)
	enablePlatformMode(r)
	r.cfg.MockEnabled = true // the provider select's mock option, if it rendered

	req := httptest.NewRequest("GET", "/agent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	for _, absent := range []string{
		"key-btn", "key-modal", "key-provider", "key-model", "key-secret",
		"openKeyModal()", "Bring your own key",
		"exhibit-mock", "platform-secret-key",
	} {
		assert.NotContains(t, body, absent, "platform mode must not render %q", absent)
	}
	assert.Contains(t, body, "const BYOK = false", "page JS must know not to ask for a key")
}

// BYOK renders all of it, so the assertions above test the switch and not a
// typo in an element id.
func TestBYOKAgentPageRendersKeyUI(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MockEnabled = true

	req := httptest.NewRequest("GET", "/agent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	for _, present := range []string{
		"key-btn", "key-modal", "key-provider", "key-model", "key-secret",
		"Bring your own key", "const BYOK = true",
	} {
		assert.Contains(t, body, present)
	}
}

// The mode is the one thing that decides which credential a session runs on,
// and it decides it for an owner who has never stored a key. Availability is a
// separate signal and still comes first: with no manager this is a 503, in
// either mode, not a key error.
func TestPlatformModeResolvesOptsWithoutAStoredKey(t *testing.T) {
	r := newTestRouter(t)

	// BYOK, no stored key: the precondition failure this replaces.
	req := httptest.NewRequest("POST", "/api/agent/sessions", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	_, ok := r.agentSessionOpts(w, req)
	require.False(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, "no pi binary is reported before any key is")

	// Platform mode with a manager present is exercised end to end in
	// agent_platform_pipeline_test.go; here, prove the availability signal is
	// unchanged by the mode.
	enablePlatformMode(r)
	w = httptest.NewRecorder()
	_, ok = r.agentSessionOpts(w, req)
	assert.False(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// The edit page's "Generate widget" button gates on a key, and in platform
// mode there is none to find — and no screen to add one on. A disabled button
// telling the user to add an API key would send them somewhere that does not
// exist (av-fafu's affordance, av-siqf's mode).
func TestPlatformModeEnablesWidgetGeneration(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/", nil)

	// With no agent manager at all, availability is still about pi, in either
	// mode — the message a user can act on.
	ok, why := r.widgetGenerateAvailability(req)
	require.False(t, ok)
	assert.Contains(t, why, "pi binary")

	r.cfg.Agent = &agent.Manager{}
	ok, why = r.widgetGenerateAvailability(req)
	require.False(t, ok, "BYOK with no stored key is unchanged")
	assert.Contains(t, why, "agent API key")

	enablePlatformMode(r)
	ok, why = r.widgetGenerateAvailability(req)
	assert.True(t, ok)
	assert.Empty(t, why)
}

func TestPlatformKeyFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      map[string]string
		wantNil  bool
		wantErr  string
		provider string
		model    string
	}{
		{name: "unset is BYOK", env: nil, wantNil: true},
		{name: "blank is BYOK", env: map[string]string{"AGENT_API_KEY": "  "}, wantNil: true},
		{
			name:    "key without provider fails at startup",
			env:     map[string]string{"AGENT_API_KEY": "sk-test"},
			wantErr: "AGENT_PROVIDER",
		},
		{
			name:    "unknown provider fails at startup",
			env:     map[string]string{"AGENT_API_KEY": "sk-test", "AGENT_PROVIDER": "hal9000"},
			wantErr: `unsupported AGENT_PROVIDER "hal9000"`,
		},
		{
			name:     "the mock provider resolves, so the path is testable",
			env:      map[string]string{"AGENT_API_KEY": "mock-key", "AGENT_PROVIDER": "exhibit-mock", "AGENT_MODEL": "exhibit-mock-1"},
			provider: "exhibit-mock", model: "exhibit-mock-1",
		},
		{
			name:     "model is optional",
			env:      map[string]string{"AGENT_API_KEY": "sk-test", "AGENT_PROVIDER": "anthropic"},
			provider: "anthropic",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"AGENT_API_KEY", "AGENT_PROVIDER", "AGENT_MODEL"} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			pk, err := PlatformKeyFromEnv()
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, pk)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, pk)
				return
			}
			require.NotNil(t, pk)
			assert.Equal(t, tc.provider, pk.Provider)
			assert.Equal(t, tc.model, pk.Model)
		})
	}
}
