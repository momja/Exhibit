package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrInteractionStopping = errors.New("the current request is stopping; please try again when it finishes")

var ErrConnectorPending = errors.New("connection approval is waiting")

const apiConnectorDialogTitle = "exhibit:api-connector:v1"

// The session retains only a pointer to the durable, immutable proposal. Key
// acquisition and validation happen in the owner-authenticated HTTP handler.
func (s *Session) receiveAPIConnector(line []byte) bool {
	var dialog struct{ Type, ID, Method, Title, Placeholder string }
	if json.Unmarshal(line, &dialog) != nil || dialog.Type != "extension_ui_request" || dialog.Method != "input" || dialog.Title != apiConnectorDialogTitle {
		return false
	}
	var request struct {
		ToolCallID string `json:"toolCallId"`
		ProposalID string `json:"proposalId"`
	}
	if json.Unmarshal([]byte(dialog.Placeholder), &request) != nil || request.ToolCallID == "" || dialog.ID == "" {
		s.sendQuestionCancellation(dialog.ID)
		return true
	}
	if _, err := uuid.Parse(request.ProposalID); err != nil {
		s.sendQuestionCancellation(dialog.ID)
		return true
	}
	if !s.addInteraction(Interaction{ID: dialog.ID, ToolCallID: request.ToolCallID, Kind: InteractionAPIConnector, Blocking: true, ProposalID: request.ProposalID}) {
		s.sendQuestionCancellation(dialog.ID)
	}
	return true
}

// ResolveAPIConnector serializes approval with Stop and competing browser tabs.
// resolve receives only the proposal id. Its closure lives for this HTTP call;
// neither the private key nor the callback is retained in session state or RPC.
// A failed RPC write leaves the interaction pending. The durable domain result
// makes the next attempt safe even when the connector was already created.
func (s *Session) ResolveAPIConnector(id string, revision uint64, resolve func(string) (string, error)) error {
	if revision == 0 {
		return ErrInvalidInteractionAction
	}
	s.writeMu.Lock()
	s.mu.Lock()
	interaction, ok := s.interactions[id]
	if !ok || interaction.Kind != InteractionAPIConnector || !interaction.Pending() || interaction.Revision != revision || s.closed || s.stopping || s.aborting {
		s.mu.Unlock()
		s.writeMu.Unlock()
		return ErrInteractionExpired
	}
	state, err := resolve(interaction.ProposalID)
	if err == nil && state != "approved" && state != "denied" && state != "disconnected" {
		err = ErrInvalidInteractionAction
	}
	if err == nil {
		value, _ := json.Marshal(map[string]string{"status": state})
		line, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": id, "value": string(value)})
		if _, err = s.stdin.Write(append(line, '\n')); err != nil {
			err = fmt.Errorf("send connector outcome: %w", err)
		}
	}
	if err == nil {
		interaction.State = InteractionCompleted
		if state == "denied" {
			interaction.State = InteractionCancelled
		}
		interaction.Revision++
		s.interactions[id] = interaction
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
