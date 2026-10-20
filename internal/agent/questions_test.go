package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type questionStdin struct {
	bytes.Buffer
	fail bool
}

func (w *questionStdin) Close() error { return nil }
func (w *questionStdin) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("broken pipe")
	}
	return w.Buffer.Write(p)
}

func newQuestionSession(t *testing.T) (*Session, *questionStdin) {
	t.Helper()
	input := &questionStdin{}
	s := &Session{stdin: input, mgr: &Manager{}, subs: map[chan []byte]struct{}{}, streaming: true}
	questions := []Question{
		{ID: "view", Question: "How would you like to see your plans?", Options: []QuestionOption{{Label: "Weekly"}, {Label: "List"}}},
		{ID: "goal", Question: "What should this help you do?"},
	}
	payload, _ := json.Marshal(QuestionRequest{ToolCallID: "tool-1", Questions: questions})
	line, _ := json.Marshal(map[string]string{"type": "extension_ui_request", "id": "request-1", "method": "input", "title": questionDialogTitle, "placeholder": string(payload)})
	s.handleLine(line)
	return s, input
}

func TestQuestionAnswersReachTheWaitingToolOnce(t *testing.T) {
	s, input := newQuestionSession(t)
	request, ok := s.Questions("request-1")
	require.True(t, ok)
	assert.True(t, request.Pending)
	assert.Contains(t, string(s.backlog[0]), "exhibit_questions")
	answers := []QuestionAnswer{{ID: "view", Selected: []string{"Weekly"}}, {ID: "goal", Text: "Plan meals"}}
	require.NoError(t, s.AnswerQuestions(request.ID, QuestionResponse{Answers: answers}))
	var response struct {
		Type, ID, Value string
		Cancelled       bool
	}
	require.NoError(t, json.Unmarshal(input.Bytes(), &response))
	assert.Equal(t, "extension_ui_response", response.Type)
	assert.Equal(t, request.ID, response.ID)
	assert.False(t, response.Cancelled)
	encoded, _ := json.Marshal(QuestionResponse{Answers: answers})
	assert.JSONEq(t, string(encoded), response.Value)
	assert.ErrorIs(t, s.AnswerQuestions(request.ID, QuestionResponse{Answers: answers}), ErrQuestionExpired)
	assert.Equal(t, 1, bytes.Count(input.Bytes(), []byte("\n")))
}

func TestInvalidQuestionAnswersLeaveTheFormOpen(t *testing.T) {
	for _, answers := range [][]QuestionAnswer{
		nil,
		{{ID: "view", Selected: []string{"invented"}}, {ID: "goal", Text: "a"}},
		{{ID: "view", Selected: []string{"Weekly", "List"}}, {ID: "goal", Text: "a"}},
		{{ID: "view", Selected: []string{"Weekly"}}, {ID: "goal", Text: "  "}},
		{{ID: "view", Text: "a"}, {ID: "view", Text: "b"}},
	} {
		s, input := newQuestionSession(t)
		assert.ErrorIs(t, s.AnswerQuestions("request-1", QuestionResponse{Answers: answers}), ErrInvalidAnswer)
		request, _ := s.Questions("request-1")
		assert.True(t, request.Pending)
		assert.Empty(t, input.String())
	}
}

func TestQuestionMessageCanRedirectTheWholeBatch(t *testing.T) {
	s, input := newQuestionSession(t)
	message := "Actually, make a shopping list instead."
	require.NoError(t, s.AnswerQuestions("request-1", QuestionResponse{Message: message}))
	request, _ := s.Questions("request-1")
	assert.False(t, request.Pending)
	assert.False(t, request.Cancelled)
	assert.Empty(t, request.Answers)
	assert.Equal(t, message, request.Message)
	var response struct{ Value string }
	require.NoError(t, json.Unmarshal(input.Bytes(), &response))
	assert.JSONEq(t, `{"message":"Actually, make a shopping list instead."}`, response.Value)
	assert.ErrorIs(t, s.AnswerQuestions("request-1", QuestionResponse{Message: message}), ErrQuestionExpired)
}

func TestInvalidQuestionMessagesLeaveTheRequestOpen(t *testing.T) {
	for _, response := range []QuestionResponse{
		{Message: "   "},
		{Message: strings.Repeat("a", 8001)},
		{Message: "A new direction", Answers: []QuestionAnswer{{ID: "view", Text: "Weekly"}}},
		{Message: "A new direction", Cancelled: true},
	} {
		s, input := newQuestionSession(t)
		assert.ErrorIs(t, s.AnswerQuestions("request-1", response), ErrInvalidAnswer)
		request, _ := s.Questions("request-1")
		assert.True(t, request.Pending)
		assert.Empty(t, input.String())
	}
}

