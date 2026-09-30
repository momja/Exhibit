package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The usage-policy guardrail (av-gust), end to end: a real `pi --mode rpc`
// sidecar with ext/guard.ts loaded, screening on the mock LLM. The mock blocks
// a message containing "mock-policy-violation" and answers garbage to one
// containing "mock-guard-garbage" (internal/mockllm).

var mockGuardrail = &agent.Guardrail{Provider: "exhibit-mock", Model: "exhibit-mock-1", APIKey: "guard-mock-key"}

const greenButtonArtifact = `<html><head><style>#submit-btn{background:#f7d51d}</style></head><body><button id="submit-btn">Count!</button></body></html>`

// awaitEvent reads a session's events until one satisfies want, returning
// every line seen on the way.
func awaitEvent(t *testing.T, events <-chan []byte, want func(string) bool, why string) []string {
	t.Helper()
	var seen []string
	deadline := time.After(60 * time.Second)
	for {
		select {
		case line := <-events:
			seen = append(seen, string(line))
			if want(string(line)) {
				return seen
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s; saw:\n%s", why, strings.Join(seen, "\n"))
		}
	}
}

func isGuardBlocked(line string) bool { return strings.Contains(line, `"exhibit_guard_blocked"`) }

// agentTurns counts the requests that reached the agent model, as opposed to
// the screen: only the agent's system prompt names the artifact builder.
func agentTurns(h *piHarness) int {
	n := 0
	for _, sp := range h.llm.systemPrompts() {
		if strings.Contains(sp, "You are the artifact builder") {
			n++
		}
	}
	return n
}

// screenedMessages returns the user message of every request that reached the
// screen rather than the agent.
func screenedMessages(h *piHarness) []string {
	h.llm.mu.Lock()
	defer h.llm.mu.Unlock()
	var out []string
	for _, turn := range h.llm.turns {
		if len(turn) == 0 || !strings.Contains(messageText(turn[0].Content), "You are a usage-policy screen") {
			continue
		}
		for _, m := range turn {
			if m.Role == "user" {
				out = append(out, messageText(m.Content))
			}
		}
	}
	return out
}

func TestGuardrailBlocksAPromptBeforeTheAgentRuns(t *testing.T) {
	h := newPiHarnessWith(t, false, mockGuardrail)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": greenButtonArtifact})

	session := startSessionFor(t, r, id)
	s := r.cfg.Agent.Get(defaultOwnerID, session)
	require.NotNil(t, s)
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "mock-policy-violation make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)

	seen := awaitEvent(t, events, isGuardBlocked, "the guardrail to block the prompt")
	stream := strings.Join(seen, "\n")
	assert.Contains(t, stream, agent.GuardrailBlockedReply, "the user sees the host's fixed reply")
	assert.NotContains(t, stream, "exhibit_guard:", "the extension's raw signal is not forwarded")
	assert.NotContains(t, stream, "agent_start", "a blocked prompt must not start a turn")
	assert.Zero(t, agentTurns(h), "the blocked prompt must never reach the agent model")
	assert.NotContains(t, artifactBody(t, r, id), "#22a15c")

	// The next, allowed prompt still runs, and still has the artifact source:
	// the opening data block rode the blocked prompt and must not be lost with it.
	w = doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)
	waitForBody(t, r, id, func(b string) bool { return strings.Contains(b, "#22a15c") },
		"an allowed prompt to run after a blocked one")
}

func TestGuardrailFailsClosed(t *testing.T) {
	h := newPiHarnessWith(t, false, mockGuardrail)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": greenButtonArtifact})

	session := startSessionFor(t, r, id)
	s := r.cfg.Agent.Get(defaultOwnerID, session)
	require.NotNil(t, s)
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "mock-guard-garbage make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)

	seen := awaitEvent(t, events, isGuardBlocked, "an unparseable screen reply to block the prompt")
	assert.Contains(t, strings.Join(seen, "\n"), agent.GuardrailBlockedReply,
		"a failed screen reads exactly like a refusal")
	assert.Zero(t, agentTurns(h))
}

// Stored artifact source is fenced data the user did not just type. The
// screen judges the user's words, so an artifact whose body happens to
// contain prohibited-looking text can still be edited.
func TestGuardrailScreensTheUsersWordsNotTheArtifact(t *testing.T) {
	h := newPiHarnessWith(t, false, mockGuardrail)
	r := h.router
	body := strings.Replace(greenButtonArtifact, "<body>", "<body><!-- mock-policy-violation -->", 1)
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": body})

	session := startSessionFor(t, r, id)
	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)
	waitForBody(t, r, id, func(b string) bool { return strings.Contains(b, "#22a15c") },
		"the agent to edit an artifact whose stored body the screen must not judge")

	// Passing proves nothing unless the screen ran, and ran on the user's words alone.
	screened := screenedMessages(h)
	require.NotEmpty(t, screened, "the prompt was never screened")
	for _, m := range screened {
		assert.Contains(t, m, "make the button green")
		assert.NotContains(t, m, "mock-policy-violation", "the stored body reached the screen")
		assert.NotContains(t, m, "UNTRUSTED DATA", "a data block reached the screen")
	}
}
