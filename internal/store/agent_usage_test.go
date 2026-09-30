package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usageRow builds one ledger event with distinguishable token counts and a
// cost in micros, so a sum that double-counts or drops a filter is visible in
// every column at once.
func usageRow(owner int64, session, paidBy, source string, input, output, cost int64) AgentUsage {
	return AgentUsage{
		OwnerID:      owner,
		SessionID:    session,
		Provider:     "anthropic",
		Model:        "test-model",
		PaidBy:       paidBy,
		Source:       source,
		InputTokens:  input,
		OutputTokens: output,
		CostMicros:   cost,
	}
}

// The owner budget meter (av-99f4) counts one owner's platform-paid agent
// spend and nothing else: BYO-key rows are the owner's own money, guardrail
// rows are the operator's overhead (backstopped by the instance ceiling),
// and another owner's spend is never this owner's.
func TestOwnerAgentSpendMetersOnlyThisOwnersPlatformAgentSpend(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rows := []AgentUsage{
		usageRow(7, "s1", PaidByPlatform, UsageSourceModel, 100, 30, 600_000),
		usageRow(7, "s1", PaidByPlatform, UsageSourceTool, 10, 5, 100_000),
		usageRow(7, "s1", PaidByPlatform, UsageSourceGuardrail, 20, 10, 300_000),
		usageRow(7, "s1", PaidByUser, UsageSourceModel, 900, 900, 9_000_000),
		usageRow(8, "s2", PaidByPlatform, UsageSourceModel, 500, 500, 7_000_000),
	}
	for _, r := range rows {
		require.NoError(t, s.RecordAgentUsage(ctx, r))
	}

	totals, err := s.OwnerAgentSpend(ctx, 7, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, UsageTotals{
		InputTokens: 110, OutputTokens: 35, CostMicros: 700_000,
	}, totals, "owner 7's budget meter: platform agent spend only — no guardrail, no BYO-key, no other owner")
}

// The period bound is the budget's reset semantics: a monthly budget sums
// this month, not the account's lifetime.
func TestOwnerAgentSpendIsBoundedToThePeriod(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(7, "s1", PaidByPlatform, UsageSourceModel, 1, 1, 400_000)))

	later := time.Now().UTC().Add(time.Hour)
	totals, err := s.OwnerAgentSpend(ctx, 7, later)
	require.NoError(t, err)
	assert.Equal(t, int64(0), totals.CostMicros, "a period that starts after the spend sees none of it")

	totals, err = s.OwnerAgentSpend(ctx, 7, time.Now().UTC().Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(400_000), totals.CostMicros)
}

// The per-session ceiling meters what the conversation cost the operator —
// guardrail screening included — and is owner-scoped like every sum (av-ep8k):
// another owner's session id is not a window onto this one.
func TestSessionAgentSpendCountsTheWholeConversation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(7, "s1", PaidByPlatform, UsageSourceModel, 10, 10, 500_000)))
	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(7, "s1", PaidByPlatform, UsageSourceGuardrail, 5, 5, 200_000)))
	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(7, "s-other", PaidByPlatform, UsageSourceModel, 1, 1, 900_000)))

	totals, err := s.SessionAgentSpend(ctx, 7, "s1")
	require.NoError(t, err)
	assert.Equal(t, int64(700_000), totals.CostMicros, "guardrail spend is part of what the conversation cost")

	totals, err = s.SessionAgentSpend(ctx, 8, "s1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), totals.CostMicros, "the sum is owner-scoped (av-ep8k)")
}

// The instance ceiling is the operator's exposure: every platform-paid row,
// guardrail screening of BYO-key sessions very much included — that is
// operator money spent inside a conversation the operator does not otherwise
// pay for.
func TestInstanceAgentSpendCountsAllPlatformSpendIncludingGuardrail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(7, "s1", PaidByPlatform, UsageSourceModel, 10, 10, 500_000)))
	// A BYO-key session: its model spend is the user's, its guardrail spend
	// is still the operator's.
	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(8, "s2", PaidByUser, UsageSourceModel, 10, 10, 4_000_000)))
	require.NoError(t, s.RecordAgentUsage(ctx, usageRow(8, "s2", PaidByPlatform, UsageSourceGuardrail, 3, 3, 250_000)))

	totals, err := s.InstanceAgentSpend(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, int64(750_000), totals.CostMicros,
		"everything the operator paid for: platform sessions and guardrail screening, never a BYO-key user's own tokens")
}

// Input and output are the meter and are recorded separately (the ticket, by
// name) — they price differently, and a total without the split cannot become
// money.
func TestUsageTokensAreRecordedSeparately(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.RecordAgentUsage(ctx, AgentUsage{
		OwnerID: 7, SessionID: "s1", Provider: "anthropic", Model: "m",
		PaidBy: PaidByPlatform, Source: UsageSourceModel,
		InputTokens: 111, OutputTokens: 222, CacheReadTokens: 333, CacheWriteTokens: 444, CostMicros: 5,
	}))

	totals, err := s.SessionAgentSpend(ctx, 7, "s1")
	require.NoError(t, err)
	assert.Equal(t, UsageTotals{
		InputTokens: 111, OutputTokens: 222, CacheReadTokens: 333, CacheWriteTokens: 444, CostMicros: 5,
	}, totals)
}
