package inference

import (
	"context"
	"encoding/json"
	"errors"
	"math"
)

// Runtime reaches providers. Production uses the Pi worker (worker.go): one
// warm, pinned Pi process whose model registry serves every call through its
// own provider adapters (Exh-urrg). Tests substitute a fake.
type Runtime interface {
	// Resolve reports whether model names a model of this kind for provider
	// in the runtime's catalog: a chat model for complete, a classifier for
	// decide.
	Resolve(ctx context.Context, provider string, kind Kind, model string) (bool, error)
	// Run performs one provider call. The Outcome is meaningful whatever the
	// error: it says whether the call may have reached the provider and what
	// it reported spending, so the caller meters before it maps the error.
	Run(ctx context.Context, job Job) (Result, Outcome, error)
	// Screen runs the usage-policy guardrail over text.
	Screen(ctx context.Context, job ScreenJob) (ScreenVerdict, error)
}

// Job is one provider call with its credential. The key travels with the job
// and nowhere else: never into the worker's environment, argv or logs.
type Job struct {
	Kind     Kind
	Provider string
	Model    string
	APIKey   string
	// Context is Pi's request context: {systemPrompt, messages} for complete,
	// {state, questions} for decide.
	Context   json.RawMessage
	MaxTokens int
}

// Result is a provider's answer, before the kind validates it.
type Result struct {
	// StopReason is Pi's: stop, length, error or aborted.
	StopReason string
	Text       string
	Answers    json.RawMessage
}

// Outcome is what metering needs from a call.
type Outcome struct {
	// Dispatched is false only when the call certainly never reached a
	// provider (refused before sending, no such model). A call whose fate is
	// unknown — the worker died, the deadline passed — counts as dispatched.
	Dispatched bool
	// Usage is what the provider reported; nil when it reported nothing,
	// which is unknown spend, not zero spend.
	Usage *Usage
}

// Usage is Pi's usage report.
type Usage struct {
	Input, Output, CacheRead, CacheWrite int64
	Cost                                 UsageCost
}

// UsageCost is the cost of a call in USD.
type UsageCost struct {
	Total float64
	// TotalFromProvider is true when the provider reported the cost itself
	// rather than Pi estimating it from its price table.
	TotalFromProvider bool
}

// ScreenJob is one guardrail screen.
type ScreenJob struct {
	Provider, Model, APIKey, Text string
}

// ScreenVerdict is a screen's answer and what it spent.
type ScreenVerdict struct {
	Allowed    bool
	Dispatched bool
	Usage      *Usage
}

// Runtime errors the core maps to stable codes.
var (
	// ErrRuntimeUnavailable: the worker is not running and may not be
	// started now (circuit open, binary missing). Nothing was dispatched.
	ErrRuntimeUnavailable = errors.New("inference runtime unavailable")
	// ErrRuntimeBusy: admission refused the call. Nothing was dispatched.
	ErrRuntimeBusy = errors.New("inference runtime busy")
	// ErrNoModel: the provider has no such model. Nothing was dispatched.
	ErrNoModel = errors.New("no such model")
	// ErrProvider: the provider failed, or the outcome is unknown.
	ErrProvider = errors.New("provider error")
)

// decodeUsage reads Pi's usage shape and returns nil for an absent report.
func decodeUsage(raw json.RawMessage) *Usage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var wire struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cacheRead"`
		CacheWrite float64 `json:"cacheWrite"`
		Cost       struct {
			Total             float64 `json:"total"`
			TotalFromProvider bool    `json:"totalFromProvider"`
		} `json:"cost"`
	}
	if json.Unmarshal(raw, &wire) != nil {
		return nil
	}
	count := func(f float64) int64 {
		if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return int64(math.Round(f))
	}
	u := &Usage{Input: count(wire.Input), Output: count(wire.Output), CacheRead: count(wire.CacheRead), CacheWrite: count(wire.CacheWrite)}
	if wire.Cost.Total >= 0 && !math.IsInf(wire.Cost.Total, 0) {
		u.Cost.Total = wire.Cost.Total
		u.Cost.TotalFromProvider = wire.Cost.TotalFromProvider
	}
	// Pi starts every call with a zero-filled usage object and keeps it when
	// the stream fails before the provider reports anything. With no explicit
	// provider cost, the zero-filled placeholder means unknown spend.
	if u.Input == 0 && u.Output == 0 && u.CacheRead == 0 && u.CacheWrite == 0 && u.Cost == (UsageCost{}) {
		return nil
	}
	return u
}

// costMicros converts USD to the ledger's micro-dollars.
func costMicros(usd float64) int64 {
	if usd <= 0 || math.IsNaN(usd) {
		return 0
	}
	return int64(math.Round(usd * 1e6))
}
