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

func TestDisabledConnectionManagerKeepsRetainedDataControlsAvailable(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	policy := connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	connection, err := policy.Create(ctx, 1, connectors.CreateRequest{Name: "Private API", Recipe: "bearer", Origin: "https://private.example.test", Secret: "never-render-this-secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = policy.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetStateAsViewer(ctx, 1, id, "private", "saved financial data"))
	require.NoError(t, policy.Revoke(ctx, 1, connection.ID))
	ro.cfg.ConnectionPolicy = policy
	ro.cfg.Connections = nil
	ro.cfg.Relay = RelayConfig{}
	for _, path := range []string{"/connections?artifact=" + id, "/partials/connections?artifact=" + id} {
		w := doJSON(t, ro, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		body := w.Body.String()
		assert.Contains(t, body, "Saved results are separate")
		assert.NotContains(t, body, "Saved data protection")
		assert.Contains(t, body, `data-action="connections#clearData"`)
		assert.NotContains(t, body, "never-render-this-secret")
		assert.NotContains(t, body, "saved financial data")
		for _, form := range []string{"connections#create", "connections#grant", "connections#saveRelay", "connections#schedule"} {
			assert.NotContains(t, body, form, "disabled engines must not render an acquisition or relay form")
		}
	}
	w := doJSON(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-data", nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	w = doJSON(t, ro, http.MethodGet, "/connections?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `data-action="connections#clearData"`, "cleanup stays available without a network lock")
	assert.Contains(t, w.Body.String(), "Connections are switched off.")
	state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Empty(t, state)
}

func TestDisabledConnectionManagerStillAuthorizesArtifactContext(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay = RelayConfig{}
	require.NoError(t, ro.cfg.Store.PutArtifact(context.Background(), &store.Artifact{ID: "foreign-disabled-policy", OwnerID: 2, Title: "Other person's private app", Tier: store.Tier1, SourceBlobID: "unreadable-body"}))
	for _, path := range []string{"/connections?artifact=foreign-disabled-policy", "/partials/connections?artifact=foreign-disabled-policy", "/connections?artifact=missing"} {
		w := doJSON(t, ro, http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.NotContains(t, w.Body.String(), "Other person's private app")
	}
}
