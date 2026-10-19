package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Pi's input dialog is a correlated, abortable RPC request. The extension
// uses this private title to carry a questionnaire over that existing channel.
const questionDialogTitle = "exhibit:questions:v1"

var (
	ErrQuestionExpired = errors.New("these questions are no longer waiting for an answer")
	ErrInvalidAnswer   = errors.New("please answer every question by choosing an option or writing a response")
	ErrQuestionPending = errors.New("please answer the questions above or reply in chat")
)

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type Question struct {
	ID          string           `json:"id"`
	Question    string           `json:"question"`
	Options     []QuestionOption `json:"options,omitempty"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
}

type QuestionAnswer struct {
	ID       string   `json:"id"`
	Selected []string `json:"selected,omitempty"`
	Text     string   `json:"text,omitempty"`
}

// A chat message can answer or redirect the whole request without forcing the
// person to fill in each field. Structured answers and a message are exclusive.
type QuestionResponse struct {
	Answers   []QuestionAnswer `json:"answers,omitempty"`
	Message   string           `json:"message,omitempty"`
	Cancelled bool             `json:"cancelled,omitempty"`
}

// QuestionRequest is also the view model for the server-rendered chat card.
// Questions never change; a response replaces the pending state once.
type QuestionRequest struct {
	ID         string           `json:"id"`
	ToolCallID string           `json:"toolCallId"`
	Questions  []Question       `json:"questions"`
	Answers    []QuestionAnswer `json:"answers,omitempty"`
	Message    string           `json:"message,omitempty"`
	Pending    bool             `json:"pending"`
	Cancelled  bool             `json:"cancelled"`
}

func validQuestions(questions []Question) bool {
	if len(questions) < 1 || len(questions) > 4 {
		return false
	}
	ids := map[string]bool{}
	for _, q := range questions {
		if strings.TrimSpace(q.ID) == "" || len(q.ID) > 64 || ids[q.ID] || strings.TrimSpace(q.Question) == "" || len(q.Question) > 2000 || len(q.Options) > 6 {
			return false
		}
		ids[q.ID] = true
		labels := map[string]bool{}
		for _, option := range q.Options {
			if strings.TrimSpace(option.Label) == "" || len(option.Label) > 200 || len(option.Description) > 1000 || labels[option.Label] {
				return false
			}
			labels[option.Label] = true
		}
	}
	return true
}

// receiveQuestions consumes only our extension's dialog; other extension UI
// requests (including guardrail notifications) keep their existing handling.
func (s *Session) receiveQuestions(line []byte) bool {
	var dialog struct {
		Type, ID, Method, Title, Placeholder string
	}
	if json.Unmarshal(line, &dialog) != nil || dialog.Type != "extension_ui_request" || dialog.Method != "input" || dialog.Title != questionDialogTitle {
		return false
	}
	var request QuestionRequest
	if json.Unmarshal([]byte(dialog.Placeholder), &request) != nil || !validQuestions(request.Questions) || request.ToolCallID == "" || dialog.ID == "" {
		s.sendQuestionCancellation(dialog.ID)
		return true
	}
	request.ID, request.Pending, request.Cancelled, request.Answers = dialog.ID, true, false, nil
	request.Message = ""
	s.mu.Lock()
	if s.questions == nil {
		s.questions = map[string]QuestionRequest{}
	}
	if s.closed || s.stopping || len(s.questions) >= 128 {
		s.mu.Unlock()
		s.sendQuestionCancellation(dialog.ID)
		return true
	}
	s.questions[request.ID] = request
	s.lastActive = time.Now()
	s.mu.Unlock()
	// Time spent waiting for a person is not a runaway model turn.
	s.clearTurnClock()
	s.questionsChanged(request.ID)
	return true
}

func (s *Session) Questions(id string) (QuestionRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.questions[id]
	return request, ok
}

// Caller holds mu.
func (s *Session) hasPendingQuestions() bool {
	for _, request := range s.questions {
		if request.Pending {
			return true
		}
	}
	return false
}

func validateAnswers(questions []Question, answers []QuestionAnswer) error {
	if len(questions) != len(answers) {
		return ErrInvalidAnswer
	}
	for i, q := range questions {
		a := answers[i]
		if a.ID != q.ID || len(a.Text) > 8000 || (len(a.Selected) == 0 && strings.TrimSpace(a.Text) == "") || (!q.MultiSelect && len(a.Selected) > 1) {
			return ErrInvalidAnswer
		}
		seen := map[string]bool{}
		for _, label := range a.Selected {
			valid := false
			for _, option := range q.Options {
				valid = valid || label == option.Label
			}
			if !valid || seen[label] {
				return ErrInvalidAnswer
			}
			seen[label] = true
		}
	}
	return nil
}

func validateQuestionResponse(questions []Question, response QuestionResponse) error {
	if response.Message != "" {
		if len(response.Message) > 8000 || strings.TrimSpace(response.Message) == "" || len(response.Answers) != 0 || response.Cancelled {
			return ErrInvalidAnswer
		}
		return nil
	}
	if response.Cancelled {
		return nil
	}
	return validateAnswers(questions, response.Answers)
}

// AnswerQuestions claims a request while holding the same lock used for
// cancellation. Two tabs cannot answer it twice, and a failed write leaves it
// pending so the person can retry. UI replies have no normal RPC response.
func (s *Session) AnswerQuestions(id string, response QuestionResponse) error {
	s.writeMu.Lock()
	s.mu.Lock()
	request, ok := s.questions[id]
	if !ok || !request.Pending || s.closed || s.stopping {
		s.mu.Unlock()
		s.writeMu.Unlock()
		return ErrQuestionExpired
	}
	if err := validateQuestionResponse(request.Questions, response); err != nil {
		s.mu.Unlock()
		s.writeMu.Unlock()
		return err
	}
	if response.Cancelled {
		response.Answers = nil
	}
	err := s.writeQuestionResponse(id, response)
	if err == nil {
		request.Pending, request.Cancelled, request.Answers, request.Message = false, response.Cancelled, response.Answers, response.Message
		s.questions[id] = request
		s.lastActive = time.Now()
	}
	s.mu.Unlock()
	s.writeMu.Unlock()
	if err != nil {
		return err
	}
	s.armTurnClock()
	s.questionsChanged(id)
	return nil
}

// Caller serializes writes with writeMu.
func (s *Session) writeQuestionResponse(id string, response QuestionResponse) error {
	value, _ := json.Marshal(response)
	line, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": id, "value": string(value), "cancelled": response.Cancelled})
	if _, err := s.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("send answer to agent: %w", err)
	}
	return nil
}

func (s *Session) sendQuestionCancellation(id string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.writeQuestionResponse(id, QuestionResponse{Cancelled: true})
}

// cancelQuestions also retires requests when a tool is aborted by Pi or the
// process exits, so reconnecting cannot revive an obsolete answer form.
func (s *Session) cancelQuestions(toolCallID string) {
	s.mu.Lock()
	var changed []string
	for id, request := range s.questions {
		if request.Pending && (toolCallID == "" || request.ToolCallID == toolCallID) {
			request.Pending, request.Cancelled = false, true
			s.questions[id] = request
			changed = append(changed, id)
		}
	}
	s.mu.Unlock()
	for _, id := range changed {
		s.questionsChanged(id)
	}
}

func (s *Session) questionsChanged(id string) {
	request, _ := s.Questions(id)
	ev, _ := json.Marshal(map[string]any{"type": "exhibit_questions", "requestId": id, "pending": request.Pending})
	s.broadcast(ev)
}
