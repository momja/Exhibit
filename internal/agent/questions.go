package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Pi's input dialog is a correlated, abortable RPC request. The extension
// uses this private title to carry a questionnaire over that existing channel.
const questionDialogTitle = "exhibit:questions:v1"

var (
	ErrQuestionExpired = ErrInteractionExpired
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
	Images    []ImageContent   `json:"images,omitempty"`
	Snippets  []string         `json:"snippets,omitempty"`
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
	Blocked    bool             `json:"blocked,omitempty"`
	Expired    bool             `json:"expired,omitempty"`
}

func validQuestions(questions []Question) bool {
	if len(questions) < 1 || len(questions) > 4 {
		return false
	}
	ids := map[string]bool{}
	for _, q := range questions {
		if strings.TrimSpace(q.ID) == "" || utf8.RuneCountInString(q.ID) > 64 || ids[q.ID] || strings.TrimSpace(q.Question) == "" || utf8.RuneCountInString(q.Question) > 2000 || len(q.Options) > 6 {
			return false
		}
		ids[q.ID] = true
		labels := map[string]bool{}
		for _, option := range q.Options {
			if strings.TrimSpace(option.Label) == "" || utf8.RuneCountInString(option.Label) > 200 || utf8.RuneCountInString(option.Description) > 1000 || labels[option.Label] {
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
	request = QuestionRequest{ID: dialog.ID, ToolCallID: request.ToolCallID, Questions: request.Questions}
	if !s.addInteraction(Interaction{ID: dialog.ID, ToolCallID: request.ToolCallID, Kind: InteractionQuestions, Blocking: true, Questions: &request}) {
		s.sendQuestionCancellation(dialog.ID)
	}
	return true
}

// Questions is the compatibility projection used by stored transcripts and
// legacy clients. Live state lives exclusively in the interaction envelope.
func (s *Session) Questions(id string) (QuestionRequest, bool) {
	interaction, ok := s.Interaction(id)
	if !ok || interaction.Kind != InteractionQuestions || interaction.Questions == nil {
		return QuestionRequest{}, false
	}
	return *interaction.Questions, true
}

func validateAnswers(questions []Question, answers []QuestionAnswer) error {
	if len(questions) != len(answers) {
		return ErrInvalidAnswer
	}
	for i, q := range questions {
		a := answers[i]
		if a.ID != q.ID || utf8.RuneCountInString(a.Text) > 8000 || (len(a.Selected) == 0 && strings.TrimSpace(a.Text) == "") || (!q.MultiSelect && len(a.Selected) > 1) {
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
	if (len(response.Images) != 0 || len(response.Snippets) != 0) && (response.Message == "" || response.Cancelled) {
		return ErrInvalidAnswer
	}
	if response.Message != "" {
		if utf8.RuneCountInString(response.Message) > 8000 || strings.TrimSpace(response.Message) == "" || len(response.Answers) != 0 || response.Cancelled {
			return ErrInvalidAnswer
		}
		return nil
	}
	if response.Cancelled {
		return nil
	}
	return validateAnswers(questions, response.Answers)
}

// AnswerQuestions preserves the old question-only interface. New panel actions
// use RespondInteraction and supply the revision from the server-rendered form.
func (s *Session) AnswerQuestions(id string, response QuestionResponse) error {
	action := "answer"
	if response.Cancelled {
		action = "skip"
	} else if response.Message != "" {
		action = "message"
	}
	err := s.respondInteraction(id, InteractionAction{Action: action, Questions: response})
	if errors.Is(err, ErrInvalidInteractionAction) {
		return ErrInvalidAnswer
	}
	return err
}

// Caller holds writeMu and mu. Every kind validates and executes its own
// actions; the common layer commits the lifecycle only after a successful write.
func (s *Session) respondQuestions(interaction Interaction, action InteractionAction) (Interaction, error) {
	response := action.Questions
	switch action.Action {
	case "answer":
		if response.Cancelled || response.Message != "" {
			return interaction, ErrInvalidInteractionAction
		}
	case "skip":
		if len(response.Answers) != 0 || response.Message != "" || len(response.Images) != 0 || len(response.Snippets) != 0 {
			return interaction, ErrInvalidInteractionAction
		}
		response.Cancelled = true
	case "message":
		if response.Message == "" || response.Cancelled || len(response.Answers) != 0 {
			return interaction, ErrInvalidInteractionAction
		}
	default:
		return interaction, ErrInvalidInteractionAction
	}
	if err := validateQuestionResponse(interaction.Questions.Questions, response); err != nil {
		return interaction, err
	}
	if len(response.Snippets) != 0 {
		if err := s.writeSelection(response.Snippets); err != nil {
			return interaction, err
		}
	}
	if err := s.writeQuestionResponse(interaction.ID, response); err != nil {
		return interaction, err
	}
	request := *interaction.Questions
	request.Answers, request.Message = response.Answers, response.Message
	interaction.Questions = &request
	interaction.State = InteractionCompleted
	if response.Cancelled {
		interaction.State = InteractionCancelled
	}
	return interaction, nil
}

// A refusal is reported after the dialog reply has been screened. Retire the
// browser's accepted reply without retaining refused text in its summary.
func (s *Session) blockQuestionReply(id, toolCallID string) {
	s.mu.Lock()
	interaction, ok := s.interactions[id]
	if !ok || interaction.Kind != InteractionQuestions || interaction.ToolCallID != toolCallID {
		s.mu.Unlock()
		return
	}
	request := *interaction.Questions
	request.Answers, request.Message = nil, ""
	interaction.Questions, interaction.State = &request, InteractionBlocked
	interaction.Revision++
	s.interactions[id] = interaction
	s.mu.Unlock()
	s.interactionChanged(id)
}

// Caller serializes writes with writeMu.
func (s *Session) writeQuestionResponse(id string, response QuestionResponse) error {
	value, _ := json.Marshal(struct {
		QuestionResponse
		RequestID string `json:"requestId"`
	}{response, id})
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
