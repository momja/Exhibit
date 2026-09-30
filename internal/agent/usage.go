package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

// Token metering (av-2yws). This file is the whole of the question "what did
// this session spend" — reading Pi's usage reports off the event stream and
// landing them as rows in the store's ledger. It counts; it refuses nothing
// (av-99f4 refuses).
//
// # What Pi reports, and when (the ticket's first question; full note in
// .tickets/av-2yws.md)
//
//   - `message_update` carries the *cumulative* usage of the assistant
//     response in flight (`usage` block: input/output/cacheRead/cacheWrite
//     tokens and a cost breakdown). It can stay zero until completion for a
//     provider that does not report usage while streaming, so it is folded
//     in per field with max, never summed.
//   - `message_end` carries the authoritative final `usage` on the message —
//     for assistant messages (source: model) and for toolResult messages
//     (source: tool; Pi rolls nested tool calls' usage into the calling
//     tool's result).
//   - `compaction_end` carries `result.usage` (source: compaction).
//   - Some usage emits no event at all (Pi's cache warmer appends straight to
//     its session ledger; extension hook model calls report nothing anywhere
//     — see the guardrail seam on Session.RecordGuardrailUsage). So at every
//     settle the session reconciles against `get_session_stats` — Pi's own
//     cumulative total, which counts assistant messages, usage reported by
//     tools, and compaction — and records whatever the events missed as one
//     tool-sourced row. The ledger therefore equals Pi's total even for
//     usage sources that never reach the wire.
//   - Usage arrives as the spend happens, per model request, so rows land
//     before the process can die; whatever is in flight when it does is
//     flushed in finish(). That is what makes aborted, idle-reaped, and
//     killed-mid-turn sessions attribute what they spent.
//
// cost_micros is Pi's price-table estimate and is not a bill; the token
// columns are the meter.

// Usage is the `usage` block Pi reports (its TypeScript `Usage`).
type Usage struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	CacheRead   int64 `json:"cacheRead"`
	CacheWrite  int64 `json:"cacheWrite"`
	TotalTokens int64 `json:"totalTokens"`
	Cost        struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cacheRead"`
		CacheWrite float64 `json:"cacheWrite"`
		Total      float64 `json:"total"`
	} `json:"cost"`
}

