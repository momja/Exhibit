package agent

import (
	"context"
	"encoding/json"
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

// The metering probe must keep every line it fails to understand. Guard
// signals ride extension_ui_request notifications whose "message" is a
// *string*, where message_end's is an object — a typed probe field once made
// the whole-line unmarshal fail, silently dropping every guard signal before
// guardSignalOf saw them, so nothing ever blocked. Whatever is not
// understood costs the extraction, never the line.
func TestTheMeteringProbeKeepsLinesItCannotFullyParse(t *testing.T) {
	s, db := newMeteringSession(t, false)

	events := feed(t, s, `{"type":"extension_ui_request","id":"1","method":"notify",`+
		`"message":"exhibit_guard:usage {\"provider\":\"p\",\"model\":\"m\",`+
		`\"usage\":{\"input\":10,\"output\":2,\"cost\":{\"total\":0.001}}}","notifyType":"info"}`)
	assert.Empty(t, events, "a usage signal is metered, never forwarded")
	instance, err := db.InstanceAgentSpend(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, store.UsageTotals{InputTokens: 10, OutputTokens: 2, CostMicros: 1_000}, instance,
		"the usage signal inside the notification is still metered")

	events = feed(t, s, `{"type":"extension_ui_request","id":"2","method":"notify",`+
		`"message":"exhibit_guard:blocked p(violates)=0.99","notifyType":"warning"}`)
	require.NotNil(t, find(events, "exhibit_guard_blocked"),
		"a block signal must survive the probe — this is what stops the prompt")
}

// Reported counts travel through JS serialization and can arrive as 100.0 or
// 1e3. They are tokens all the same.
func TestUsageBlocksTolerateNumericShapes(t *testing.T) {
	s, db := newMeteringSession(t, true)

	feed(t, s, `{"type":"message_update","usage":{"input":100.0,"output":1e3,"cacheRead":0,"cacheWrite":0,`+
		`"totalTokens":1100,"cost":{"total":0.5}},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"a"}}`)
	s.flushInFlightUsage()

	totals := sessionTotals(t, db, s)
	assert.Equal(t, int64(100), totals.InputTokens)
	assert.Equal(t, int64(1000), totals.OutputTokens)
}

// statsResponder stands in for a subprocess that answers get_session_stats.
// Each request is handed to respond, which feeds the session whatever lines
// the test wants the read loop to see around the answer.
type statsResponder struct {
	s       *Session
	respond func(id string)
}

func (p statsResponder) Write(b []byte) (int, error) {
	var cmd struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &cmd); err != nil {
		return 0, err
	}
	go p.respond(cmd.ID)
	return len(b), nil
}
func (statsResponder) Close() error { return nil }

func statsLine(id string, input, output int64, cost float64) string {
	b, _ := json.Marshal(map[string]any{
		"type": "response", "id": id, "command": "get_session_stats", "success": true,
		"data": map[string]any{
			"tokens": map[string]any{"input": input, "output": output, "cacheRead": 0, "cacheWrite": 0},
			"cost":   cost,
		},
	})
	return string(b)
}

// Guardrail rows are operator spend Pi never counts, so they must not be
// subtracted from Pi's totals: a screen that cost as much as the tool usage
// the settle is looking for would otherwise hide that tool usage entirely.
func TestGuardrailSpendDoesNotHideTheReconcileGap(t *testing.T) {
	s, db := newMeteringSession(t, true)
	s.stdin = statsResponder{s: s, respond: func(id string) {
		// Pi's total: the assistant message (100/10) plus tool usage (50/0).
		s.handleLine([]byte(statsLine(id, 150, 10, 0.02)))
	}}

	var guardUsage Usage
	guardUsage.Input = 50
	guardUsage.Cost.Total = 0.005
	s.RecordGuardrailUsage("exhibit-guard", "guard-model", guardUsage)
	feed(t, s, `{"type":"message_end","message":{"role":"assistant",`+
		`"usage":{"input":100,"output":10,"cacheRead":0,"cacheWrite":0,"totalTokens":110,"cost":{"total":0.01}}}}`)

	s.reconcileUsage()

	assert.Equal(t, store.UsageTotals{InputTokens: 150, OutputTokens: 10, CostMicros: 20_000},
		sessionTotals(t, db, s), "the model row plus the whole 50-token tool gap")
	instance, err := db.InstanceAgentSpend(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.Equal(t, store.UsageTotals{InputTokens: 200, OutputTokens: 10, CostMicros: 25_000}, instance,
		"and the guardrail's own 50 on top, at the instance level")
}

// The stats snapshot is compared against what was recorded when the read loop
// reached it. A response that finishes after Pi answered is not in Pi's
// numbers, so counting it against them would suppress the gap the snapshot
// actually shows.
func TestReconcileIgnoresRowsRecordedAfterTheSnapshot(t *testing.T) {
	s, db := newMeteringSession(t, true)
	s.stdin = statsResponder{s: s, respond: func(id string) {
		// Pi's total at answer time: one assistant message (100/10) plus 40
		// tokens of tool usage no event reported.
		s.handleLine([]byte(statsLine(id, 140, 10, 0.012)))
		// A steered follow-up response lands before the reconcile goroutine
		// reads its result.
		s.handleLine([]byte(`{"type":"message_end","message":{"role":"assistant",` +
			`"usage":{"input":500,"output":50,"cacheRead":0,"cacheWrite":0,"totalTokens":550,"cost":{"total":0.05}}}}`))
	}}

	feed(t, s, `{"type":"message_end","message":{"role":"assistant",`+
		`"usage":{"input":100,"output":10,"cacheRead":0,"cacheWrite":0,"totalTokens":110,"cost":{"total":0.01}}}}`)

	s.reconcileUsage()

	assert.Equal(t, store.UsageTotals{InputTokens: 640, OutputTokens: 60, CostMicros: 62_000},
		sessionTotals(t, db, s),
		"both responses once, plus the 40-token gap the snapshot showed")
}

// The settle flushes the response in flight on the read loop, before the
// reconcile goroutine starts, so a later response's partial numbers can never
// be flushed as this turn's and then recorded again at its own message_end.
func TestSettleFlushesOnlyTheSettledTurn(t *testing.T) {
	s, db := newMeteringSession(t, true)
	answered := make(chan string, 1)
	s.stdin = statsResponder{s: s, respond: func(id string) { answered <- id }}

	feed(t, s, `{"type":"message_update","usage":{"input":30,"output":3,"cacheRead":0,"cacheWrite":0,`+
		`"totalTokens":33,"cost":{"total":0.003}},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"a"}}`)
	feed(t, s, `{"type":"agent_settled"}`)
	id := <-answered

	// A new turn streams and completes while the reconcile is still waiting.
	feed(t, s, `{"type":"message_update","usage":{"input":200,"output":20,"cacheRead":0,"cacheWrite":0,`+
		`"totalTokens":220,"cost":{"total":0.02}},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"b"}}`)
	feed(t, s, `{"type":"message_end","message":{"role":"assistant",`+
		`"usage":{"input":200,"output":20,"cacheRead":0,"cacheWrite":0,"totalTokens":220,"cost":{"total":0.02}}}}`)
	s.handleLine([]byte(statsLine(id, 30, 3, 0.003)))

	// The reconcile holds reconcileMu until its gap row (if any) has landed.
	s.reconcileMu.Lock()
	s.reconcileMu.Unlock() //nolint:staticcheck // waiting for the reconcile, not guarding anything
	assert.Equal(t, store.UsageTotals{InputTokens: 230, OutputTokens: 23, CostMicros: 23_000},
		sessionTotals(t, db, s), "each response recorded exactly once")
}
