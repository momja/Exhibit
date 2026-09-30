package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMeteringSession is newDetachedSession with a real store behind it and a
// configured provider/model/paidBy, which is what turns handleLine's usage
// events into ledger rows (av-2yws). No subprocess: the tests drive the same
// lines the read loop would.
func newMeteringSession(t *testing.T, platformPaid bool) (*Session, *store.SQLiteStore) {
	t.Helper()
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := newDetachedSession(t, "")
	s.OwnerID = 7
	s.provider = "anthropic"
	s.model = "test-model"
	s.paidBy = paidByFor(platformPaid)
	s.mgr = &Manager{st: db}
	// No subprocess behind this one; a cap that stops it aborts into this
	// pipe and gets an error back, exactly as a dead subprocess would.
	s.stdin = deadPipe{}
	return s, db
}

// deadPipe stands in for a subprocess that is not there.
type deadPipe struct{}

func (deadPipe) Write([]byte) (int, error) { return 0, errors.New("no subprocess in this test") }
func (deadPipe) Close() error              { return nil }

func sessionTotals(t *testing.T, db *store.SQLiteStore, s *Session) store.UsageTotals {
	t.Helper()
	totals, err := db.SessionAgentSpend(context.Background(), s.OwnerID, s.ID)
	require.NoError(t, err)
	return totals
}

func ownerTotals(t *testing.T, db *store.SQLiteStore) store.UsageTotals {
	t.Helper()
	totals, err := db.OwnerAgentSpend(context.Background(), 7, time.Time{})
	require.NoError(t, err)
	return totals
}

// One assistant response, reported cumulatively while streaming and finalized
// on message_end: the row is the final numbers exactly once. Summing the
// streaming reports would record the response two and three times over — the
// failure max-folding exists to prevent.
func TestAssistantMessageUsageIsRecordedOnce(t *testing.T) {
	s, db := newMeteringSession(t, true)

	feed(t, s, `{"type":"message_update","usage":{"input":100,"output":10,"cacheRead":0,"cacheWrite":0,`+
		`"totalTokens":110,"cost":{"total":0.005}},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"a"}}`)
	feed(t, s, `{"type":"message_update","usage":{"input":100,"output":25,"cacheRead":0,"cacheWrite":0,`+
		`"totalTokens":125,"cost":{"total":0.01}},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"b"}}`)
	feed(t, s, `{"type":"message_end","message":{"role":"assistant","provider":"anthropic","model":"test-model",`+
		`"usage":{"input":100,"output":30,"cacheRead":0,"cacheWrite":0,"totalTokens":130,"cost":{"total":0.015}}}}`)

	totals := sessionTotals(t, db, s)
	assert.Equal(t, store.UsageTotals{InputTokens: 100, OutputTokens: 30, CostMicros: 15_000}, totals,
		"the response is recorded once, at its final numbers — not once per streaming report")
	assert.Equal(t, totals, ownerTotals(t, db), "the owner meter sums the same rows")
}

// Tool-reported usage rides toolResult messages (nested calls fold into their
// caller) and is recorded as tool spend, kept apart from model spend.
func TestToolResultUsageIsRecordedAsToolSpend(t *testing.T) {
	s, db := newMeteringSession(t, true)

	feed(t, s, `{"type":"message_end","message":{"role":"toolResult",`+
		`"usage":{"input":50,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":50,"cost":{"total":0.002}}}}`)

	assert.Equal(t, int64(50), ownerTotals(t, db).InputTokens,
		"tool-reported usage is spend the owner's budget meters")
}

// A compaction summary is its own recorded source.
func TestCompactionUsageIsRecorded(t *testing.T) {
	s, db := newMeteringSession(t, true)

	feed(t, s, `{"type":"compaction_end","reason":"threshold","result":{`+
		`"usage":{"input":1000,"output":200,"cacheRead":0,"cacheWrite":0,"totalTokens":1200,"cost":{"total":0.02}}}}`)

	totals := sessionTotals(t, db, s)
	assert.Equal(t, store.UsageTotals{InputTokens: 1000, OutputTokens: 200, CostMicros: 20_000}, totals)
}

// A response the subprocess never finished reporting — the abort, the idle
// kill, the mid-turn death — is flushed when the process exits. Those are the
// sessions that spent money and produced nothing, and they are the whole
// reason the flush exists.
func TestInFlightUsageSurvivesAKilledResponse(t *testing.T) {
	s, db := newMeteringSession(t, true)

	feed(t, s, `{"type":"message_update","usage":{"input":200,"output":5,"cacheRead":0,"cacheWrite":0,`+
		`"totalTokens":205,"cost":{"total":0.008}},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"a"}}`)
	s.flushInFlightUsage() // what finish() runs after the subprocess dies

	totals := sessionTotals(t, db, s)
	assert.Equal(t, store.UsageTotals{InputTokens: 200, OutputTokens: 5, CostMicros: 8_000}, totals)
}

// BYO-key rows are recorded too — a self-hosted instance gets the number it
// lacked — and are distinguishable: they are the owner's own money, so they
// sit outside both the owner budget meter and the operator's exposure.
func TestBYOKUsageIsRecordedButMeteredAsTheUsersOwn(t *testing.T) {
	s, db := newMeteringSession(t, false)

	feed(t, s, `{"type":"message_end","message":{"role":"assistant","provider":"anthropic","model":"test-model",`+
		`"usage":{"input":300,"output":40,"cacheRead":0,"cacheWrite":0,"totalTokens":340,"cost":{"total":0.05}}}}`)

	assert.Equal(t, store.UsageTotals{InputTokens: 300, OutputTokens: 40, CostMicros: 50_000},
		sessionTotals(t, db, s), "the ledger records BYO-key spend")

	assert.Equal(t, int64(0), ownerTotals(t, db).CostMicros,
		"a user spending their own tokens is not metered against any operator budget")
	instance, err := db.InstanceAgentSpend(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), instance.CostMicros)
}

// The guardrail seam (av-gust): its nested model calls report to no Pi hook
// and no session total, so their spend lands here tagged guardrail — operator
// money even inside a BYO-key session, and forced to say so regardless of
// what the caller claims about who pays.
func TestGuardrailUsageIsOperatorSpendEvenInABYOKSession(t *testing.T) {
	s, db := newMeteringSession(t, false)

	var guardUsage Usage
	guardUsage.Input = 10
	guardUsage.Output = 2
	guardUsage.Cost.Total = 0.001
	s.RecordGuardrailUsage("exhibit-guard", "guard-model", guardUsage)

	assert.Equal(t, int64(0), ownerTotals(t, db).CostMicros,
		"guardrail overhead is not the owner's budget problem")
	instance, err := db.InstanceAgentSpend(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, int64(1_000), instance.CostMicros,
		"but it is the operator's, whatever key the session runs on")
}

// costMicros refuses to invent credit: a malformed or negative price records
// zero rather than a negative the sums would treat as a refund.
func TestCostMicrosNeverGoesNegative(t *testing.T) {
	assert.Equal(t, int64(15_000), costMicros(0.015))
	assert.Equal(t, int64(0), costMicros(0))
	assert.Equal(t, int64(0), costMicros(-1))
}
