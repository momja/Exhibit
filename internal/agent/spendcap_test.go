package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func microsPtr(v int64) *int64 { return &v }

func seedSpend(t *testing.T, db *store.SQLiteStore, owner int64, session, paidBy, source string, costMicros int64) {
	t.Helper()
	require.NoError(t, db.RecordAgentUsage(context.Background(), store.AgentUsage{
		OwnerID: owner, SessionID: session, Provider: "anthropic", Model: "m",
		PaidBy: paidBy, Source: source, InputTokens: 1, OutputTokens: 1, CostMicros: costMicros,
	}))
}

// The configured value is cents; the world is micros. Absence, zero, and a
// number are three different things — an explicit zero budget refuses
// everything, and an unset limit refuses nothing.
func TestSpendCapsFromEnv(t *testing.T) {
	t.Setenv("AGENT_SPEND_CAP_OWNER_CENTS", "")
	t.Setenv("AGENT_SPEND_CAP_SESSION_CENTS", "")
	t.Setenv("AGENT_SPEND_CAP_INSTANCE_CENTS", "")
	t.Setenv("AGENT_SPEND_CAP_TURN_SECONDS", "")

	caps, err := SpendCapsFromEnv()
	require.NoError(t, err)
	assert.False(t, caps.AnySet(), "absent configuration means no limit (and no enforcement)")

	t.Setenv("AGENT_SPEND_CAP_OWNER_CENTS", "2000") // $20
	caps, err = SpendCapsFromEnv()
	require.NoError(t, err)
	require.NotNil(t, caps.OwnerMicrosPerMonth)
	assert.Equal(t, int64(20_000_000), *caps.OwnerMicrosPerMonth)
	assert.Nil(t, caps.SessionMicros)
	assert.True(t, caps.AnySet())

	t.Setenv("AGENT_SPEND_CAP_OWNER_CENTS", "0")
	caps, err = SpendCapsFromEnv()
	require.NoError(t, err)
	assert.True(t, caps.AnySet(), "a budget of zero is a limit, not absence")

	t.Setenv("AGENT_SPEND_CAP_OWNER_CENTS", "twenty")
	_, err = SpendCapsFromEnv()
	assert.Error(t, err, "a malformed limit fails at startup rather than booting a guess")
}

// Refusal wording is the ticket's legibility requirement by another name: it
// names the limit and when it resets, and never claims a budget was reached
// when the check itself failed.
func TestCapRefusalMessagesNameTheLimitAndItsReset(t *testing.T) {
	reset := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	owner := &CapRefusal{Which: "owner", LimitMicros: 20_000_000, UsedMicros: 20_100_000, ResetAt: reset}
	assert.Contains(t, owner.Message(), "$20.00")
	assert.Contains(t, owner.Message(), "resets on 2026-10-01 00:00 UTC")

	session := &CapRefusal{Which: "session", LimitMicros: 1_000_000, UsedMicros: 1_200_000}
	assert.Contains(t, session.Message(), "Start a new conversation")

	instance := &CapRefusal{Which: "instance", LimitMicros: 100_000_000, UsedMicros: 101_000_000, ResetAt: reset}
	assert.Contains(t, instance.Message(), "New agent sessions are refused")

	unresolved := &CapRefusal{Which: "unresolved"}
	assert.Contains(t, unresolved.Message(), "could not be checked")
	assert.NotContains(t, unresolved.Message(), "budget reached")
}

// The budget periods are calendar months in UTC, which is exactly what the
// refusal message promises when it names a reset.
func TestBudgetsResetOnCalendarMonthsUTC(t *testing.T) {
	now := time.Date(2026, 9, 29, 23, 12, 0, 0, time.FixedZone("pdt", -7*3600))
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), monthStart(now))
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), nextMonth(now))
}

// The pre-spawn check: an owner over their budget is refused before any
// subprocess exists (av-99f4's acceptance, by name), with the message naming
// the limit and its reset. Under budget, silence.
func TestProbeCapsRefusesAnOwnerOverTheBudget(t *testing.T) {
	s, db := newMeteringSession(t, true)
	s.mgr.cfg.Caps = SpendCaps{OwnerMicrosPerMonth: microsPtr(1_000_000)} // $1
	seedSpend(t, db, s.OwnerID, "s1", store.PaidByPlatform, store.UsageSourceModel, 600_000)
	seedSpend(t, db, s.OwnerID, "s1", store.PaidByPlatform, store.UsageSourceModel, 600_000)

	refusal := s.mgr.ProbeCaps(context.Background(), s.OwnerID)
	require.NotNil(t, refusal, "an owner over their monthly budget is refused before spawn")
	assert.Equal(t, "owner", refusal.Which)
	assert.Equal(t, int64(1_000_000), refusal.LimitMicros)
	assert.Equal(t, int64(1_200_000), refusal.UsedMicros)
	assert.False(t, refusal.ResetAt.IsZero(), "the message must be able to say when the budget resets")
	assert.Contains(t, refusal.Message(), "monthly agent budget")

	under := s.mgr.ProbeCaps(context.Background(), 8)
	assert.Nil(t, under, "an owner with no spend is not refused")
}

