package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomConnectionSetupRequiresConfigurationAndAdvancedPage(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	cfg := connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets}
	ro.cfg.Connections = connectors.New(cfg)
	for _, path := range []string{"/connections", "/connections?advanced=1"} {
		w := doJSON(t, ro, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), `name="origin"`)
		assert.NotContains(t, w.Body.String(), `name="secret"`)
	}
	w := doJSON(t, ro, http.MethodPost, "/api/connections", map[string]any{"name": "Custom", "recipe": "bearer", "origin": "https://api.example.test", "secret": "fixture-key"})
	assert.NotEqual(t, http.StatusCreated, w.Code)
	cfg.CustomEnabled = true
	ro.cfg.Connections = connectors.New(cfg)
	w = doJSON(t, ro, http.MethodGet, "/connections", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), `name="origin"`)
	assert.NotContains(t, w.Body.String(), `name="secret"`)
	w = doJSON(t, ro, http.MethodGet, "/connections?advanced=1", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `name="origin"`)
	assert.Contains(t, w.Body.String(), `name="secret"`)
	assert.Contains(t, w.Body.String(), "Advanced connections")
}

func TestConnectionRequestActionsKeepToolAndSelectExistingAccount(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	st := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: st, Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	request, err := st.RequestArtifactConnection(context.Background(), 1, id, "bearer", "Show my saved reading list")
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodGet, "/connections?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Set up custom connection")
	assert.Contains(t, w.Body.String(), "advanced=1")
	assert.Contains(t, w.Body.String(), "artifact="+id)
	connection, err := ro.cfg.Connections.Create(context.Background(), 1, connectors.CreateRequest{Name: "My reading list", Recipe: "bearer", Origin: "https://reading.example.test", Secret: "fixture-only"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	w = doJSON(t, ro, http.MethodGet, "/connections?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "connection="+connection.ID+"#connection-access")
	assert.Contains(t, w.Body.String(), "Allow this tool")
	assert.NotContains(t, w.Body.String(), `name="sharing"`)
	assert.Contains(t, w.Body.String(), "Saved results are separate")
	w = doJSON(t, ro, http.MethodGet, "/partials/agent-connection-requests?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "connection="+connection.ID+"#connection-access")
	assert.Contains(t, w.Body.String(), request.Reason)
}
