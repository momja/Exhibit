package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

// Per-owner agent spend cap (av-99f4): refuse before the spend, not after.
//
// This file owns the limits and the question "may this session spend?".
// metering (usage.go) owns the answer to "what was spent"; enforcement is
// layered here because the three limits fail differently:
//
//   - Owner budget — a per-owner ceiling per calendar month (UTC), the
//     product-level limit a plan will map onto (av-2p8z holds the plan data;
//     per-owner overrides land there, and this reads a configured default
//     until they do). Enforced between runs: before a session spawns and
//     before every prompt.
//   - Session ceiling — one conversation's lifetime, catching a loop without
//     waiting for the month to drain. Also enforced between runs.
//   - Instance ceiling — the operator's total exposure per calendar month,
//     guardrail screening included, and the only layer that bounds a failure
//     nobody predicted. Logged loudly when reached. The one budget layer
//     that will also stop a run in progress (below).
//
// All three meter platform-paid spend only. A BYO-key session spends its
// owner's own tokens and is never limited by the operator's defaults, on any
// instance — so enforcement simply does not run for one (see
// Session.capBeforePrompt).
//
// # Graceful stops (product decision, 2026-09-30)
//
// Sessions finish what they started. A run in progress when the owner's
// budget or the session's ceiling crosses is *not* cut off: it runs to
// settle, and the next prompt is refused with the message naming the limit
// and its reset. Nothing is ever stopped mid-thought over a budget.
//
// Two hard stops remain, as failure backstops rather than budgets: the
// per-turn wall-clock ceiling (AGENT_SPEND_CAP_TURN_SECONDS) and the
// instance ceiling — the operator's own money running out is the one thing
// that cannot wait for anybody's turn to end.
//
// # Enforcement granularity: what the protocol actually permits
//
// `before_provider_request` cannot cancel a request — it may replace the
// payload, nothing more — and enforcement between runs is the policy anyway,
// so the honest bound (docs/agent.md) is: **at most one full run per open
// session spends past the budget, so at most 10 runs — one per session a
// single owner can hold.** A run is one prompt through settle, including
// every tool call inside it.
//
// # Fail closed, fail nowhere else
//
// Absence of the feature is unlimited: no caps configured means no
// enforcement, today's behaviour. Absence of a limit the feature requires is
// a refusal to boot: a platform credential with no cap at all fails at
// startup (cmd/server/main.go), because the resource being spent is money
// and the failure is unbounded. Enforcement holds with every external
// service unreachable — the sums are local rows and the limits are local
// config.

const (
	envCapOwnerMicros    = "AGENT_SPEND_CAP_OWNER_CENTS"
	envCapSessionMicros  = "AGENT_SPEND_CAP_SESSION_CENTS"
	envCapInstanceMicros = "AGENT_SPEND_CAP_INSTANCE_CENTS"
	envCapTurnSeconds    = "AGENT_SPEND_CAP_TURN_SECONDS"
)

// SpendCaps is the operator's spend-ceiling configuration. A nil pointer
// field is a layer not in use — the distinction that keeps "no limit
// configured" from meaning "zero". The zero value enforces nothing.
type SpendCaps struct {
	// OwnerMicrosPerMonth is the per-owner budget (calendar month, UTC).
	OwnerMicrosPerMonth *int64
	// SessionMicros is the per-session ceiling (one session's lifetime).
	SessionMicros *int64
	// InstanceMicrosPerMonth is the operator's total exposure per calendar
	// month, guardrail screening included.
	InstanceMicrosPerMonth *int64
	// TurnSeconds is the wall-clock ceiling per turn — the fallback that
	// bounds one pathological response without needing token counts.
	// Zero is off.
	TurnSeconds int64
}