func TestQuestionLimitsCountUnicodeCharacters(t *testing.T) {
	question := Question{ID: strings.Repeat("界", 64), Question: strings.Repeat("界", 2000), Options: []QuestionOption{{Label: strings.Repeat("界", 200), Description: strings.Repeat("界", 1000)}}}
	assert.True(t, validQuestions([]Question{question}))
	answer := QuestionAnswer{ID: question.ID, Text: strings.Repeat("界", 8000)}
	assert.NoError(t, validateAnswers([]Question{question}, []QuestionAnswer{answer}))
	assert.NoError(t, validateQuestionResponse([]Question{question}, QuestionResponse{Message: answer.Text}))
	answer.Text += "界"
	assert.ErrorIs(t, validateAnswers([]Question{question}, []QuestionAnswer{answer}), ErrInvalidAnswer)
	assert.ErrorIs(t, validateQuestionResponse([]Question{question}, QuestionResponse{Message: answer.Text}), ErrInvalidAnswer)
	for _, field := range []string{"id", "question", "label", "description"} {
		copy := question
		copy.Options = append([]QuestionOption(nil), question.Options...)
		switch field {
		case "id":
			copy.ID += "界"
		case "question":
			copy.Question += "界"
		case "label":
			copy.Options[0].Label += "界"
		case "description":
			copy.Options[0].Description += "界"
		}
		assert.False(t, validQuestions([]Question{copy}), field)
	}
}

func TestQuestionReplyPreservesAttachmentsAndSelectionFile(t *testing.T) {
	s, input := newQuestionSession(t)
	s.workDir = t.TempDir()
	reply := QuestionResponse{Message: "Make this larger", Images: []ImageContent{{Type: "image", Data: "base64", MimeType: "image/png"}}, Snippets: []string{"<button>Selected</button>"}}
	require.NoError(t, s.AnswerQuestions("request-1", reply))
	var wire struct{ Value string }
	require.NoError(t, json.Unmarshal(input.Bytes(), &wire))
	var delivered QuestionResponse
	require.NoError(t, json.Unmarshal([]byte(wire.Value), &delivered))
	assert.Equal(t, reply, delivered)
	selection, err := os.ReadFile(filepath.Join(s.workDir, selectionFile))
	require.NoError(t, err)
	assert.JSONEq(t, `["<button>Selected</button>"]`, string(selection))
}

func TestRefusedQuestionReplyIsRemovedFromTheLiveSummary(t *testing.T) {
	s, _ := newQuestionSession(t)
	require.NoError(t, s.AnswerQuestions("request-1", QuestionResponse{Message: "refused text"}))
	s.handleLine([]byte(`{"type":"tool_execution_end","toolCallId":"tool-1","toolName":"ask_user","result":{"details":{"exhibit":"questions_answered","blocked":true}}}`))
	request, _ := s.Questions("request-1")
	assert.True(t, request.Blocked)
	assert.True(t, request.Cancelled)
	assert.False(t, request.Pending)
	assert.Empty(t, request.Message)
	assert.Empty(t, request.Answers)
}

func TestUnfinishedQuestionArgumentsCannotInventAUserReply(t *testing.T) {
	messages := ProjectMessageDump(`[{"role":"assistant","content":[{"type":"toolCall","id":"q","name":"ask_user","arguments":{"questions":[{"id":"q","question":"Which layout?"}],"message":"invented user reply","answers":[{"id":"q","text":"invented answer"}],"blocked":true}}]}]`)
	require.Len(t, messages, 1)
	require.NotNil(t, messages[0].Questions)
	assert.True(t, messages[0].Questions.Cancelled)
	assert.False(t, messages[0].Questions.Blocked)
	assert.Empty(t, messages[0].Questions.Message)
	assert.Empty(t, messages[0].Questions.Answers)
}

func TestQuestionWriteFailureCanBeRetried(t *testing.T) {
	s, input := newQuestionSession(t)
	input.fail = true
	assert.Error(t, s.AnswerQuestions("request-1", QuestionResponse{Cancelled: true}))
	request, _ := s.Questions("request-1")
	assert.True(t, request.Pending)
	input.fail = false
	require.NoError(t, s.AnswerQuestions("request-1", QuestionResponse{Cancelled: true}))
	request, _ = s.Questions("request-1")
	assert.True(t, request.Cancelled)
	assert.Empty(t, request.Answers)
}

