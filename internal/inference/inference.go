// Package inference is the shared core every artifact AI call goes through
// (Exh-hwjg). An artifact calls window.exhibit.ai.complete() or decide(); the
// host frame posts the call to POST /api/artifacts/:id/inference; the handler
// resolves who is asking and hands this package a Call. Everything after that
// is here, in one order, for every kind:
//
//	enablement → principal → payload → payer → model → limits → screen → provider → meter
//
// The three questions it answers are who pays (always the viewer, never an
// owner fallback), may they (enablement, a key, the shared spend ceilings,
// rate limits), and what did it cost (one agent_usage row per provider call,
// recorded before any error or validation mapping, because a failed call can
// still have been billed).
//
// A kind (complete, decide) contributes only what is particular to it: how its
// payload is validated and bounded, how the provider's answer is checked, and
// its per-call defaults. Neither kind reimplements payer, limits or metering.
//
// What reaches a browser is a value or a stable error code with fixed text.
// Provider and model identifiers, upstream error text and usage never leave
// the server through this package's results.
package inference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/momja/Exhibit/internal/store"
)

// Kind is one family of call.
type Kind string

const (
	// KindComplete is a general language-model call (Exh-gd3g).
	KindComplete Kind = "complete"
	// KindDecide is a decision-model call that answers typed questions with
	// probabilities or confidence (Exh-x16u).
	KindDecide Kind = "decide"
)

// Kinds lists every kind in the order surfaces describe them.
var Kinds = []Kind{KindComplete, KindDecide}