// SpendCapsFromEnv reads the configuration, returning an error the caller
// makes fatal. Values are integer cents — legible at a glance where micros
// would be written as twenty million — and converted once here.
//
// An unparseable value is an error rather than a limit quietly fixed up,
// on the reasoning a malformed AGENT_PROVIDER already takes: an instance
// that boots looking configured and enforces something other than what the
// operator wrote is worse than one that does not boot.
func SpendCapsFromEnv() (SpendCaps, error) {
	var caps SpendCaps
	owner, err := capLimitFromEnv(envCapOwnerMicros)
	if err != nil {
		return caps, err
	}
	caps.OwnerMicrosPerMonth = owner
	session, err := capLimitFromEnv(envCapSessionMicros)
	if err != nil {
		return caps, err
	}
	caps.SessionMicros = session
	instance, err := capLimitFromEnv(envCapInstanceMicros)
	if err != nil {
		return caps, err
	}
	caps.InstanceMicrosPerMonth = instance

	raw := strings.TrimSpace(os.Getenv(envCapTurnSeconds))
	if raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return caps, fmt.Errorf("%s must be a non-negative number of seconds, got %q", envCapTurnSeconds, raw)
		}
		caps.TurnSeconds = n
	}
	return caps, nil
}

// capLimitFromEnv reads one limit in cents. Unset is nil ("this layer is not
// in use"); a number is cents in micros. Zero is a real limit — an operator
// who sets a budget of zero wants spend refused — so it is never confused
// with absence.
func capLimitFromEnv(name string) (*int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, nil
	}
	cents, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cents < 0 {
		return nil, fmt.Errorf("%s must be a non-negative number of cents, got %q", name, raw)
	}
	micros := cents * 10_000 // 1 cent = 10^4 micros
	return &micros, nil
}

// AnySet reports whether at least one ceiling is configured. Platform mode
// requires one: a platform credential with no cap at all must not boot.
func (c SpendCaps) AnySet() bool {
	return c.OwnerMicrosPerMonth != nil || c.SessionMicros != nil ||
		c.InstanceMicrosPerMonth != nil || c.TurnSeconds > 0
}

// monthStart is the first instant of t's calendar month in UTC — the period
// boundary every budget resets on. Calendar months, matching how the refusal
// message promises a reset ("resets on the 1st"), rather than a rolling
// window nobody can predict.
func monthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// nextMonth is when the current period's budgets reset.
func nextMonth(t time.Time) time.Time { return monthStart(t).AddDate(0, 1, 0) }

// CapRefusal is a spend cap saying no. It is a typed error so the API can
// answer with its message (the ticket's "clear message about a limit and when
// it resets" — never a stalled stream or a generic error) and so a stopped
// session can keep saying it for every later prompt.
type CapRefusal struct {
	// Which is the layer that refused: "owner", "session", "instance",
	// "turn" (the wall-clock fallback), or "unresolved" (the check itself
	// failed — see probeCaps).
	Which string
	// LimitMicros and UsedMicros are the ceiling and the spend behind it.
	LimitMicros int64
	UsedMicros  int64
	// Seconds is the "turn" refusal's ceiling, which is wall-clock, not
	// money.
	Seconds int64
	// ResetAt is when the budget resets; zero for the session ceiling (a
	// session never resets — start a new one).
	ResetAt time.Time
}

func (e *CapRefusal) Error() string { return e.Message() }

// Message is the canned wording, shared by the pre-spawn refusal (an HTTP
// error body) and the stopped-run notice (a chat line) so the two cannot
// tell a user different stories.
func (e *CapRefusal) Message() string {
	limit, used := formatCents(e.LimitMicros), formatCents(e.UsedMicros)
	switch e.Which {
	case "owner":
		return fmt.Sprintf("Agent budget reached: %s of your %s monthly agent budget is used. "+
			"It resets on %s. Your message was not sent.", used, limit, formatReset(e.ResetAt))
	case "instance":
		return fmt.Sprintf("The instance's monthly agent budget is exhausted (%s of %s used); "+
			"agent runs stop until it resets on %s.", used, limit, formatReset(e.ResetAt))
	case "session":
		return fmt.Sprintf("This conversation reached its spend ceiling (%s of %s). "+
			"Start a new conversation to keep going.", used, limit)
	case "turn":
		return fmt.Sprintf("A single agent turn ran past the %d-second wall-clock ceiling and was stopped. "+
			"Try a smaller request.", e.Seconds)
	default:
		// "unresolved": the check itself failed, which is not evidence about
		// any budget — say so rather than claiming a limit was reached.
		return "The agent spend limit could not be checked, so the request was refused. Try again later."
	}
}