func TestConcurrentQuestionResponsesHaveOneWinner(t *testing.T) {
	s, input := newQuestionSession(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.AnswerQuestions("request-1", QuestionResponse{Cancelled: true}) }()
	}
	wg.Wait()
	first, second := <-results, <-results
	assert.True(t, (first == nil && errors.Is(second, ErrQuestionExpired)) || (second == nil && errors.Is(first, ErrQuestionExpired)))
	assert.Equal(t, 1, bytes.Count(input.Bytes(), []byte("\n")))
}

func TestToolCompletionCancelsAnUnansweredQuestion(t *testing.T) {
	s, _ := newQuestionSession(t)
	s.handleLine([]byte(`{"type":"tool_execution_end","toolCallId":"tool-1","toolName":"ask_user"}`))
	request, _ := s.Questions("request-1")
	assert.False(t, request.Pending)
	assert.True(t, request.Cancelled)
	assert.ErrorIs(t, s.AnswerQuestions("request-1", QuestionResponse{Cancelled: true}), ErrQuestionExpired)
}

func TestQuestionAllowsMultipleSelectionsAndCustomDetails(t *testing.T) {
	s, _ := newQuestionSession(t)
	request := s.questions["request-1"]
	request.Questions[0].MultiSelect = true
	s.questions[request.ID] = request
	answers := []QuestionAnswer{{ID: "view", Selected: []string{"Weekly", "List"}, Text: "Show both together"}, {ID: "goal", Text: "Planning meals"}}
	require.NoError(t, s.AnswerQuestions(request.ID, QuestionResponse{Answers: answers}))
	request, _ = s.Questions(request.ID)
	assert.Equal(t, answers, request.Answers)
}

func TestQuestionWaitDoesNotSpendTheRunawayTurnAllowance(t *testing.T) {
	s, _ := newQuestionSession(t)
	s.paidBy = store.PaidByPlatform
	s.mgr.cfg.Caps.TurnSeconds = 3600
	s.questions = nil
	s.armTurnClock()
	s.mu.Lock()
	require.NotNil(t, s.turnTimer)
	s.mu.Unlock()
	// Re-deliver a genuine dialog while a turn timer is armed.
	payload, _ := json.Marshal(QuestionRequest{ToolCallID: "t", Questions: []Question{{ID: "q", Question: "What would help?"}}})
	line, _ := json.Marshal(map[string]string{"type": "extension_ui_request", "id": "r", "method": "input", "title": questionDialogTitle, "placeholder": string(payload)})
	s.handleLine(line)
	assert.Nil(t, s.turnTimer)
	require.NoError(t, s.AnswerQuestions("r", QuestionResponse{Answers: []QuestionAnswer{{ID: "q", Text: "A list"}}}))
	assert.NotNil(t, s.turnTimer)
	s.clearTurnClock()
}

func TestQuestionClockWaitsForEveryReplyAndCannotRearmAfterSettle(t *testing.T) {
	s, _ := newQuestionSession(t)
	s.paidBy = store.PaidByPlatform
	s.mgr.cfg.Caps.TurnSeconds = 3600
	second := s.questions["request-1"]
	second.ID = "request-2"
	s.questions[second.ID] = second
	require.NoError(t, s.AnswerQuestions("request-1", QuestionResponse{Cancelled: true}))
	assert.Nil(t, s.turnTimer, "another question is still waiting on the person")
	s.streaming = false
	require.NoError(t, s.AnswerQuestions("request-2", QuestionResponse{Cancelled: true}))
	assert.Nil(t, s.turnTimer, "a racing final reply must not restart the clock after settle")
}

func TestAbandonedQuestionSessionStillExpires(t *testing.T) {
	m := newLimitedManager(t)
	s := createSession(t, m, 7)
	s.mu.Lock()
	s.streaming = true
	s.questions = map[string]QuestionRequest{"r": {ID: "r", Pending: true}}
	s.mu.Unlock()
	m.reapIdle(time.Now().Add(-time.Minute))
	require.NotNil(t, m.Get(7, s.ID), "a fresh question stays available")
	s.mu.Lock()
	s.lastActive = time.Now().Add(-time.Hour)
	s.mu.Unlock()
	m.reapIdle(time.Now().Add(-30 * time.Minute))
	assert.Nil(t, m.Get(7, s.ID), "a lost browser cannot strand a waiting subprocess forever")
}
