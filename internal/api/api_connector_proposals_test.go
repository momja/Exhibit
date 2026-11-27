package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enableAPIProposals(ro *Router) {
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
}

func TestAPIProposalScope(t *testing.T) {
	ro, registry := newScopedTestRouter(t)
	enableAPIProposals(ro)
	id := createArtifact(t, ro, map[string]any{"title": "App", "body": counterBody})
	other := createArtifact(t, ro, map[string]any{"title": "Other", "body": counterBody})
	grant, err := registry.Issue(1, id, "")
	require.NoError(t, err)
	spec := map[string]any{"name": "Weather", "origin": "https://api.weather.example.com", "auth": "api-key", "header": "X-API-Key", "reason": "Show weather"}
	path := "/api/artifacts/" + id + "/agent-api-proposals"
	w := doWithToken(t, ro, http.MethodPost, path, grant.Token(), spec)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var first struct {
		Proposal connectors.APIProposal `json:"proposal"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))
	require.Equal(t, "pending", first.Proposal.State)
	w = doWithToken(t, ro, http.MethodPost, path, grant.Token(), spec)
	assert.Contains(t, w.Body.String(), first.Proposal.ID)
	w = doWithToken(t, ro, http.MethodPost, "/api/artifacts/"+other+"/agent-api-proposals", grant.Token(), spec)
	assert.Equal(t, http.StatusForbidden, w.Code)
	spec["secret"] = "should-never-be-accepted"
	w = doWithToken(t, ro, http.MethodPost, path, grant.Token(), spec)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotContains(t, w.Body.String(), "should-never-be-accepted")
	connections, err := ro.cfg.Connections.List(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, connections)
}

func askMockAPIConnector(t *testing.T, h *piHarness) (session, artifact, requestID string, events <-chan []byte, unsubscribe func()) {
	t.Helper()
	enableAPIProposals(h.router)
	artifact = createArtifact(t, h.router, map[string]any{"title": "Weather", "body": counterBody})
	session = startSessionFor(t, h.router, artifact)
	events, unsubscribe = h.router.cfg.Agent.Get(defaultOwnerID, session).Subscribe()
	w := doJSON(t, h.router, http.MethodPost, "/api/agent/sessions/"+session+"/prompt", map[string]string{"message": "mock-propose-api-connector"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	seen := awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"exhibit_interaction"`) }, "API proposal")
	var event struct {
		RequestID string `json:"requestId"`
	}
	require.NoError(t, json.Unmarshal([]byte(seen[len(seen)-1]), &event))
	require.NotEmpty(t, event.RequestID)
	return session, artifact, event.RequestID, events, unsubscribe
}

func TestAPIConnectorPrivateApprovalRoundTrip(t *testing.T) {
	for _, approve := range []bool{true, false} {
		t.Run(map[bool]string{true: "approve", false: "deny"}[approve], func(t *testing.T) {
			h := newPiHarness(t)
			session, artifact, requestID, events, unsubscribe := askMockAPIConnector(t, h)
			defer unsubscribe()
			path := "/api/agent/sessions/" + session + "/interactions/" + requestID
			w := doJSON(t, h.router, http.MethodGet, path, nil)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "https://api.weather.example.com")
			assert.Contains(t, w.Body.String(), `type="password"`)
			assert.NotContains(t, w.Body.String(), `checked`)
			secret := "private-connector-key-only-in-http"
			if approve {
				for _, bad := range []map[string]any{
					{"revision": 1, "action": "approve", "secret": secret},
					{"revision": 1, "action": "approve", "trusted_origin": "https://evil.example", "secret": secret},
					{"revision": 1, "action": "approve", "trusted_origin": "https://api.weather.example.com"},
					{"revision": 1, "action": "approve", "trusted_origin": "https://api.weather.example.com", "secret": "sk-proj-fake-key-for-the-wrong-domain"},
					{"revision": 0, "action": "approve", "trusted_origin": "https://api.weather.example.com", "secret": secret},
				} {
					w = doJSON(t, h.router, http.MethodPost, path, bad)
					assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
					assert.NotContains(t, w.Body.String(), secret)
				}
			}
			body := map[string]any{"revision": 1, "action": "deny"}
			if approve {
				body["action"] = "approve"
				body["secret"] = secret
				body["trusted_origin"] = "https://api.weather.example.com"
			}
			grant, err := h.router.cfg.AgentCredentials.Issue(defaultOwnerID, artifact, session)
			require.NoError(t, err)
			w = doWithToken(t, h.router, http.MethodPost, path, grant.Token(), body)
			assert.Equal(t, http.StatusForbidden, w.Code)
			w = doJSON(t, h.router, http.MethodPost, path, body)
			require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
			settles(t, events, 1)
			w = doJSON(t, h.router, http.MethodPost, path, body)
			assert.Equal(t, http.StatusConflict, w.Code)
			connections, err := h.router.cfg.Connections.List(context.Background(), defaultOwnerID)
			require.NoError(t, err)
			if approve {
				require.Len(t, connections, 1)
			} else {
				assert.Empty(t, connections)
			}
			grants, err := h.router.cfg.Store.(*store.SQLiteStore).ListConnectionGrants(context.Background(), defaultOwnerID, artifact)
			require.NoError(t, err)
			assert.Empty(t, grants)
			w = doJSON(t, h.router, http.MethodGet, path, nil)
			assert.NotContains(t, w.Body.String(), secret)
			assert.NotContains(t, w.Body.String(), `type="password"`)
			s := h.router.cfg.Agent.Get(defaultOwnerID, session)
			replay, cancelReplay := s.Subscribe()
			defer cancelReplay()
			for {
				select {
				case line := <-replay:
					assert.NotContains(t, string(line), secret)
				default:
					goto drained
				}
			}
		drained:
			closeAndWait(t, h.router, session)
			history := keptConversation(t, h.router, artifact, session, 1)
			assert.NotContains(t, history.SessionFile, secret)
			h.llm.mu.Lock()
			defer h.llm.mu.Unlock()
			raw, err := json.Marshal(h.llm.turns)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), secret)
		})
	}
}
