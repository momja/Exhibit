package agent

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIConnectorInteractionIsSecretFreeAndRevisionChecked(t *testing.T) {
	s, input := newQuestionSession(t)
	s.expireInteractions("")
	id := uuid.NewString()
	payload, _ := json.Marshal(map[string]string{"toolCallId": "tool-api", "proposalId": id})
	line, _ := json.Marshal(map[string]string{"type": "extension_ui_request", "method": "input", "id": "api-1", "title": apiConnectorDialogTitle, "placeholder": string(payload)})
	s.handleLine(line)
	called := 0
	resolve := func(proposalID string) (string, error) {
		called++
		assert.Equal(t, id, proposalID)
		return "approved", nil
	}
	require.ErrorIs(t, s.ResolveAPIConnector("api-1", 0, resolve), ErrInvalidInteractionAction)
	require.ErrorIs(t, s.ResolveAPIConnector("api-1", 2, resolve), ErrInteractionExpired)
	require.ErrorIs(t, s.RespondInteraction("api-1", InteractionAction{Revision: 1, Action: "approve"}), ErrInvalidInteractionAction)
	assert.Zero(t, called)
	require.Error(t, s.ResolveAPIConnector("api-1", 1, func(string) (string, error) { return "", errors.New("quota unavailable") }))
	interaction, _ := s.Interaction("api-1")
	assert.True(t, interaction.Pending())
	input.fail = true
	require.Error(t, s.ResolveAPIConnector("api-1", 1, resolve))
	interaction, _ = s.Interaction("api-1")
	assert.True(t, interaction.Pending())
	input.fail = false
	require.NoError(t, s.ResolveAPIConnector("api-1", 1, resolve))
	require.ErrorIs(t, s.ResolveAPIConnector("api-1", 1, resolve), ErrInteractionExpired)
	assert.Equal(t, 2, called)
	assert.Contains(t, input.String(), `\"status\":\"approved\"`)
	assert.NotContains(t, input.String(), id, "only outcome reaches subprocess")
	interaction, _ = s.Interaction("api-1")
	assert.Equal(t, InteractionCompleted, interaction.State)
}
