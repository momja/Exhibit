package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractionOwnsQuestionLifecycleAndRevision(t *testing.T) {
	s, input := newQuestionSession(t)
	interaction, ok := s.Interaction("request-1")
	require.True(t, ok)
	assert.Equal(t, InteractionQuestions, interaction.Kind)
	assert.Equal(t, InteractionPending, interaction.State)
	assert.EqualValues(t, 1, interaction.Revision)
	assert.True(t, interaction.Blocking)
	assert.True(t, interaction.Questions.Pending)
	require.NoError(t, s.RespondInteraction(interaction.ID, InteractionAction{Revision: 1, Action: "skip"}))
	completed, _ := s.Interaction(interaction.ID)
	assert.Equal(t, InteractionCancelled, completed.State)
	assert.EqualValues(t, 2, completed.Revision)
	assert.False(t, completed.Pending())
	assert.True(t, completed.Questions.Cancelled)
	assert.False(t, completed.Questions.Expired)
	assert.ErrorIs(t, s.RespondInteraction(interaction.ID, InteractionAction{Revision: 1, Action: "skip"}), ErrInteractionExpired)
	assert.Equal(t, 1, strings.Count(input.String(), "\n"))
	var event map[string]any
	require.NoError(t, json.Unmarshal(s.backlog[len(s.backlog)-1], &event))
	assert.Equal(t, "exhibit_interaction", event["type"])
	assert.Equal(t, "cancelled", event["state"])
	assert.Equal(t, float64(2), event["revision"])
}

func TestInteractionRejectsStaleMissingOrIncompatibleActions(t *testing.T) {
	for name, action := range map[string]InteractionAction{
		"stale revision":    {Revision: 8, Action: "skip"},
		"missing revision":  {Action: "skip"},
		"unknown action":    {Revision: 1, Action: "approve"},
		"answer as skip":    {Revision: 1, Action: "skip", Questions: QuestionResponse{Answers: []QuestionAnswer{{ID: "view", Text: "Weekly"}}}},
		"message as answer": {Revision: 1, Action: "answer", Questions: QuestionResponse{Message: "Change direction"}},
		"empty message":     {Revision: 1, Action: "message"},
	} {
		t.Run(name, func(t *testing.T) {
			s, input := newQuestionSession(t)
			require.Error(t, s.RespondInteraction("request-1", action))
			interaction, _ := s.Interaction("request-1")
			assert.True(t, interaction.Pending())
			assert.EqualValues(t, 1, interaction.Revision)
			assert.Empty(t, input.String())
		})
	}
}

func TestInteractionExpiresWithoutBeingRecordedAsAnExplicitSkip(t *testing.T) {
	s, _ := newQuestionSession(t)
	s.expireInteractions("unrelated-tool")
	interaction, _ := s.Interaction("request-1")
	require.True(t, interaction.Pending())
	s.expireInteractions("tool-1")
	interaction, _ = s.Interaction("request-1")
	assert.Equal(t, InteractionExpired, interaction.State)
	assert.True(t, interaction.Questions.Expired)
	assert.EqualValues(t, 2, interaction.Revision)
	s.expireInteractions("")
	interaction, _ = s.Interaction("request-1")
	assert.EqualValues(t, 2, interaction.Revision, "repeated cancellation does not invent a new revision")
}

func TestInteractionWriteFailurePreservesRevisionAndCanBeRetried(t *testing.T) {
	s, input := newQuestionSession(t)
	input.fail = true
	action := InteractionAction{Revision: 1, Action: "message", Questions: QuestionResponse{Message: "A shopping list instead"}}
	require.Error(t, s.RespondInteraction("request-1", action))
	interaction, _ := s.Interaction("request-1")
	assert.EqualValues(t, 1, interaction.Revision)
	assert.True(t, interaction.Pending())
	input.fail = false
	require.NoError(t, s.RespondInteraction("request-1", action))
	interaction, _ = s.Interaction("request-1")
	assert.Equal(t, InteractionCompleted, interaction.State)
	assert.Equal(t, "A shopping list instead", interaction.Questions.Message)
	s.blockQuestionReply("request-1", "tool-1")
	interaction, _ = s.Interaction("request-1")
	assert.Equal(t, InteractionBlocked, interaction.State)
	assert.EqualValues(t, 3, interaction.Revision)
	assert.Empty(t, interaction.Questions.Message)
}
