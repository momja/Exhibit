package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
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

// QuestionAction is deliberately question-specific. Private connector inputs
// must never be added here or to the shared resolution interface.
type QuestionAction struct {
	Revision uint64
	Action   string
	Response QuestionResponse
}

// AnswerQuestions is the compatibility-only unversioned question interface.
// It still claims the revision it observed; it cannot bypass common admission.
func (s *Session) AnswerQuestions(id string, response QuestionResponse) error {
	interaction, ok := s.Interaction(id)
	if !ok || interaction.Kind != InteractionQuestions {
		return ErrQuestionExpired
	}
	action := "answer"
	if response.Cancelled {
		action = "skip"
	} else if response.Message != "" {
		action = "message"
	}
	err := s.RespondQuestions(context.Background(), id, QuestionAction{Revision: interaction.Revision, Action: action, Response: response})
	if errors.Is(err, ErrInvalidInteractionAction) {
		return ErrInvalidAnswer
	}
	return err
}

// RespondQuestions uses the common lifecycle, but commits no answer until its
// safe reply is written successfully. Failed writes keep this revision editable;
// a later submission is freshly validated and sends that submission's answers.
func (s *Session) RespondQuestions(ctx context.Context, id string, action QuestionAction) error {
	return s.ResolveInteraction(ctx, id, action.Revision, InteractionQuestions,
		func(ctx context.Context, interaction Interaction) (InteractionCompletion, error) {
			return s.questionCompletion(interaction, action)
		})
}

func (s *Session) questionCompletion(interaction Interaction, action QuestionAction) (InteractionCompletion, error) {
	response := action.Response
	switch action.Action {
	case "answer":
		if response.Cancelled || response.Message != "" {
			return InteractionCompletion{}, ErrInvalidInteractionAction
		}
	case "skip":
		if len(response.Answers) != 0 || response.Message != "" || len(response.Images) != 0 || len(response.Snippets) != 0 {
			return InteractionCompletion{}, ErrInvalidInteractionAction
		}
		response.Cancelled = true
	case "message":
		if response.Message == "" || response.Cancelled || len(response.Answers) != 0 {
			return InteractionCompletion{}, ErrInvalidInteractionAction
		}
	default:
		return InteractionCompletion{}, ErrInvalidInteractionAction
	}
	if err := validateQuestionResponse(interaction.Questions.Questions, response); err != nil {
		return InteractionCompletion{}, err
	}
	if len(response.Snippets) != 0 {
		if err := s.writeSelection(response.Snippets); err != nil {
			return InteractionCompletion{}, err
		}
	}
	value, err := json.Marshal(struct {
		QuestionResponse
		RequestID string `json:"requestId"`
	}{response, interaction.ID})
	if err != nil {
		return InteractionCompletion{}, ErrInvalidAnswer
	}
	request := cloneQuestionRequest(interaction.Questions)
	request.Answers, request.Message = response.Answers, response.Message
	state := InteractionCompleted
	if response.Cancelled {
		state = InteractionCancelled
	}
	return InteractionCompletion{kind: InteractionQuestions, state: state, value: string(value), cancelled: response.Cancelled, questions: request}, nil
}

func cloneQuestionRequest(request *QuestionRequest) *QuestionRequest {
	if request == nil {
		return nil
	}
	copy := *request
	copy.Questions = slices.Clone(request.Questions)
	for i := range copy.Questions {
		copy.Questions[i].Options = slices.Clone(request.Questions[i].Options)
	}
	copy.Answers = slices.Clone(request.Answers)
	for i := range copy.Answers {
		copy.Answers[i].Selected = slices.Clone(request.Answers[i].Selected)
	}
	return &copy
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

// Rejected dialogs also use the bounded writer; a late dialog must not block
// the stdout reader forever while Stop is trying to recover an unresponsive Pi.
func (s *Session) sendQuestionCancellation(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	value, _ := json.Marshal(struct {
		Cancelled bool   `json:"cancelled"`
		RequestID string `json:"requestId"`
	}{true, id})
	line, _ := json.Marshal(struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Value     string `json:"value"`
		Cancelled bool   `json:"cancelled"`
	}{"extension_ui_response", id, string(value), true})
	_ = s.writePi(ctx, append(line, '\n'), nil)
}
