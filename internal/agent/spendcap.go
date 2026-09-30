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
//     until they do). Checked before a session spawns and before every turn.
//   - Session ceiling — one conversation's lifetime, catching a loop without
//     waiting for the month to drain.
//   - Instance ceiling — the operator's total exposure per calendar month,
//     guardrail screening included, and the only layer that bounds a failure
//     nobody predicted. Logged loudly when reached.
//
// All three meter platform-paid spend only. A BYO-key session spends its
// owner's own tokens and is never limited by the operator's defaults, on any
// instance — so enforcement simply does not run for one (see
// Session.enforceCaps).
//
// # Enforcement granularity: what the protocol actually permits
//
// `before_provider_request` cannot cancel a request — it may replace the
// payload, nothing more — so the finest enforcement is per model request,
// which is better than it sounds: Pi reports usage per assistant response,
// cumulatively *during* the response, so the running total is visible before
// the next request can spend and the run is aborted the moment it crosses.
// The honest bound (docs/agent.md): **spend already in flight when a budget
// crosses is at most one request** — a request bills its input before any
// usage for it can be reported. For the residual case (a provider that
// reports usage only at completion, one endless response) the wall-clock
// ceiling per turn below aborts on time instead of on tokens.
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
			"It resets on %s. This conversation has been stopped.", used, limit, formatReset(e.ResetAt))
	case "instance":
		return fmt.Sprintf("The instance's monthly agent budget is exhausted (%s of %s used). "+
			"New agent sessions are refused until it resets on %s.", used, limit, formatReset(e.ResetAt))
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
		totals, err := m.st.InstanceAgentSpend(ctx, since)
		if err != nil {
			return m.unresolvedCaps(ctx, ownerID, err)
		}
		if totals.CostMicros >= *caps.InstanceMicrosPerMonth {
			// Logged loudly here rather than at each caller: this is the
			// backstop tripping — a mistake in the other layers or a failure
			// nobody predicted — and an operator wants to know before the
			// next support thread does.
			slog.Error("instance agent spend ceiling reached",
				slog.Int64("used_micros", totals.CostMicros),
				slog.Int64("limit_micros", *caps.InstanceMicrosPerMonth),
				slog.String("resets_at", formatReset(reset)))
			return &CapRefusal{Which: "instance", LimitMicros: *caps.InstanceMicrosPerMonth,
				UsedMicros: totals.CostMicros, ResetAt: reset}
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

// checkCaps is the between-turns check, beside the readLoop as the ticket
// asks: a session that is already running is the one that runs away. It goes
// off in a goroutine because the read loop must not block on SQL, and because
// stopping the run needs an RPC round trip the loop itself would deadlock on.
func (s *Session) checkCaps() {
	if s.paidBy != store.PaidByPlatform || !s.mgr.cfg.Caps.AnySet() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if refusal := s.mgr.probeCaps(ctx, s.OwnerID, s.ID); refusal != nil {
			s.stopForCap(refusal)
		}
	}()
}

// checkCapsStreaming is the mid-response check: the cumulative usage Pi
// streams during one assistant response is live input, so a runaway response
// is stopped while it is still generating rather than after it lands. It is
// throttled to one probe per capCheckInterval — streaming deltas are
// high-frequency and the answer only has to beat the *next* request.
func (s *Session) checkCapsStreaming() {
	if s.paidBy != store.PaidByPlatform || !s.mgr.cfg.Caps.AnySet() {
		return
	}
	s.mu.Lock()
	if time.Since(s.lastCapCheck) < capCheckInterval {
		s.mu.Unlock()
		return
	}
	s.lastCapCheck = time.Now()
	s.mu.Unlock()
	s.checkCaps()
}

const capCheckInterval = 2 * time.Second

// stopForCap stops a running session at the first opportunity after it
// crossed a budget. The refusal sticks: every later prompt gets the same
// message, so a session cannot spend its way back under by racing the next
// turn. The artifact is untouched — whatever the last successful save left is
// what stands — and the transcript stays persisted.
//
// The canned notice goes out only when there is a run to stop: a turn refused
// before it starts carries the same message in its HTTP response instead, and
// the chat must not be told twice.
func (s *Session) stopForCap(refusal *CapRefusal) {
	s.mu.Lock()
	if s.capped != nil {
		s.mu.Unlock()
		return
	}
	s.capped = refusal
	running := s.streaming
	s.mu.Unlock()

	slog.Warn("agent session stopped at spend cap",
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

// capBeforePrompt is the between-turns gate on starting *new* spend: a
// session already running must not begin another turn over budget. It is
// synchronous, because a refused prompt must never reach the subprocess.
func (s *Session) capBeforePrompt() error {
	s.mu.Lock()
	capped := s.capped
	s.mu.Unlock()
	if capped != nil {
		return capped
	}
	if s.paidBy != store.PaidByPlatform || !s.mgr.cfg.Caps.AnySet() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if refusal := s.mgr.probeCaps(ctx, s.OwnerID, s.ID); refusal != nil {
		s.stopForCap(refusal)
		return refusal
	}
	return nil
}

// armTurnClock starts the wall-clock ceiling for one turn (the fallback for
// a response whose provider reports usage only at completion). Cleared at
// turn_end and agent_settled; a fire stops the session like a budget
// crossing does.
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
		s.stopForCap(&CapRefusal{Which: "turn", Seconds: seconds})
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
