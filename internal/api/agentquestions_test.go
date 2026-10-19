package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func askMockQuestions(t *testing.T, h *piHarness) (session, artifact, requestID string, events <-chan []byte, unsubscribe func()) {
	t.Helper()
	artifact = createArtifact(t, h.router, map[string]any{"title": "Planner", "body": counterBody})
	session = startSessionFor(t, h.router, artifact)
	events, unsubscribe = h.router.cfg.Agent.Get(defaultOwnerID, session).Subscribe()
	w := doJSON(t, h.router, http.MethodPost, "/api/agent/sessions/"+session+"/prompt", map[string]string{"message": "mock-ask-questions"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	seen := awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"exhibit_questions"`) }, "the agent to ask questions")
	var event struct {
		RequestID string `json:"requestId"`
	}
	require.NoError(t, json.Unmarshal([]byte(seen[len(seen)-1]), &event))
	require.NotEmpty(t, event.RequestID)
	return session, artifact, event.RequestID, events, unsubscribe
}

func TestAgentQuestionsRoundTripAndHistory(t *testing.T) {
	h := newPiHarness(t)
	session, artifact, requestID, events, unsubscribe := askMockQuestions(t, h)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/questions/" + requestID
	w := doJSON(t, h.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "How would you like to see your plans?")
	assert.Contains(t, w.Body.String(), `type="radio"`)
	assert.Contains(t, w.Body.String(), "What would make this most useful for you?")
	assert.NotContains(t, w.Body.String(), " checked")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	w = doJSON(t, h.router, http.MethodPost, "/api/agent/sessions/"+session+"/prompt", map[string]string{"message": "an accidental message while answering"})
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "answer the questions above")

	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"answers": []agent.QuestionAnswer{{ID: "view", Selected: []string{"not a suggestion"}}}})
	require.Equal(t, http.StatusBadRequest, w.Code)
	answers := []agent.QuestionAnswer{{ID: "view", Selected: []string{"A weekly view"}}, {ID: "goal", Text: "Meals for my family <script>hello</script>"}}
	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"answers": answers})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	seen := awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the agent to continue after the answers")
	assert.Contains(t, strings.Join(seen, "\n"), "Meals for my family")
	assert.Contains(t, strings.Join(h.llm.messagesInRole("tool"), "\n"), "user's own instructions and preferences")

	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"answers": answers})
	assert.Equal(t, http.StatusConflict, w.Code, "a second tab cannot answer again")
	w = doJSON(t, h.router, http.MethodGet, path, nil)
	assert.NotContains(t, w.Body.String(), "data-question-form")
	assert.Contains(t, w.Body.String(), "A weekly view")
	assert.Contains(t, w.Body.String(), "&lt;script&gt;hello&lt;/script&gt;")
	assert.NotContains(t, w.Body.String(), "<script>hello</script>")

	kept := keptConversation(t, h.router, artifact, session, 1)
	var projected *agent.QuestionRequest
	for _, message := range agent.Messages(kept) {
		if message.Role == "questions" {
			projected = message.Questions
		}
	}
	require.NotNil(t, projected, "questions and answers survive Pi's persisted session file")
	assert.Equal(t, answers, projected.Answers)
	assert.False(t, projected.Pending)
	history := getPage(t, h.router, "/partials/agent-transcript?artifact="+artifact+"&session="+session)
	assert.Contains(t, history, "Meals for my family &lt;script&gt;hello&lt;/script&gt;")
	assert.NotContains(t, history, "data-question-form")
}

func TestAgentQuestionsSkipAndStopReleaseTheTool(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "skip", true: "stop"}[stop], func(t *testing.T) {
			h := newPiHarness(t)
			session, _, requestID, events, unsubscribe := askMockQuestions(t, h)
			defer unsubscribe()
			path := "/api/agent/sessions/" + session + "/questions/" + requestID
			if stop {
				w := doJSON(t, h.router, http.MethodPost, "/api/agent/sessions/"+session+"/abort", nil)
				require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
			} else {
				w := doJSON(t, h.router, http.MethodPost, path, map[string]bool{"cancelled": true})
				require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
			}
			awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the unanswered tool to settle")
			w := doJSON(t, h.router, http.MethodGet, path, nil)
			assert.Contains(t, w.Body.String(), "Questions skipped")
			assert.NotContains(t, w.Body.String(), "data-question-form")
		})
	}
}

func TestAgentQuestionsArePrivateToSessionOwner(t *testing.T) {
	h := newAgentOwnerHarness(t)
	session, artifact, requestID, events, unsubscribe := askMockQuestions(t, h.piHarness)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/questions/" + requestID
	w := doAs(t, h.router, http.MethodGet, path, h.ownerCookie, nil)
	require.Equal(t, http.StatusOK, w.Code)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w = doAs(t, h.router, method, path, h.intruderCookie, map[string]bool{"cancelled": true})
		assert.Equal(t, http.StatusNotFound, w.Code, "another account cannot read or answer live questions")
	}
	grant, err := h.router.cfg.AgentCredentials.Issue(defaultOwnerID, artifact, session)
	require.NoError(t, err)
	defer h.router.cfg.AgentCredentials.Revoke(grant)
	w = doWithToken(t, h.router, http.MethodPost, path, grant.Token(), map[string]bool{"cancelled": true})
	assert.Equal(t, http.StatusForbidden, w.Code, "an agent cannot submit an answer on the person's behalf")
	request, _ := h.router.cfg.Agent.Get(defaultOwnerID, session).Questions(requestID)
	assert.True(t, request.Pending, "refused answers leave the actual person's questions open")
	w = doAs(t, h.router, http.MethodPost, path, h.ownerCookie, map[string]bool{"cancelled": true})
	require.Equal(t, http.StatusNoContent, w.Code)
	settles(t, events, 1)
}

func TestAgentQuestionTemplateEscapesPromptsAndSupportsMultipleSelections(t *testing.T) {
	request := agent.QuestionRequest{ID: "r", Pending: true, Questions: []agent.Question{{ID: "q", Question: `What about <img src=x onerror=alert(1)>?`, MultiSelect: true, Options: []agent.QuestionOption{{Label: `A "quote"`, Description: `<b>detail</b>`}}}}}
	fragment, err := renderPage("agentQuestions", agentQuestionsData{QuestionRequest: request, SessionID: "s"})
	require.NoError(t, err)
	assert.Contains(t, fragment, `type="checkbox"`)
	assert.Contains(t, fragment, "Choose any that fit")
	assert.Contains(t, fragment, "&lt;img")
	assert.Contains(t, fragment, "&lt;b&gt;detail&lt;/b&gt;")
	assert.NotContains(t, fragment, "<img")
}
