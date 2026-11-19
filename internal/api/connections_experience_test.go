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
	assert.Contains(t, w.Body.String(), "Allow this artifact")
	assert.NotContains(t, w.Body.String(), `name="sharing"`)
	assert.Contains(t, w.Body.String(), "Saved data")
	w = doJSON(t, ro, http.MethodGet, "/partials/agent-connection-requests?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "connection="+connection.ID+"#connection-access")
	assert.Contains(t, w.Body.String(), request.Reason)
}

func TestArtifactConnectionsOnlySuggestAvailableNextSteps(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		custom, advanced, account bool
	}{
		{name: "no setup"},
		{name: "custom setup only in advanced", custom: true},
		{name: "available advanced setup", custom: true, advanced: true},
		{name: "existing account", custom: true, account: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ro := newTestRouter(t)
			ro.cfg.Relay.Enabled = true
			ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: tc.custom, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
			id := createOwnArtifact(t, ro)
			if tc.account {
				_, err := ro.cfg.Connections.Create(context.Background(), 1, connectors.CreateRequest{Name: "My account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "fixture-only"}, connectors.UnlimitedLimits())
				require.NoError(t, err)
			}
			for _, base := range []string{"/connections", "/partials/connections"} {
				path := base + "?artifact=" + id
				if tc.advanced {
					path += "&advanced=1"
				}
				w := doJSON(t, ro, http.MethodGet, path, nil)
				require.Equal(t, http.StatusOK, w.Code)
				body := w.Body.String()
				if tc.account {
					assert.Contains(t, body, "Choose an account to review")
					assert.Contains(t, body, `data-action="submit->connections#grant"`)
				} else {
					assert.NotContains(t, body, "Choose an account to review")
					assert.NotContains(t, body, `data-action="submit->connections#grant"`)
					if tc.custom && tc.advanced {
						assert.Contains(t, body, "Connect your own account first")
					} else {
						assert.Contains(t, body, "No accounts are available for this artifact.")
						assert.NotContains(t, body, "Connect your own account first")
					}
				}
			}
		})
	}
}
