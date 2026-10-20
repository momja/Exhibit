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
	assert.Contains(t, w.Body.String(), "How should your plan look?")
	assert.Contains(t, w.Body.String(), "data-question-choice")
	assert.Contains(t, w.Body.String(), "What should the plan help you with?")
	assert.NotContains(t, w.Body.String(), `aria-pressed="true"`)
	assert.NotContains(t, w.Body.String(), ">Continue</button>")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	w = doJSON(t, h.router, http.MethodPost, "/api/agent/sessions/"+session+"/prompt", map[string]string{"message": "an accidental message while answering"})
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "answer the questions above")

	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"answers": []agent.QuestionAnswer{{ID: "view", Selected: []string{"not a suggestion"}}}})
	require.Equal(t, http.StatusBadRequest, w.Code)
	answers := []agent.QuestionAnswer{{ID: "view", Selected: []string{"Weekly calendar"}}, {ID: "goal", Text: "Meals for my family <script>hello</script>"}}
	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"answers": answers})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	seen := awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the agent to continue after the answers")
	assert.Contains(t, strings.Join(seen, "\n"), "Meals for my family")
	assert.Contains(t, strings.Join(h.llm.messagesInRole("tool"), "\n"), "user's own instructions and preferences")

	w = doJSON(t, h.router, http.MethodPost, path, map[string]any{"answers": answers})
	assert.Equal(t, http.StatusConflict, w.Code, "a second tab cannot answer again")
	w = doJSON(t, h.router, http.MethodGet, path, nil)
	assert.NotContains(t, w.Body.String(), "data-question-form")
	assert.Contains(t, w.Body.String(), "Weekly calendar")
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
			assert.Contains(t, w.Body.String(), "Skipped")
			assert.NotContains(t, w.Body.String(), "data-question-form")
		})
	}
}

func TestAgentQuestionChatReplyReachesTheToolAndHistory(t *testing.T) {
	h := newPiHarness(t)
	session, artifact, requestID, events, unsubscribe := askMockQuestions(t, h)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/questions/" + requestID
	message := "Actually, a shopping list <script>hello</script>"
	w := doJSON(t, h.router, http.MethodPost, path, map[string]string{"message": message})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the agent to continue after a chat reply")
	assert.Contains(t, strings.Join(h.llm.messagesInRole("tool"), "\n"), "Actually, a shopping list")
	w = doJSON(t, h.router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "data-question-form")
	assert.Contains(t, w.Body.String(), "Actually, a shopping list &lt;script&gt;hello&lt;/script&gt;")
	assert.NotContains(t, w.Body.String(), "<script>hello</script>")
	kept := keptConversation(t, h.router, artifact, session, 1)
	var projected *agent.QuestionRequest
	for _, entry := range agent.Messages(kept) {
		if entry.Role == "questions" {
			projected = entry.Questions
		}
	}
	require.NotNil(t, projected)
	assert.Equal(t, message, projected.Message)
	assert.Empty(t, projected.Answers)
	history := getPage(t, h.router, "/partials/agent-transcript?artifact="+artifact+"&session="+session)
	assert.Contains(t, history, "Actually, a shopping list &lt;script&gt;hello&lt;/script&gt;")
}

func TestGuardrailScreensQuestionRepliesBeforeTheAgentReceivesThem(t *testing.T) {
	for _, kind := range []string{"message", "answer"} {
		for _, text := range []string{"mock-policy-violation", "mock-guard-garbage"} {
			t.Run(kind+"/"+text, func(t *testing.T) {
				h := newPiHarnessWith(t, false, mockGuardrail)
				session, artifact, requestID, events, unsubscribe := askMockQuestions(t, h)
				defer unsubscribe()
				path := "/api/agent/sessions/" + session + "/questions/" + requestID
				var body any = map[string]string{"message": text}
				if kind == "answer" {
					body = map[string]any{"answers": []agent.QuestionAnswer{{ID: "view", Selected: []string{"Weekly calendar"}}, {ID: "goal", Text: text}}}
				}
				w := doJSON(t, h.router, http.MethodPost, path, body)
				require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
				seen := awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the refused question reply to settle")
				assert.Contains(t, strings.Join(seen, "\n"), agent.GuardrailBlockedReply)
				assert.NotContains(t, strings.Join(h.llm.messagesInRole("tool"), "\n"), text)
				assert.Contains(t, strings.Join(screenedMessages(h), "\n"), text)
				w = doJSON(t, h.router, http.MethodGet, path, nil)
				assert.NotContains(t, w.Body.String(), text)
				assert.Contains(t, w.Body.String(), "Reply not sent.")
				kept := keptConversation(t, h.router, artifact, session, 1)
				for _, message := range agent.Messages(kept) {
					if message.Questions != nil {
						assert.True(t, message.Questions.Blocked)
						assert.Empty(t, message.Questions.Message)
						assert.Empty(t, message.Questions.Answers)
					}
				}
			})
		}
	}
}