// decodeUsage parses one usage block leniently. Numbers arrive as whatever
// the provider reported and travel through JS serialization — a token count
// can legitimately appear as 100.0 or 1e3 — so they are read as floats and
// rounded. A block that is not an object at all is not a usage block: the
// caller skips the extraction and keeps the line.
func decodeUsage(raw json.RawMessage) (Usage, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return Usage{}, false
	}
	var wire struct {
		Input       json.Number `json:"input"`
		Output      json.Number `json:"output"`
		CacheRead   json.Number `json:"cacheRead"`
		CacheWrite  json.Number `json:"cacheWrite"`
		TotalTokens json.Number `json:"totalTokens"`
		Cost        struct {
			Input      float64 `json:"input"`
			Output     float64 `json:"output"`
			CacheRead  float64 `json:"cacheRead"`
			CacheWrite float64 `json:"cacheWrite"`
			Total      float64 `json:"total"`
		} `json:"cost"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Usage{}, false
	}
	u := Usage{
		Input:       numInt(wire.Input),
		Output:      numInt(wire.Output),
		CacheRead:   numInt(wire.CacheRead),
		CacheWrite:  numInt(wire.CacheWrite),
		TotalTokens: numInt(wire.TotalTokens),
	}
	u.Cost = wire.Cost
	return u, true
}

// numInt rounds one reported count to whole tokens. Malformed and negative
// counts record as zero rather than as a credit.
func numInt(n json.Number) int64 {
	f, err := n.Float64()
	if err != nil || f <= 0 {
		return 0
	}
	return int64(math.Round(f))
}

// costMicros converts Pi's dollar float to integer millionths. Integers so a
// spend cap compares exact sums instead of accumulated float error; micros so
// the conversion loses nothing a cap could hide behind. NaN and negatives are
// refused at zero — a malformed cost must not become a credit.
func costMicros(dollars float64) int64 {
	if math.IsNaN(dollars) || dollars <= 0 {
		return 0
	}
	return int64(math.Round(dollars * 1e6))
}

// max folds a cumulative streaming report into an accumulator. Usage for one
// assistant response is cumulative, so the same event replayed or reported
// late must take the larger, never add.
func (u Usage) max(o Usage) Usage {
	u.Input = max(u.Input, o.Input)
	u.Output = max(u.Output, o.Output)
	u.CacheRead = max(u.CacheRead, o.CacheRead)
	u.CacheWrite = max(u.CacheWrite, o.CacheWrite)
	u.TotalTokens = max(u.TotalTokens, o.TotalTokens)
	if o.Cost.Total > u.Cost.Total {
		u.Cost = o.Cost
	}
	return u
}

func (u Usage) isZero() bool {
	return u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheWrite == 0 &&
		u.Cost.Total == 0
}

func (u Usage) row(s *Session, source string) store.AgentUsage {
	return store.AgentUsage{
		OwnerID:          s.OwnerID,
		SessionID:        s.ID,
		Provider:         s.provider,
		Model:            s.model,
		PaidBy:           s.paidBy,
		Source:           source,
		InputTokens:      u.Input,
		OutputTokens:     u.Output,
		CacheReadTokens:  u.CacheRead,
		CacheWriteTokens: u.CacheWrite,
		CostMicros:       costMicros(u.Cost.Total),
	}
}

// noteStreamingUsage folds one cumulative streaming report into the response
// in flight. It runs on the read loop, so it must not block: the cap check it
// triggers goes off in a goroutine (and the report is also the live input to
// the mid-response check in spendcap.go).
func (s *Session) noteStreamingUsage(u Usage) {
	s.mu.Lock()
	s.usageCur = s.usageCur.max(u)
	s.usageCurActive = true
	s.mu.Unlock()
	// Spend enforcement (av-99f4) checks the caps here.
}

// noteMessageUsage closes out one message that carried usage. Assistant
// messages are model spend and toolResult messages are tool-reported spend;
// the two sources are recorded apart so a per-owner budget can meter one and
// not the other (av-99f4). provider and model come off the message envelope
// when Pi names them — the model that actually answered, which may differ
// from the configured one (a compaction or retry can move it) — falling back
// to the session's configuration.
func (s *Session) noteMessageUsage(role, provider, model string, u Usage) {
	source := store.UsageSourceModel
	if role == "toolResult" {
		source = store.UsageSourceTool
	}
	s.mu.Lock()
	final := s.usageCur.max(u)
	s.usageCur = Usage{}
	s.usageCurActive = false
	s.mu.Unlock()
	if final.isZero() {
		return
	}
	row := final.row(s, source)
	if provider != "" {
		row.Provider = provider
		row.Model = model
	}
	s.recordUsage(row)
}

// noteCompactionUsage records the usage of a compaction summary.
func (s *Session) noteCompactionUsage(u Usage) {
	s.mu.Lock()
	s.usageCur = Usage{}
	s.usageCurActive = false
	s.mu.Unlock()
	if u.isZero() {
		return
	}
	s.recordUsage(u.row(s, store.UsageSourceCompaction))
}

// recordUsage lands one ledger row and folds it into the session's recorded
// total. Synchronous on purpose: the row is the only record of the spend, a
// local insert is small, and an asynchronous one would be exactly the kind of
// work a kill loses. The reconcile below assumes recorded rows are durable the
// moment their events were handled.
func (s *Session) recordUsage(row store.AgentUsage) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.mgr.st.RecordAgentUsage(ctx, row); err != nil {
		// Logged, not fatal, and deliberately not retried into the read
		// loop: the reconcile at the next settle re-records the gap, because
		// it compares against what is durably written rather than what we
		// meant to write.
		slog.Warn("agent usage row failed", slog.String("session_id", s.ID), slog.String("err", err.Error()))
		return
	}
	s.mu.Lock()
	s.usageRecorded.Add(row)
	s.mu.Unlock()
	// Spend enforcement (av-99f4) checks the caps here.
}

// reconcileUsage closes the gap between what the event stream showed and
// Pi's own session totals. get_session_stats counts assistant messages,
// usage reported by tools, and compaction across the whole session; the
// first and third arrive on the wire and are already rowed, so the delta is
// tool-reported usage (and anything else Pi counted that emitted no event,
// such as cache warming). It is recorded as one tool-sourced row so the
// ledger's sum equals Pi's total. Runs at every settle, off the read loop,
// because it makes an RPC round trip.
func (s *Session) reconcileUsage() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := s.roundTrip(ctx, map[string]any{"type": "get_session_stats"})
	if err != nil {
		slog.Warn("usage reconcile skipped", slog.String("session_id", s.ID), slog.String("err", err.Error()))
		return
	}
	var r struct {
		Data struct {
			Tokens struct {
				Input      int64 `json:"input"`
				Output     int64 `json:"output"`
				CacheRead  int64 `json:"cacheRead"`
				CacheWrite int64 `json:"cacheWrite"`
			} `json:"tokens"`
			Cost float64 `json:"cost"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return
	}
	stats := store.UsageTotals{
		InputTokens:      r.Data.Tokens.Input,
		OutputTokens:     r.Data.Tokens.Output,
		CacheReadTokens:  r.Data.Tokens.CacheRead,
		CacheWriteTokens: r.Data.Tokens.CacheWrite,
		CostMicros:       costMicros(r.Data.Cost),
	}

	// The in-flight response is flushed first so its spend is recorded
	// exactly once — the delta computed below is against durable rows, and
	// finish() must not re-record what reconcile already folded in.
	s.flushInFlightUsage()

	s.mu.Lock()
	recorded := s.usageRecorded
	s.mu.Unlock()

	delta := store.UsageTotals{
		InputTokens:      max(0, stats.InputTokens-recorded.InputTokens),
		OutputTokens:     max(0, stats.OutputTokens-recorded.OutputTokens),
		CacheReadTokens:  max(0, stats.CacheReadTokens-recorded.CacheReadTokens),
		CacheWriteTokens: max(0, stats.CacheWriteTokens-recorded.CacheWriteTokens),
		CostMicros:       max(0, stats.CostMicros-recorded.CostMicros),
	}
	if delta == (store.UsageTotals{}) {
		return
	}
	s.recordUsage(store.AgentUsage{
		OwnerID:          s.OwnerID,
		SessionID:        s.ID,
		Provider:         s.provider,
		Model:            s.model,
		PaidBy:           s.paidBy,
		Source:           store.UsageSourceTool,
		InputTokens:      delta.InputTokens,
		OutputTokens:     delta.OutputTokens,
		CacheReadTokens:  delta.CacheReadTokens,
		CacheWriteTokens: delta.CacheWriteTokens,
		CostMicros:       delta.CostMicros,
	})
}

