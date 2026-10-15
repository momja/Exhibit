package agent

import (
	"context"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Resuming a conversation (av-b4yh) against the usage meter (av-2yws). A
// resumed session is a new process over an old conversation, and two of the
// meter's assumptions are about the process rather than the conversation.

// Pi's own totals count the whole conversation, the turns before this process
// included, and every settle reconciles the ledger against them. The ledger
// already holds those turns, so a resumed session has to start from them:
// starting from nothing, its first settle reads the whole earlier conversation
// as spend nobody recorded and records it a second time.
func TestAResumedSessionDoesNotRecordTheEarlierTurnsAgain(t *testing.T) {
	m := newLimitedManager(t)
	ctx := context.Background()
	const owner, conversation = int64(7), "conversation-1"

	earlier := store.AgentUsage{
		OwnerID: owner, SessionID: conversation, Provider: "anthropic", Model: "test-model",
		PaidBy: store.PaidByUser, Source: store.UsageSourceModel,
		InputTokens: 100, OutputTokens: 10, CostMicros: 10_000,
	}
	require.NoError(t, m.st.RecordAgentUsage(ctx, earlier))
	// Screening is operator spend Pi never counts; it must not be taken for
	// part of what Pi's totals already include.
	screen := earlier
	screen.Source, screen.PaidBy, screen.InputTokens, screen.OutputTokens, screen.CostMicros =
		store.UsageSourceGuardrail, store.PaidByPlatform, 180, 1, 900
	require.NoError(t, m.st.RecordAgentUsage(ctx, screen))

	s, err := m.Create(ctx, CreateOpts{
		OwnerID: owner, Provider: "anthropic", APIKey: "k", ArtifactID: "art-1",
		Resume: &store.Transcript{SessionID: conversation, Title: "make it purple", SessionFile: "{}\n"},
	})
	require.NoError(t, err)
	assert.Equal(t, conversation, s.ID, "a conversation keeps its identity")
	assert.Equal(t, store.UsageTotals{InputTokens: 100, OutputTokens: 10, CostMicros: 10_000},
		s.usageRecorded, "what the conversation had already spent, and not the screens")

	// One more turn: Pi's total is the earlier turns plus this one.
	s.stdin = statsResponder{s: s, respond: func(id string) {
		s.handleLine([]byte(statsLine(id, 150, 15, 0.0125)))
	}}
	feed(t, s, `{"type":"message_end","message":{"role":"assistant",`+
		`"usage":{"input":50,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":55,"cost":{"total":0.0025}}}}`)
	s.reconcileUsage()

	assert.Equal(t, store.UsageTotals{InputTokens: 150, OutputTokens: 15, CostMicros: 12_500},
		sessionTotals(t, m.st.(*store.SQLiteStore), s), "every turn once: the earlier one, and this one")
	instance, err := m.st.InstanceAgentSpend(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, int64(900), instance.CostMicros, "platform spend here is the screen alone")
}

// A resumed conversation is told, on its first prompt, that the artifact may
// have changed since it last read it. A prompt the guardrail refused never
// reached the model, so the sentence it carried was never read: it waits for the
// first prompt that does.
func TestTheResumeNoticeWaitsForAPromptThatReachesTheAgent(t *testing.T) {
	s, _ := newMeteringSession(t, false)
	s.resumeNotice = resumedNotice

	s.guardBlocked = true
	s.promptLanded("a prompt the guardrail refused")
	assert.Equal(t, resumedNotice, s.resumeNotice)

	s.guardBlocked = false
	s.promptLanded("one that went through")
	assert.Empty(t, s.resumeNotice, "said once, to the model")
}
