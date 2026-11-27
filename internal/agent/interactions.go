package agent

import (
	"encoding/json"
	"errors"
	"time"
)

// An Interaction is a server-owned request displayed in the agent panel. Its
// lifecycle and correlation are shared; the kind owns its payload and actions.
// Questions are the first kind. Adding a kind requires an explicit validator,
// action handler, and server template, never agent-supplied markup or code.
type Interaction struct {
	ID         string           `json:"id"`
	ToolCallID string           `json:"toolCallId"`
	Kind       string           `json:"kind"`
	State      string           `json:"state"`
	Revision   uint64           `json:"revision"`
	Blocking   bool             `json:"blocking"`
	Questions  *QuestionRequest `json:"questions,omitempty"`
}

const (
	InteractionQuestions = "questions"
	InteractionPending   = "pending"
	InteractionCompleted = "completed"
	InteractionCancelled = "cancelled"
	InteractionExpired   = "expired"
	InteractionBlocked   = "blocked"
)

var (
	ErrInteractionExpired       = errors.New("this request is no longer waiting for a response")
	ErrInvalidInteractionAction = errors.New("invalid interaction action")
)

func (interaction Interaction) Pending() bool { return interaction.State == InteractionPending }

// InteractionAction uses a validated, kind-specific response. Revision must
// match the form the person reviewed; zero is never accepted on this interface.
type InteractionAction struct {
	Revision  uint64
	Action    string
	Questions QuestionResponse
}

func (s *Session) Interaction(id string) (Interaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	interaction, ok := s.interactions[id]
	if ok && interaction.Questions != nil {
		// Project legacy view/history flags from the shared lifecycle, rather than
		// maintaining two independent copies of pending or cancellation state.
		questions := *interaction.Questions
		questions.Pending = interaction.Pending()
		questions.Cancelled = interaction.State == InteractionCancelled || interaction.State == InteractionExpired || interaction.State == InteractionBlocked
		questions.Expired = interaction.State == InteractionExpired
		questions.Blocked = interaction.State == InteractionBlocked
		interaction.Questions = &questions
	}
	return interaction, ok
}

// Caller holds mu. Only blocking pending interactions suspend ordinary prompts.
func (s *Session) hasPendingInteractions() bool {
	for _, interaction := range s.interactions {
		if interaction.Blocking && interaction.Pending() {
			return true
		}
	}
	return false
}

func (s *Session) addInteraction(interaction Interaction) bool {
	s.mu.Lock()
	if s.interactions == nil {
		s.interactions = map[string]Interaction{}
	}
	_, duplicate := s.interactions[interaction.ID]
	if s.closed || s.stopping || duplicate || len(s.interactions) >= 128 {
		s.mu.Unlock()
		return false
	}
	interaction.State, interaction.Revision = InteractionPending, 1
	s.interactions[interaction.ID] = interaction
	s.lastActive = time.Now()
	s.mu.Unlock()
	if interaction.Blocking {
		s.clearTurnClock()
	}
	s.interactionChanged(interaction.ID)
	return true
}

// RespondInteraction claims the current request under the same locks as tool
// cancellation. Validation and failed writes leave the form pending; competing
// tabs and stale forms can never produce a second tool reply.
func (s *Session) RespondInteraction(id string, action InteractionAction) error {
	if action.Revision == 0 {
		return ErrInvalidInteractionAction
	}
	return s.respondInteraction(id, action)
}

// A zero revision is reserved for the compatibility-only AnswerQuestions
// wrapper. New browser routes always call RespondInteraction.
func (s *Session) respondInteraction(id string, action InteractionAction) error {
	s.writeMu.Lock()
	s.mu.Lock()
	interaction, ok := s.interactions[id]
	if !ok || !interaction.Pending() || s.closed || s.stopping || (action.Revision != 0 && action.Revision != interaction.Revision) {
		s.mu.Unlock()
		s.writeMu.Unlock()
		return ErrInteractionExpired
	}
	var err error
	switch interaction.Kind {
	case InteractionQuestions:
		interaction, err = s.respondQuestions(interaction, action)
	default:
		err = ErrInvalidInteractionAction
	}
	if err == nil {
		interaction.Revision++
		s.interactions[id] = interaction
		s.lastActive = time.Now()
	}
	waiting := s.hasPendingInteractions()
	s.mu.Unlock()
	s.writeMu.Unlock()
	if err != nil {
		return err
	}
	if !waiting {
		s.armTurnClock()
	}
	s.interactionChanged(id)
	return nil
}

// expireInteractions retires pending requests when a tool is aborted or its
// session ends. A later reconnect must never revive an obsolete action form.
func (s *Session) expireInteractions(toolCallID string) {
	s.mu.Lock()
	var changed []string
	for id, interaction := range s.interactions {
		if interaction.Pending() && (toolCallID == "" || interaction.ToolCallID == toolCallID) {
			interaction.State = InteractionExpired
			interaction.Revision++
			s.interactions[id] = interaction
			changed = append(changed, id)
		}
	}
	s.mu.Unlock()
	for _, id := range changed {
		s.interactionChanged(id)
	}
}

func (s *Session) interactionChanged(id string) {
	interaction, ok := s.Interaction(id)
	if !ok {
		return
	}
	event, _ := json.Marshal(map[string]any{
		"type": "exhibit_interaction", "requestId": id, "kind": interaction.Kind,
		"state": interaction.State, "revision": interaction.Revision,
		"blocking": interaction.Blocking, "pending": interaction.Pending(),
	})
	s.broadcast(event)
}