func formatCents(micros int64) string {
	cents := (micros + 5_000) / 10_000
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func formatReset(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// probeCaps asks every configured layer whether a platform-paid session may
// spend. sessionID is "" before spawn. It returns the first refusal; nil is
// the all-clear. Every sum is a local row count — enforcement holds with
// every external service unreachable (the ticket, by name).
func (m *Manager) probeCaps(ctx context.Context, ownerID int64, sessionID string) *CapRefusal {
	caps := m.cfg.Caps
	now := time.Now()
	since := monthStart(now)
	reset := nextMonth(now)

	if caps.OwnerMicrosPerMonth != nil {
		totals, err := m.st.OwnerAgentSpend(ctx, ownerID, since)
		if err != nil {
			return m.unresolvedCaps(ctx, ownerID, err)
		}
		if totals.CostMicros >= *caps.OwnerMicrosPerMonth {
			return &CapRefusal{Which: "owner", LimitMicros: *caps.OwnerMicrosPerMonth,
				UsedMicros: totals.CostMicros, ResetAt: reset}
		}
	}
	if sessionID != "" && caps.SessionMicros != nil {
		totals, err := m.st.SessionAgentSpend(ctx, ownerID, sessionID)
		if err != nil {
			return m.unresolvedCaps(ctx, ownerID, err)
		}
		if totals.CostMicros >= *caps.SessionMicros {
			return &CapRefusal{Which: "session", LimitMicros: *caps.SessionMicros,
				UsedMicros: totals.CostMicros}
		}
	}
	if caps.InstanceMicrosPerMonth != nil {
		if refusal := m.probeInstanceCap(ctx); refusal != nil {
			return refusal
		}
	}
	return nil
}

// unresolvedCaps is the fail-closed answer to "the database is unreachable":
// that is not evidence about what somebody is allowed to spend, so the
// session is refused — with a message that says the check failed rather than
// pretending a budget was reached.
func (m *Manager) unresolvedCaps(ctx context.Context, ownerID int64, err error) *CapRefusal {
	slog.ErrorContext(ctx, "spend cap check failed: refusing rather than allowing",
		slog.Int64("owner_id", ownerID), slog.String("err", err.Error()))
	return &CapRefusal{Which: "unresolved", LimitMicros: 0, UsedMicros: 0}
}

// ProbeCaps is the pre-spawn check: the one home both session creators share
// (agentSessionOpts), answering before a subprocess exists. A refusal names
// the limit and its reset, which is the whole of the ticket's legibility
// requirement.
func (m *Manager) ProbeCaps(ctx context.Context, ownerID int64) *CapRefusal {
	if !m.cfg.Caps.AnySet() {
		return nil
	}
	return m.probeCaps(ctx, ownerID, "")
}

// checkInstanceCap is the one in-flight enforcement left: the operator's own
// money running out cannot wait for anybody's turn to end, so the instance
// ceiling is a hard stop even mid-run (the product decision of 2026-09-30
// removed every other one — a run in progress always finishes). It goes off
// in a goroutine because the read loop must not block on SQL, and because
// stopping the run needs an RPC round trip the loop itself would deadlock
// on.
func (s *Session) checkInstanceCap() {
	if s.paidBy != store.PaidByPlatform || s.mgr.cfg.Caps.InstanceMicrosPerMonth == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if refusal := s.mgr.probeInstanceCap(ctx); refusal != nil {
			s.stopRunForCap(refusal)
		}
	}()
}

// probeInstanceCap answers only the instance layer — the backstop's own
// question, asked without an owner or a session because neither bounds it.
func (m *Manager) probeInstanceCap(ctx context.Context) *CapRefusal {
	caps := m.cfg.Caps
	if caps.InstanceMicrosPerMonth == nil {
		return nil
	}
	totals, err := m.st.InstanceAgentSpend(ctx, monthStart(time.Now()))
	if err != nil {
		return m.unresolvedCaps(ctx, 0, err)
	}
	if totals.CostMicros < *caps.InstanceMicrosPerMonth {
		return nil
	}
	reset := nextMonth(time.Now())
	slog.Error("instance agent spend ceiling reached",
		slog.Int64("used_micros", totals.CostMicros),
		slog.Int64("limit_micros", *caps.InstanceMicrosPerMonth),
		slog.String("resets_at", formatReset(reset)))
	return &CapRefusal{Which: "instance", LimitMicros: *caps.InstanceMicrosPerMonth,
		UsedMicros: totals.CostMicros, ResetAt: reset}
}

// stopRunForCap is a hard stop: the run in flight is aborted and the chat is
// told once, in the canned wording. It is not a lasting verdict — the next
// prompt re-asks the limits (which will refuse it while the budget holds),
// so a session can pick back up when a budget resets.
//
// The artifact is untouched — whatever the last successful save left is what
// stands — and the transcript stays persisted: stopping is recoverable in a
// way silently continuing is not.
func (s *Session) stopRunForCap(refusal *CapRefusal) {
	s.mu.Lock()
	running := s.streaming
	s.mu.Unlock()

	slog.Warn("agent run stopped at spend cap",
		slog.String("session_id", s.ID), slog.String("owner_id", strconv.FormatInt(s.OwnerID, 10)),
		slog.String("cap", refusal.Which), slog.String("message", refusal.Message()))
	if !running {
		return
	}

	ev, _ := json.Marshal(map[string]any{
		"type":    "exhibit_spend_cap",
		"cap":     refusal.Which,
		"message": refusal.Message(),
	})
	s.broadcast(ev)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.Abort(ctx); err != nil {
			slog.Debug("spend-cap abort", slog.String("session_id", s.ID), slog.String("err", err.Error()))
		}
	}()
}

