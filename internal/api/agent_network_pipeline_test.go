package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArxivFixtureFlowsThroughRelayAwareAgent(t *testing.T) {
	h := newPiHarness(t)
	ro := h.router
	ro.cfg.Relay.Enabled = true
	sessionID := startSessionFor(t, ro, "")
	defer ro.cfg.Agent.Close(defaultOwnerID, sessionID)
	session := ro.cfg.Agent.Get(defaultOwnerID, sessionID)
	require.NotNil(t, session)
	w := doJSON(t, ro, http.MethodPost, "/api/agent/sessions/"+sessionID+"/prompt", map[string]any{"message": "Build an arXiv paper search app"})
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Eventually(t, func() bool { return session.ArtifactID() != "" }, 10*time.Second, 25*time.Millisecond)
	waitForBody(t, ro, session.ArtifactID(), func(body string) bool {
		return strings.Contains(body, "fetch('https://export.arxiv.org/api/query?")
	}, "the direct arXiv fixture to save through the scoped tool")
	require.Eventually(t, func() bool {
		return strings.Contains(strings.Join(h.llm.messagesInRole("tool"), "\n"), "https://export.arxiv.org")
	}, 5*time.Second, 25*time.Millisecond)
	for _, prompt := range h.llm.systemPrompts() {
		assert.Contains(t, prompt, "including APIs without CORS headers")
		assert.Contains(t, prompt, "Each origin needs the user's approval")
		assert.Contains(t, prompt, "Never use a public CORS proxy or mode: 'no-cors'")
	}
}