func TestRefusedReplyDoesNotReplaceAnEarlierQuestionAnswer(t *testing.T) {
	h := newPiHarnessWith(t, false, mockGuardrail)
	session, artifact, firstID, events, unsubscribe := askMockQuestions(t, h)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/questions/"
	w := doJSON(t, h.router, http.MethodPost, path+firstID, map[string]string{"message": "A weekly calendar for family meals"})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the first question reply to settle")
	w = doJSON(t, h.router, http.MethodPost, "/api/agent/sessions/"+session+"/prompt", map[string]string{"message": "mock-ask-questions"})
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	seen := awaitEvent(t, events, func(line string) bool {
		return strings.Contains(line, `"exhibit_questions"`) && strings.Contains(line, `"pending":true`)
	}, "the second question request")
	var next struct {
		RequestID string `json:"requestId"`
	}
	require.NoError(t, json.Unmarshal([]byte(seen[len(seen)-1]), &next))
	require.NotEqual(t, firstID, next.RequestID)
	// The deterministic provider reuses call_mock_1 in both turns. Matching a
	// refusal or saved result by tool ID alone would overwrite the first reply.
	w = doJSON(t, h.router, http.MethodPost, path+next.RequestID, map[string]string{"message": "mock-policy-violation"})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the second reply to be refused")
	w = doJSON(t, h.router, http.MethodGet, path+firstID, nil)
	assert.Contains(t, w.Body.String(), "A weekly calendar for family meals")
	assert.NotContains(t, w.Body.String(), "Reply not sent.")
	w = doJSON(t, h.router, http.MethodGet, path+next.RequestID, nil)
	assert.Contains(t, w.Body.String(), "Reply not sent.")
	var answers []*agent.QuestionRequest
	for _, message := range agent.Messages(keptConversation(t, h.router, artifact, session, 2)) {
		if message.Questions != nil {
			answers = append(answers, message.Questions)
		}
	}
	require.Len(t, answers, 2)
	assert.Equal(t, "A weekly calendar for family meals", answers[0].Message)
	assert.False(t, answers[0].Blocked)
	assert.True(t, answers[1].Blocked)
	assert.Empty(t, answers[1].Message)
}

func TestQuestionChatAttachmentsReachTheAgentAsData(t *testing.T) {
	h := newPiHarnessWith(t, false, mockGuardrail)
	session, _, requestID, events, unsubscribe := askMockQuestions(t, h)
	defer unsubscribe()
	path := "/api/agent/sessions/" + session + "/questions/" + requestID
	image := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jX1sAAAAASUVORK5CYII="
	for _, invalid := range [][]agentPromptImage{
		{{Data: "not base64!", MimeType: "image/png"}},
		{{Data: image, MimeType: "image/svg+xml"}},
		make([]agentPromptImage, maxAgentImages+1),
	} {
		w := doJSON(t, h.router, http.MethodPost, path, map[string]any{"message": "Make this larger", "images": invalid})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	w := doJSON(t, h.router, http.MethodPost, path, map[string]any{
		"message":  "Make the selected button larger",
		"snippets": []string{"<button>mock-policy-violation artifact content</button>"},
		"images":   []map[string]string{{"data": image, "mime_type": "image/png"}},
	})
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	seen := awaitEvent(t, events, func(line string) bool { return strings.Contains(line, `"agent_settled"`) }, "the attached reply to settle")
	assert.NotContains(t, strings.Join(seen, "\n"), agent.GuardrailBlockedReply)
	tools := strings.Join(h.llm.messagesInRole("tool"), "\n")
	assert.Contains(t, tools, "Make the selected button larger")
	assert.Contains(t, tools, "mock-policy-violation artifact content")
	assert.NotContains(t, strings.Join(screenedMessages(h), "\n"), "mock-policy-violation")
	h.llm.mu.Lock()
	defer h.llm.mu.Unlock()
	imageDelivered := false
	for _, turn := range h.llm.turns {
		for _, message := range turn {
			imageDelivered = imageDelivered || strings.Contains(string(message.Content), "data:image/png;base64,")
		}
	}
	assert.True(t, imageDelivered, "the captured image must reach the agent model")
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
	assert.Contains(t, fragment, `data-multi-select="true"`)
	assert.Contains(t, fragment, ">Done</button>")
	assert.Contains(t, fragment, "&lt;img")
	assert.Contains(t, fragment, "&lt;b&gt;detail&lt;/b&gt;")
	assert.NotContains(t, fragment, "<img")
}