// The per-session ceiling stops a looping conversation on its own, without
// waiting for the owner's month to drain — and the whole conversation's cost,
// guardrail screening included, is what it meters.
func TestSessionCeilingStopsALoopingConversation(t *testing.T) {
	s, db := newMeteringSession(t, true)
	s.mgr.cfg.Caps = SpendCaps{SessionMicros: microsPtr(1_000_000)}
	seedSpend(t, db, s.OwnerID, s.ID, store.PaidByPlatform, store.UsageSourceModel, 800_000)

	assert.NoError(t, s.capBeforePrompt(), "under the ceiling, a running conversation continues")

	// The loop keeps going — model spend plus its guardrail screening — and
	// the ceiling stops it on its own, without waiting for the owner's month.
	seedSpend(t, db, s.OwnerID, s.ID, store.PaidByPlatform, store.UsageSourceGuardrail, 300_000)
	refusal := s.mgr.probeCaps(context.Background(), s.OwnerID, s.ID)
	require.NotNil(t, refusal, "a looping conversation is stopped at the session ceiling")
	assert.Equal(t, "session", refusal.Which)
	assert.Contains(t, refusal.Message(), "Start a new conversation")
}

// The instance ceiling is the operator's exposure: guardrail spend counts
// toward it even from a BYO-key session, because it is operator money.
func TestInstanceCeilingCountsGuardrailSpend(t *testing.T) {
	s, db := newMeteringSession(t, true)
	s.mgr.cfg.Caps = SpendCaps{InstanceMicrosPerMonth: microsPtr(1_000_000)}
	seedSpend(t, db, 8, "byok-session", store.PaidByUser, store.UsageSourceModel, 50_000_000)
	seedSpend(t, db, 8, "byok-session", store.PaidByPlatform, store.UsageSourceGuardrail, 1_100_000)

	refusal := s.mgr.ProbeCaps(context.Background(), s.OwnerID)
	require.NotNil(t, refusal, "the backstop protects the operator whatever session spent the money")
	assert.Equal(t, "instance", refusal.Which)
	assert.Contains(t, refusal.Message(), "resets on")
}

// A BYO-key session is never limited by the operator's defaults, on any
// instance — its owner may be wildly over a budget that does not govern them,
// and the answer is silence.
func TestBYOKSessionsAreNeverLimited(t *testing.T) {
	s, db := newMeteringSession(t, false)
	s.mgr.cfg.Caps = SpendCaps{
		OwnerMicrosPerMonth:    microsPtr(1),
		SessionMicros:          microsPtr(1),
		InstanceMicrosPerMonth: microsPtr(1),
		TurnSeconds:            1,
	}
	seedSpend(t, db, s.OwnerID, s.ID, store.PaidByPlatform, store.UsageSourceGuardrail, 99_000_000)

	assert.NoError(t, s.capBeforePrompt(), "a user spending their own tokens is not throttled")
}

// A session a cap stopped stays stopped, and says so in one voice: the canned
// notice on the stream for the run that was stopped, the same message from
// every later prompt, and no second run aborted.
func TestStopForCapSticksAndNotices(t *testing.T) {
	s, _ := newMeteringSession(t, true)
	s.streaming = true

	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	refusal := &CapRefusal{Which: "owner", LimitMicros: 100, UsedMicros: 150,
		ResetAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	s.stopForCap(refusal)
	s.stopForCap(&CapRefusal{Which: "session"}) // the second cap does not re-notice

	var notices int
	for {
		select {
		case raw := <-events:
			if strings.Contains(string(raw), "exhibit_spend_cap") {
				notices++
				assert.Contains(t, string(raw), "monthly agent budget")
			}
		default:
			goto done
		}
	}
done:
	assert.Equal(t, 1, notices, "exactly one canned notice")

	err := s.Prompt(context.Background(), "keep going", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, refusal, "every later prompt fails with the same refusal, not a generic error")
}

// Enforcement holds with every external service unreachable — and the failure
// mode when the *check* cannot run is a refusal, not an allowance. "The
// database is unreachable" is not evidence about what somebody may spend.
func TestAnUnresolvedCapCheckRefusesRatherThanAllows(t *testing.T) {
	db, err := store.OpenSQLite(t.TempDir() + "/x.db")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	m := &Manager{st: failingSpendStore{Store: db}, cfg: Config{Caps: SpendCaps{OwnerMicrosPerMonth: microsPtr(1_000)}}}
	refusal := m.ProbeCaps(context.Background(), 7)
	require.NotNil(t, refusal, "a check that cannot answer refuses")
	assert.Equal(t, "unresolved", refusal.Which)
}

type failingSpendStore struct{ store.Store }

func (failingSpendStore) OwnerAgentSpend(context.Context, int64, time.Time) (store.UsageTotals, error) {
	return store.UsageTotals{}, errors.New("database unreachable")
}
