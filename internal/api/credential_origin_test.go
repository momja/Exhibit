package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func customConnectionRouter(t *testing.T) *Router {
	t.Helper()
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	return ro
}

// Exh-sez0: a credential whose format belongs to a known service is refused
// when it is aimed somewhere else, and the refusal names both destinations so
// the app can ask a question a person can answer. Confirming the destination is
// the only way past it, and it is explicit in the request rather than implied by
// a prior warning.
func TestCreatingAConnectionWarnsWhenTheCredentialBelongsToAnotherService(t *testing.T) {
	ro := customConnectionRouter(t)
	payload := map[string]any{
		"name":   "Anthropic",
		"recipe": "bearer",
		"origin": "https://api-openai.co",
		"secret": "sk-ant-api03-not-a-real-key",
	}

	w := doJSON(t, ro, http.MethodPost, "/api/connections", payload)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "credential_origin_mismatch", body["code"])
	assert.Equal(t, "Anthropic API key", body["credential"])
	assert.Equal(t, "https://api.anthropic.com", body["expected_origin"])
	assert.Equal(t, "https://api-openai.co", body["origin"])
	assert.NotContains(t, w.Body.String(), "sk-ant-api03-not-a-real-key", "the refusal never echoes the credential")

	w = doJSON(t, ro, http.MethodGet, "/api/connections", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `"Anthropic"`, "a refused create stores nothing")

	payload["confirm_origin_mismatch"] = true
	w = doJSON(t, ro, http.MethodPost, "/api/connections", payload)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// The ordinary case is untouched: a credential aimed at its own service, and an
// opaque token aimed anywhere, both save without a warning.
func TestCreatingAConnectionWithoutAMismatchNeedsNoConfirmation(t *testing.T) {
	ro := customConnectionRouter(t)
	w := doJSON(t, ro, http.MethodPost, "/api/connections", map[string]any{
		"name": "Anthropic", "recipe": "bearer", "origin": "https://api.anthropic.com", "secret": "sk-ant-api03-not-a-real-key",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = doJSON(t, ro, http.MethodPost, "/api/connections", map[string]any{
		"name": "Private", "recipe": "bearer", "origin": "https://api.example.test", "secret": "opaque-token-99112",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

// The setup form names the destination as the person types it, and carries the
// block that the confirmation appears in. Both are server-rendered markup the
// controller fills in, never markup the script builds.
func TestConnectionSetupNamesTheDestinationAndReservesTheWarning(t *testing.T) {
	ro := customConnectionRouter(t)
	w := doJSON(t, ro, http.MethodGet, "/connections?advanced=1", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	page := w.Body.String()
	assert.Contains(t, page, `data-connections-target="destination"`)
	assert.Contains(t, page, `data-connections-target="destinationHost"`)
	assert.Contains(t, page, `data-action="input->connections#destinationChanged"`)
	// Every field edit invalidates the warning: the form captures input and
	// change events so a confirmation cannot outlive the values it described.
	assert.Contains(t, page, `input->connections#formEdited change->connections#formEdited`)
	assert.Contains(t, page, `data-connections-target="createWarningText"`)
	assert.Contains(t, page, `data-action="connections#confirmCreate"`)
	assert.Contains(t, page, "Credential is sent only to")
}