// ParseKind accepts only a known kind.
func ParseKind(s string) (Kind, bool) {
	for _, k := range Kinds {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}

// Stable error codes: the whole vocabulary an artifact can see (Exh-zbwk).
const (
	CodeUnavailable    = "unavailable"
	CodeNotApproved    = "not_approved"
	CodeNoKey          = "no_key"
	CodeLimitReached   = "limit_reached"
	CodeInvalidRequest = "invalid_request"
	CodeRateLimited    = "rate_limited"
	CodeProviderError  = "provider_error"
	// CodeInvalidOutput is complete()'s: structured output that still did not
	// match the schema after one repair attempt.
	CodeInvalidOutput = "invalid_output"
)

// Error is a refusal or failure with a stable code and fixed, server-authored
// text. Message never contains upstream text.
type Error struct {
	Code    string
	Message string
	// ResetAt is when a spend limit resets; zero otherwise.
	ResetAt time.Time
	// RetryAfter is set on rate limits.
	RetryAfter time.Duration
	// Limit names the spend layer for limit_reached: "owner" or "instance".
	Limit string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func refuse(code, message string) *Error { return &Error{Code: code, Message: message} }

var errUnavailable = refuse(CodeUnavailable, "AI is not available here.")

// Credential is a provider and the key that pays for a call.
type Credential struct {
	Provider string
	APIKey   string
}

// KeyResolver returns a viewer's own stored agent provider and key (BYOK).
// ok is false when the viewer has none.
type KeyResolver func(ctx context.Context, viewerID int64) (cred Credential, ok bool, err error)

// Ledger is the part of the store the core writes and reads.
type Ledger interface {
	RecordAgentUsage(ctx context.Context, u store.AgentUsage) error
	OwnerAgentSpend(ctx context.Context, ownerID int64, since time.Time) (store.UsageTotals, error)
	InstanceAgentSpend(ctx context.Context, since time.Time) (store.UsageTotals, error)
}

// SpendCaps are the agent's monthly ceilings, shared by inference (Exh-hwjg).
// A nil field is a layer not in use.
type SpendCaps struct {
	OwnerMicrosPerMonth    *int64
	InstanceMicrosPerMonth *int64
}

// Guardrail is the operator's usage-policy screen (av-gust), applied to
// platform-paid inputs before the provider call.
type Guardrail struct {
	Provider string
	Model    string
	APIKey   string
}

// Config configures a Service.
type Config struct {
	// Enabled is INFERENCE_ENABLED. Off, every call is unavailable and
	// Availability reports no kinds.
	Enabled bool
	// Platform is the instance credential (AGENT_API_KEY + AGENT_PROVIDER).
	// Nil is BYOK: each viewer pays with their own stored key.
	Platform *Credential
	// ChatModel is INFERENCE_MODEL; DecisionModel is INFERENCE_DECISION_MODEL.
	ChatModel     string
	DecisionModel string
	Keys          KeyResolver
	Ledger        Ledger
	Caps          SpendCaps
	Guardrail     *Guardrail
	Runtime       Runtime
	// Rates bounds calls per viewer and per artifact. Zero uses defaults.
	Rates RateConfig
	Now   func() time.Time
}

// Call is one artifact AI call, already resolved by the endpoint.
type Call struct {
	ArtifactID string
	// ViewerID pays. The core refuses a nonpositive viewer and an anonymous
	// call itself, whatever the endpoint decided (Exh-hwjg acceptance).
	ViewerID  int64
	Anonymous bool
	Kind      Kind
	Payload   json.RawMessage
}

// Availability is what a viewer may use right now, for available() and the
// Settings page. It never names a provider or model.
type Availability struct {
	Complete bool `json:"complete"`
	Decide   bool `json:"decide"`
	// Payer is "platform" (the viewer's Exhibit allowance), "own_key" (their
	// stored agent key) or "" when nothing is available.
	Payer string `json:"payer,omitempty"`
	// Reason explains an empty availability: "disabled", "no_key" or
	// "unavailable".
	Reason string `json:"reason,omitempty"`
}

// Kinds lists the available kinds.
func (a Availability) Kinds() []Kind {
	var out []Kind
	if a.Complete {
		out = append(out, KindComplete)
	}
	if a.Decide {
		out = append(out, KindDecide)
	}
	return out
}

// Service is the inference core.
type Service struct {
	cfg   Config
	rates *rateLimiter

	mu       sync.Mutex
	resolved map[resolveKey]bool
}

type resolveKey struct {
	provider string
	kind     Kind
	model    string
}

// New returns a Service. A disabled config needs no runtime.
func New(cfg Config) (*Service, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Enabled {
		if cfg.Runtime == nil || cfg.Ledger == nil {
			return nil, errors.New("inference: an enabled service needs a runtime and a ledger")
		}
		if cfg.Platform == nil && cfg.Keys == nil {
			return nil, errors.New("inference: BYOK mode needs a key resolver")
		}
	}
	return &Service{cfg: cfg, rates: newRateLimiter(cfg.Rates, cfg.Now), resolved: map[resolveKey]bool{}}, nil
}

// Enabled reports INFERENCE_ENABLED.
func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled }

// PlatformMode reports whether the instance pays.
func (s *Service) PlatformMode() bool { return s != nil && s.cfg.Platform != nil }

// CheckStartup verifies the platform configuration against the runtime's
// catalog. INFERENCE_MODEL is required in platform mode and must resolve to a
// chat model; INFERENCE_DECISION_MODEL, when set, must resolve to a
// classifier. Unset, decide is simply unavailable. The caller makes an error
// fatal, like a malformed AGENT_PROVIDER.
func (s *Service) CheckStartup(ctx context.Context) error {
	if !s.Enabled() || s.cfg.Platform == nil {
		return nil
	}
	if s.cfg.ChatModel == "" {
		return errors.New("INFERENCE_ENABLED is set in platform mode but INFERENCE_MODEL is not")
	}
	if ok, err := s.cfg.Runtime.Resolve(ctx, s.cfg.Platform.Provider, KindComplete, s.cfg.ChatModel); err != nil {
		return fmt.Errorf("check INFERENCE_MODEL: %w", err)
	} else if !ok {
		return errors.New("INFERENCE_MODEL does not name a chat model for AGENT_PROVIDER")
	}
	if s.cfg.DecisionModel != "" {
		if ok, err := s.cfg.Runtime.Resolve(ctx, s.cfg.Platform.Provider, KindDecide, s.cfg.DecisionModel); err != nil {
			return fmt.Errorf("check INFERENCE_DECISION_MODEL: %w", err)
		} else if !ok {
			return errors.New("INFERENCE_DECISION_MODEL does not name a classifier model for AGENT_PROVIDER")
		}
	}
	return nil
}

// payer is a resolved payer: who, with what, and which models.
type payer struct {
	cred   Credential
	paidBy string
	models map[Kind]string
}

// resolvePayer answers "who pays, and with which models" for a viewer. Errors
// are *Error values ready for the browser, except a store failure.
func (s *Service) resolvePayer(ctx context.Context, viewerID int64) (*payer, error) {
	if !s.Enabled() {
		return nil, errUnavailable
	}
	if viewerID <= 0 {
		return nil, errUnavailable
	}
	p := &payer{models: map[Kind]string{}}
	if s.cfg.Platform != nil {
		p.cred, p.paidBy = *s.cfg.Platform, store.PaidByPlatform
		p.models[KindComplete] = s.cfg.ChatModel
		if s.cfg.DecisionModel != "" {
			p.models[KindDecide] = s.cfg.DecisionModel
		}
		return p, nil
	}
	cred, ok, err := s.cfg.Keys(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	if !ok || cred.APIKey == "" {
		return nil, refuse(CodeNoKey, "Add your own AI key to use AI in artifacts.")
	}
	p.cred, p.paidBy = cred, store.PaidByUser
	for _, kind := range Kinds {
		for _, candidate := range []string{s.configuredModel(kind), defaultModel(kind, cred.Provider)} {
			if candidate == "" {
				continue
			}
			ok, err := s.resolve(ctx, cred.Provider, kind, candidate)
			if err != nil {
				return nil, err
			}
			if ok {
				p.models[kind] = candidate
				break
			}
		}
	}
	return p, nil
}

func (s *Service) configuredModel(kind Kind) string {
	if kind == KindDecide {
		return s.cfg.DecisionModel
	}
	return s.cfg.ChatModel
}

// resolve asks the runtime's catalog whether a model exists for a provider
// and kind, caching the answer: the catalog is offline and pinned, so an
// answer cannot change while the process runs.
func (s *Service) resolve(ctx context.Context, provider string, kind Kind, model string) (bool, error) {
	key := resolveKey{provider, kind, model}
	s.mu.Lock()
	ok, cached := s.resolved[key]
	s.mu.Unlock()
	if cached {
		return ok, nil
	}
	ok, err := s.cfg.Runtime.Resolve(ctx, provider, kind, model)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	s.resolved[key] = ok
	s.mu.Unlock()
	return ok, nil
}

// Availability reports what a viewer may use. A store or runtime failure is
// reported as unavailable rather than an error, so a page always renders.
func (s *Service) Availability(ctx context.Context, viewerID int64) Availability {
	if !s.Enabled() {
		return Availability{Reason: "disabled"}
	}
	p, err := s.resolvePayer(ctx, viewerID)
	if err != nil {
		var refusal *Error
		if errors.As(err, &refusal) && refusal.Code == CodeNoKey {
			return Availability{Reason: "no_key"}
		}
		if !errors.As(err, &refusal) {
			slog.WarnContext(ctx, "inference availability", slog.String("err", err.Error()))
		}
		return Availability{Reason: "unavailable"}
	}
	a := Availability{Complete: p.models[KindComplete] != "", Decide: p.models[KindDecide] != ""}
	if !a.Complete && !a.Decide {
		a.Reason = "unavailable"
		return a
	}
	a.Payer = "own_key"
	if p.paidBy == store.PaidByPlatform {
		a.Payer = "platform"
	}
	return a
}

// Run performs one call end to end and returns the artifact-facing value.
func (s *Service) Run(ctx context.Context, call Call) (json.RawMessage, error) {
	if !s.Enabled() {
		return nil, errUnavailable
	}
	// The core's own principal check, independent of the endpoint's: an
	// anonymous or public-link visitor never pays for anything.
	if call.Anonymous || call.ViewerID <= 0 {
		return nil, errUnavailable
	}
	k, ok := kinds[call.Kind]
	if !ok {
		return nil, refuse(CodeInvalidRequest, "Unknown AI method.")
	}
	req, err := k.prepare(call.Payload)
	if err != nil {
		return nil, err
	}
	p, err := s.resolvePayer(ctx, call.ViewerID)
	if err != nil {
		return nil, err
	}
	model := p.models[call.Kind]
	if model == "" {
		return nil, refuse(CodeUnavailable, "This kind of AI call is not available here.")
	}
	release, err := s.rates.admit(call.ViewerID, call.ArtifactID, call.Kind)
	if err != nil {
		return nil, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()
	callID := "inference:" + uuid.NewString()
	var gate func() error
	if p.paidBy == store.PaidByPlatform {
		gate = func() error { return s.checkSpend(ctx, call.ViewerID) }
		if s.cfg.Guardrail != nil {
			if err := gate(); err != nil {
				return nil, err
			}
			if err := s.screen(ctx, call, callID, req.screenText); err != nil {
				return nil, err
			}
		}
	}
	job := Job{Kind: call.Kind, Provider: p.cred.Provider, Model: model, APIKey: p.cred.APIKey}
	meter := func(o Outcome) {
		s.meter(call, callID, p, model, o)
	}
	return k.run(ctx, s.cfg.Runtime, job, req, meter, gate)
}

// screen runs the operator's guardrail over a platform-paid input. A screen
// that fails to reach a verdict refuses, as the agent's does (fail closed).
func (s *Service) screen(ctx context.Context, call Call, callID, text string) error {
	g := s.cfg.Guardrail
	verdict, err := s.cfg.Runtime.Screen(ctx, ScreenJob{Provider: g.Provider, Model: g.Model, APIKey: g.APIKey, Text: text})
	if verdict.Usage != nil {
		s.record(store.AgentUsage{OwnerID: call.ViewerID, SessionID: callID, Provider: g.Provider, Model: g.Model,
			PaidBy: store.PaidByPlatform, Source: store.UsageSourceGuardrail, ArtifactID: call.ArtifactID},
			verdict.Usage)
	}
	if err != nil {
		slog.WarnContext(ctx, "inference guardrail failed closed", slog.String("artifact_id", call.ArtifactID), slog.String("err", err.Error()))
		return refuse(CodeProviderError, "The AI request could not be checked, so it was not sent.")
	}
	if !verdict.Allowed {
		return refuse(CodeInvalidRequest, "This AI request was refused by the instance's usage policy.")
	}
	return nil
}

// meter records one provider outcome. Usage missing from a failed or
// canceled call is recorded as a zero-token row with cost_source none: the
// call happened and its spend is unknown, which is different from free.
func (s *Service) meter(call Call, callID string, p *payer, model string, o Outcome) {
	if !o.Dispatched {
		return
	}
	s.record(store.AgentUsage{
		OwnerID: call.ViewerID, SessionID: callID, Provider: p.cred.Provider, Model: model,
		PaidBy: p.paidBy, Source: store.UsageSourceInference, ArtifactID: call.ArtifactID, Kind: string(call.Kind),
	}, o.Usage)
}

func (s *Service) record(row store.AgentUsage, u *Usage) {
	if u == nil {
		row.CostSource = store.CostSourceNone
	} else {
		row.InputTokens, row.OutputTokens = u.Input, u.Output
		row.CacheReadTokens, row.CacheWriteTokens = u.CacheRead, u.CacheWrite
		row.CostMicros = costMicros(u.Cost.Total)
		row.CostSource = store.CostSourceCatalog
		if u.Cost.TotalFromProvider {
			row.CostSource = store.CostSourceProvider
		}
	}
	// A fresh context: the call's own may already be canceled, and the row
	// is the only record of money that was spent.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.cfg.Ledger.RecordAgentUsage(ctx, row); err != nil {
		slog.Error("inference usage row failed", slog.String("artifact_id", row.ArtifactID), slog.String("err", err.Error()))
	}
}

// checkSpend applies the agent's monthly ceilings to a platform-paid call:
// one owner total and one instance total, shared with the agent. Checked
// before the call; a call in flight is never cut off.
func (s *Service) checkSpend(ctx context.Context, viewerID int64) error {
	now := s.cfg.Now().UTC()
	since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	reset := since.AddDate(0, 1, 0)
	if c := s.cfg.Caps.OwnerMicrosPerMonth; c != nil {
		totals, err := s.cfg.Ledger.OwnerAgentSpend(ctx, viewerID, since)
		if err != nil {
			slog.ErrorContext(ctx, "inference spend check failed: refusing", slog.String("err", err.Error()))
			return refuse(CodeProviderError, "Your AI allowance could not be checked. Try again later.")
		}
		if totals.CostMicros >= *c {
			return &Error{Code: CodeLimitReached, Limit: "owner", ResetAt: reset,
				Message: "You have used this month's AI allowance. It resets on " + reset.Format("January 2") + "."}
		}
	}
	if c := s.cfg.Caps.InstanceMicrosPerMonth; c != nil {
		totals, err := s.cfg.Ledger.InstanceAgentSpend(ctx, since)
		if err != nil {
			slog.ErrorContext(ctx, "inference spend check failed: refusing", slog.String("err", err.Error()))
			return refuse(CodeProviderError, "The AI allowance could not be checked. Try again later.")
		}
		if totals.CostMicros >= *c {
			slog.Error("instance spend ceiling reached for artifact inference")
			return &Error{Code: CodeLimitReached, Limit: "instance", ResetAt: reset,
				Message: "AI is paused on this instance until " + reset.Format("January 2") + "."}
		}
	}
	return nil
}

// Allowance is a payer's platform-paid use this month against the owner
// ceiling, for the Settings page.
type Allowance struct {
	// Limited is false when no owner ceiling is configured.
	Limited bool
	// UsedPercent is 0–100, rounded down; exact money is deliberately not
	// shown (cost is an estimate, not a bill).
	UsedPercent int
	// InferenceShare is the share of the used amount spent by artifacts,
	// 0–100, the rest being the agent's.
	InferenceShare int
	// Spent says anything was spent this month, so a share that rounds to
	// 0% can still be told apart from nothing at all.
	Spent   bool
	ResetAt time.Time
}

// AllowanceFor reads a payer's month against the shared owner ceiling.
func (s *Service) AllowanceFor(ctx context.Context, viewerID int64, inferenceMicros int64) (Allowance, error) {
	now := s.cfg.Now().UTC()
	since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	a := Allowance{ResetAt: since.AddDate(0, 1, 0)}
	totals, err := s.cfg.Ledger.OwnerAgentSpend(ctx, viewerID, since)
	if err != nil {
		return a, err
	}
	if totals.CostMicros > 0 {
		a.Spent = true
		a.InferenceShare = int(min(100, inferenceMicros*100/totals.CostMicros))
	}
	if c := s.cfg.Caps.OwnerMicrosPerMonth; c != nil {
		a.Limited = true
		if *c <= 0 {
			a.UsedPercent = 100
		} else {
			a.UsedPercent = int(min(100, totals.CostMicros*100 / *c))
		}
	}
	return a, nil
}

// MonthStart is the first instant of the current UTC month, the period every
// allowance resets on.
func (s *Service) MonthStart() time.Time {
	now := s.cfg.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// InstanceKinds is what this instance can offer anybody, for teaching agents
// which calls to write (Exh-15tf). Platform mode offers exactly the configured
// models. BYOK offers both kinds, because whether a viewer's own provider has a
// model for a kind is known only per viewer: artifacts must check available()
// either way, and the guidance says so.
func (s *Service) InstanceKinds() []Kind {
	if !s.Enabled() {
		return nil
	}
	if s.cfg.Platform == nil {
		return []Kind{KindComplete, KindDecide}
	}
	var out []Kind
	if s.cfg.ChatModel != "" {
		out = append(out, KindComplete)
	}
	if s.cfg.DecisionModel != "" {
		out = append(out, KindDecide)
	}
	return out
}