// flushInFlightUsage records whatever the response in flight has reported so
// far. Called when a session settles, and from finish() — a subprocess killed
// mid-response never sends message_end, and that response's usage is exactly
// the spend that produced nothing, which is the spend most worth attributing.
func (s *Session) flushInFlightUsage() {
	s.mu.Lock()
	cur := s.usageCur
	s.usageCur = Usage{}
	s.usageCurActive = false
	s.mu.Unlock()
	if cur.isZero() {
		return
	}
	s.recordUsage(cur.row(s, store.UsageSourceModel))
}

// RecordGuardrailUsage is the seam for av-gust's guardrail screen: its model
// calls are nested calls inside Pi hooks, which Pi reports nowhere — not on
// the event stream and not in get_session_stats (extensions cannot append to
// Pi's usage ledger), so nothing automatic meters them. When guard.ts's
// signal reaches Go, Go calls this and the spend lands tagged `guardrail`:
// operator money even inside a BYO-key session, counted against the instance
// ceiling and never against an owner's budget (av-99f4).
//
// The row's paid_by is forced to platform — the guardrail runs on the
// instance's key by design, whatever key the session itself runs on — so a
// caller cannot book operator spend as somebody's BYO tokens.
func (s *Session) RecordGuardrailUsage(provider, model string, u Usage) {
	row := u.row(s, store.UsageSourceGuardrail)
	row.Provider = provider
	row.Model = model
	row.PaidBy = store.PaidByPlatform
	s.recordUsage(row)
}
