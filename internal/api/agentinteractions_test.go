package api

import (
	"net/http"
	"testing"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentInteractionActionsUseTheRenderedRevision(t *testing.T) {
	h := newPiHarness(t)
	session, _, requestID, events, unsubscribe := askMockQuestions(t, h)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/interactions/" + requestID
	w := doJSON(t, h.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `id="agent-interaction-`+requestID+`"`)
	assert.Contains(t, w.Body.String(), `data-revision="1"`)
	for _, body := range []map[string]any{
		{"action": "skip"},
		{"revision": 1, "action": "approve"},
		{"revision": 1, "action": "skip", "message": "incompatible"},
		{"revision": 1, "action": "answer", "answers": []any{}},
	} {
		w = doJSON(t, h.router, http.MethodPost, path, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"revision": 2, "action": "skip"})
	assert.Equal(t, http.StatusConflict, w.Code)
	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"revision": 1, "action": "message", "message": "Make a shopping list instead."})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"revision": 1, "action": "skip"})
	assert.Equal(t, http.StatusConflict, w.Code)
	settles(t, events, 1)
	w = doJSON(t, h.router, http.MethodGet, path, nil)
	assert.Contains(t, w.Body.String(), "Make a shopping list instead.")
	assert.NotContains(t, w.Body.String(), "data-question-form")
}

func TestAgentInteractionsArePrivateToSessionOwner(t *testing.T) {
	h := newAgentOwnerHarness(t)
	session, artifact, requestID, events, unsubscribe := askMockQuestions(t, h.piHarness)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/interactions/" + requestID
	w := doAs(t, h.router, http.MethodGet, path, h.ownerCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w = doAs(t, h.router, method, path, h.intruderCookie, map[string]any{"revision": 1, "action": "skip"})
		assert.Equal(t, http.StatusNotFound, w.Code)
	}
	grant, err := h.router.cfg.AgentCredentials.Issue(defaultOwnerID, artifact, session)
	require.NoError(t, err)
	defer h.router.cfg.AgentCredentials.Revoke(grant)
	w = doWithToken(t, h.router, http.MethodPost, path, grant.Token(), map[string]any{"revision": 1, "action": "skip"})
	assert.Equal(t, http.StatusForbidden, w.Code, "a scoped agent cannot take a human action")
	interaction, _ := h.router.cfg.Agent.Get(defaultOwnerID, session).Interaction(requestID)
	assert.Equal(t, agent.InteractionPending, interaction.State)
	w = doAs(t, h.router, http.MethodPost, path, h.ownerCookie, map[string]any{"revision": 1, "action": "skip"})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	settles(t, events, 1)
}
