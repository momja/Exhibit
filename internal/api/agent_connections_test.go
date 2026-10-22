package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConnectionToolsExposeMetadataAndQueueOnly(t *testing.T) {
	ro, registry := newScopedTestRouter(t)
	s := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: s, Box: ro.cfg.Secrets})
	connection, err := ro.cfg.Connections.Create(context.Background(), 1, connectors.CreateRequest{
		Name: "Reading list API", Recipe: "api-key", Origin: "https://api.example", Secret: "never-in-agent-context",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	id := createArtifact(t, ro, map[string]any{"title": "Reading list", "body": "<h1>Reading list</h1>"})
	other := createArtifact(t, ro, map[string]any{"title": "Other", "body": "<h1>Other</h1>"})
	grant, err := registry.Issue(1, id, "")
	require.NoError(t, err)
	path := "/api/artifacts/" + id + "/agent-connections"

	w := doWithToken(t, ro, http.MethodGet, path, grant.Token(), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Reading list API")
	assert.Contains(t, w.Body.String(), "https://api.example")
	assert.NotContains(t, w.Body.String(), "never-in-agent-context")
	assert.NotContains(t, w.Body.String(), "encrypted_secret")
	assert.NotContains(t, w.Body.String(), "recipe_json")
	assert.NotContains(t, w.Body.String(), connection.ID)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	w = doWithToken(t, ro, http.MethodPost, path, grant.Token(), map[string]any{"provider": "simplefin", "reason": "Show account balances"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var response struct {
		Request store.ArtifactConnectionRequest `json:"request"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "simplefin", response.Request.Provider)
	requests, err := s.ListArtifactConnectionRequests(context.Background(), 1, id)
	require.NoError(t, err)
	require.Len(t, requests, 1)
	grants, err := s.ListConnectionGrants(context.Background(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, grants, "requesting setup never grants access")

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w = doWithToken(t, ro, method, "/api/artifacts/"+other+"/agent-connections", grant.Token(), map[string]any{"provider": "simplefin", "reason": "Wrong artifact"})
		assert.Equal(t, http.StatusForbidden, w.Code)
	}
	w = doWithToken(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-requests/"+response.Request.ID, grant.Token(), nil)
	assert.Equal(t, http.StatusForbidden, w.Code, "an agent cannot dismiss the person's prompts")
	w = doJSON(t, ro, http.MethodDelete, "/api/artifacts/"+id+"/connection-requests/"+response.Request.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestAgentConnectionRequestsRequireAnEnabledProviderAndRelay(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "App", "body": "<h1>App</h1>"})
	path := "/api/artifacts/" + id + "/agent-connections"
	w := doJSON(t, ro, http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	for _, body := range []map[string]any{
		{"provider": "attacker-recipe", "reason": "Not shipped policy"},
		{"provider": "github", "reason": "Disabled phase"},
		{"provider": "simplefin", "reason": ""},
	} {
		w = doJSON(t, ro, http.MethodPost, path, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
}