// capBeforePrompt is the between-runs gate on starting *new* spend: it asks
// every configured layer and refuses the prompt when any of them is reached,
// with the message naming the limit and its reset. A run in progress is
// never touched — this is the graceful-stop boundary. It is synchronous,
// because a refused prompt must never reach the subprocess (or pay for the
// guardrail screen inside it).
func (s *Session) capBeforePrompt() error {
	if s.paidBy != store.PaidByPlatform || !s.mgr.cfg.Caps.AnySet() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if refusal := s.mgr.probeCaps(ctx, s.OwnerID, s.ID); refusal != nil {
		return refusal
	}
	return nil
}

// armTurnClock starts the wall-clock ceiling for one turn — the hard stop
// for a response whose provider reports usage only at completion, or one
// that simply never ends. Cleared at turn_end and agent_settled.
func (s *Session) armTurnClock() {
	seconds := s.mgr.cfg.Caps.TurnSeconds
	if seconds <= 0 || s.paidBy != store.PaidByPlatform {
		return
	}
	s.mu.Lock()
	if s.turnTimer != nil {
		s.turnTimer.Stop()
	}
	s.turnTimer = time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		s.stopRunForCap(&CapRefusal{Which: "turn", Seconds: seconds})
	})
	s.mu.Unlock()
}

// clearTurnClock stops the per-turn timer. The ceiling bounds one turn's
// wall clock, not the session's.
func (s *Session) clearTurnClock() {
	s.mu.Lock()
	if s.turnTimer != nil {
		s.turnTimer.Stop()
		s.turnTimer = nil
	}
	s.mu.Unlock()
}
