package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/scanner"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkWarningsFollowCreateReadAndSourceEdits(t *testing.T) {
	ro := newTestRouter(t)
	body := `<html><script>fetch('https://corsproxy.io/?url=example', {mode:'no-cors'})</script></html>`
	w := doJSON(t, ro, http.MethodPost, "/api/artifacts", map[string]any{"title": "Proxy app", "body": body})
	require.Equal(t, http.StatusCreated, w.Code)
	var created createArtifactResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Len(t, created.NetworkWarnings, 2)
	assert.Equal(t, "public_cors_proxy", created.NetworkWarnings[0].Code)
	assert.Equal(t, "no_cors", created.NetworkWarnings[1].Code)

	w = doJSON(t, ro, http.MethodGet, "/api/artifacts/"+created.Artifact.ID+"?body=true", nil)
	var read struct {
		NetworkWarnings []scanner.Warning `json:"network_warnings"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &read))
	assert.Equal(t, created.NetworkWarnings, read.NetworkWarnings)

	w = doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+created.Artifact.ID, map[string]any{
		"body": `<html><script>fetch('https://api.example/data')</script></html>`,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var updated updateArtifactResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Empty(t, updated.NetworkWarnings, "the same-turn tool can confirm the warning was fixed")
}

func TestNetworkReviewPartialsUseSavedSourceAndPreserveBlockedDecisions(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "Proxy app", "body": `<script>fetch('https://corsproxy.io/', {mode:'no-cors'})</script>`})
	w := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/origins", map[string]any{
		"origin": "https://corsproxy.io", "decision": "block", "source": "runtime",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = doJSON(t, ro, http.MethodGet, "/partials/network-approval?artifact="+id+"&mode=edit", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `data-network-warning="public_cors_proxy"`)
	assert.Contains(t, w.Body.String(), `data-network-warning="no_cors"`)
	assert.Contains(t, w.Body.String(), `value="https://corsproxy.io">`)
	assert.NotContains(t, w.Body.String(), `value="https://corsproxy.io" checked`)
	assert.Contains(t, w.Body.String(), `onclick="approveOrigins()"`)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	w = doJSON(t, ro, http.MethodGet, "/partials/network-warnings?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "opaque, unreadable")

	_, err := renderEditPage(&store.Artifact{ID: id, Title: "App"}, nil, nil, `<script>fetch('/', {mode:'no-cors'})</script>`, "", testPageCreds, testRenderURLs("https://render.example"), false, "", nil)
	require.NoError(t, err, "the edit page includes the initial server-rendered warning region")
}

func TestAgentConnectionRequestNoticeEscapesReasonAndLinksToConfiguration(t *testing.T) {
	ro := newTestRouter(t)
	s := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: s, Box: ro.cfg.Secrets})
	id := createArtifact(t, ro, map[string]any{"title": "Budget", "body": "<h1>Budget</h1>"})
	_, err := s.RequestArtifactConnection(context.Background(), 1, id, "basic", `<img src=x onerror=alert(1)> Read account balances`)
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodGet, "/partials/agent-connection-requests?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "/connections?artifact="+id)
	assert.Contains(t, w.Body.String(), "Read account balances")
	assert.Contains(t, w.Body.String(), "&lt;img")
	assert.NotContains(t, w.Body.String(), "<img")
}

func TestAgentNetworkApprovalIsAvailableWithPlatformCredentials(t *testing.T) {
	page, err := renderPage("agent", agentPageData{
		ArtifactJSON: "null", AgentEnabled: true, BYOK: false,
	})
	require.NoError(t, err)
	assert.Contains(t, page, `id="net-modal"`)
	assert.NotContains(t, page, `id="key-modal"`)
	assert.Contains(t, page, `id="agent-network-warnings"`)
	assert.Contains(t, page, `id="agent-connection-requests"`)
}
