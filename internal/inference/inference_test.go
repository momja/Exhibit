package inference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
)

// fakeRuntime answers from a script and records every job.
type fakeRuntime struct {
	mu          sync.Mutex
	jobs        []Job
	screens     []ScreenJob
	models      map[string]bool // provider|kind|model
	reply       func(Job) (Result, Outcome, error)
	screenReply func(ScreenJob) (ScreenVerdict, error)
}

func (f *fakeRuntime) Resolve(_ context.Context, provider string, kind Kind, model string) (bool, error) {
	return f.models[provider+"|"+string(kind)+"|"+model], nil
}

func (f *fakeRuntime) Run(_ context.Context, job Job) (Result, Outcome, error) {
	f.mu.Lock()
	f.jobs = append(f.jobs, job)
	f.mu.Unlock()
	if f.reply != nil {
		return f.reply(job)
	}
	return Result{StopReason: "stop", Text: "hello"}, Outcome{Dispatched: true, Usage: &Usage{Input: 10, Output: 2, Cost: UsageCost{Total: 0.001}}}, nil
}

func (f *fakeRuntime) Screen(_ context.Context, job ScreenJob) (ScreenVerdict, error) {
	f.mu.Lock()
	f.screens = append(f.screens, job)
	f.mu.Unlock()
	if f.screenReply != nil {
		return f.screenReply(job)
	}
	return ScreenVerdict{Allowed: true}, nil
}

func (f *fakeRuntime) calls() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.jobs) }

// fakeLedger is an in-memory agent_usage.
type fakeLedger struct {
	mu   sync.Mutex
	rows []store.AgentUsage
	err  error
}

func (l *fakeLedger) RecordAgentUsage(_ context.Context, u store.AgentUsage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rows = append(l.rows, u)
	return nil
}

func (l *fakeLedger) sum(match func(store.AgentUsage) bool) store.UsageTotals {
	l.mu.Lock()
	defer l.mu.Unlock()
	var t store.UsageTotals
	for _, r := range l.rows {
		if match(r) {
			t.CostMicros += r.CostMicros
		}
	}
	return t
}

func (l *fakeLedger) OwnerAgentSpend(_ context.Context, owner int64, _ time.Time) (store.UsageTotals, error) {
	return l.sum(func(r store.AgentUsage) bool { return r.OwnerID == owner && r.PaidBy == store.PaidByPlatform }), l.err
}

func (l *fakeLedger) InstanceAgentSpend(context.Context, time.Time) (store.UsageTotals, error) {
	return l.sum(func(r store.AgentUsage) bool { return r.PaidBy == store.PaidByPlatform }), l.err
}

const (
	owner     = int64(1)
	recipient = int64(2)
)

func newTestService(t *testing.T, mutate func(*Config)) (*Service, *fakeRuntime, *fakeLedger) {
	t.Helper()
	rt := &fakeRuntime{models: map[string]bool{
		"openrouter|complete|chat-1":          true,
		"openrouter|decide|judge-1":           true,
		"anthropic|complete|claude-haiku-4-5": true,
	}}
	led := &fakeLedger{}
	cfg := Config{
		Enabled: true, Platform: &Credential{Provider: "openrouter", APIKey: "platform-key"},
		ChatModel: "chat-1", DecisionModel: "judge-1", Ledger: led, Runtime: rt,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s, rt, led
}

func completeCall(viewer int64) Call {
	return Call{ArtifactID: "art-1", ViewerID: viewer, Kind: KindComplete, Payload: json.RawMessage(`{"messages":[{"role":"user","content":"Hi"}]}`)}
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestPlatformCallChargesTheViewer(t *testing.T) {
	s, rt, led := newTestService(t, nil)
	out, err := s.Run(context.Background(), completeCall(recipient))
	if err != nil || string(out) != `"hello"` {
		t.Fatalf("run = %s, %v", out, err)
	}
	if rt.jobs[0].APIKey != "platform-key" || rt.jobs[0].Model != "chat-1" {
		t.Fatalf("job = %+v", rt.jobs[0])
	}
	if len(led.rows) != 1 {
		t.Fatalf("rows = %+v", led.rows)
	}
	r := led.rows[0]
	if r.OwnerID != recipient || r.PaidBy != store.PaidByPlatform || r.Source != store.UsageSourceInference ||
		r.Kind != "complete" || r.ArtifactID != "art-1" || r.CostMicros != 1000 || r.CostSource != store.CostSourceCatalog {
		t.Fatalf("row = %+v", r)
	}
}

func TestBYOKCallUsesTheViewersOwnKeyNeverTheOwners(t *testing.T) {
	keys := map[int64]Credential{owner: {Provider: "openrouter", APIKey: "owner-key"}, recipient: {Provider: "anthropic", APIKey: "recipient-key"}}
	s, rt, led := newTestService(t, func(c *Config) {
		c.Platform = nil
		configureTestGuardrail(c)
		zero := int64(0)
		c.Caps = SpendCaps{OwnerMicrosPerMonth: &zero, InstanceMicrosPerMonth: &zero}
		c.Keys = func(_ context.Context, v int64) (Credential, bool, error) { k, ok := keys[v]; return k, ok, nil }
	})
	if _, err := s.Run(context.Background(), completeCall(recipient)); err != nil {
		t.Fatal(err)
	}
	job := rt.jobs[0]
	if job.APIKey != "recipient-key" || job.Provider != "anthropic" || job.Model != "claude-haiku-4-5" {
		t.Fatalf("job = %+v (BYOK falls back to the provider default when INFERENCE_MODEL does not resolve)", job)
	}
	if led.rows[0].OwnerID != recipient || led.rows[0].PaidBy != store.PaidByUser {
		t.Fatalf("row = %+v", led.rows[0])
	}
	if len(rt.screens) != 0 {
		t.Fatal("a BYOK call reached the platform guardrail")
	}
	// A viewer with no key of their own gets no_key; nobody else's key is used.
	delete(keys, recipient)
	if _, err := s.Run(context.Background(), completeCall(recipient)); code(err) != CodeNoKey {
		t.Fatalf("no key = %v", err)
	}
	if rt.calls() != 1 {
		t.Fatal("a viewer without a key reached the provider")
	}
}

func TestCoreRefusesAnonymousAndUnknownPrincipals(t *testing.T) {
	s, rt, _ := newTestService(t, nil)
	for _, call := range []Call{
		{ArtifactID: "a", ViewerID: owner, Anonymous: true, Kind: KindComplete, Payload: completeCall(owner).Payload},
		{ArtifactID: "a", ViewerID: 0, Kind: KindComplete, Payload: completeCall(owner).Payload},
	} {
		if _, err := s.Run(context.Background(), call); code(err) != CodeUnavailable {
			t.Fatalf("call %+v = %v", call, err)
		}
	}
	if rt.calls() != 0 {
		t.Fatal("an anonymous principal reached the provider")
	}
}

func TestSpendCapsRefuseBeforeTheProviderAndAreSharedWithTheAgent(t *testing.T) {
	ownerCap, instanceCap := int64(5000), int64(1_000_000)
	s, rt, led := newTestService(t, func(c *Config) {
		c.Caps = SpendCaps{OwnerMicrosPerMonth: &ownerCap, InstanceMicrosPerMonth: &instanceCap}
	})
	// The agent already spent most of this owner's month.
	led.rows = append(led.rows, store.AgentUsage{OwnerID: owner, PaidBy: store.PaidByPlatform, Source: store.UsageSourceModel, CostMicros: 4500})
	if _, err := s.Run(context.Background(), completeCall(owner)); err != nil {
		t.Fatalf("under the cap: %v", err)
	}
	_, err := s.Run(context.Background(), completeCall(owner))
	var refusal *Error
	if !errors.As(err, &refusal) || refusal.Code != CodeLimitReached || refusal.Limit != "owner" || refusal.ResetAt.IsZero() {
		t.Fatalf("over the cap = %v", err)
	}
	if rt.calls() != 1 {
		t.Fatalf("provider calls = %d", rt.calls())
	}
	// Another viewer is unaffected by the first one's spend.
	if _, err := s.Run(context.Background(), completeCall(recipient)); err != nil {
		t.Fatalf("other viewer: %v", err)
	}
	// The instance ceiling refuses everyone.
	led.rows = append(led.rows, store.AgentUsage{OwnerID: 99, PaidBy: store.PaidByPlatform, CostMicros: instanceCap})
	if _, err := s.Run(context.Background(), completeCall(recipient)); !errors.As(err, &refusal) || refusal.Limit != "instance" {
		t.Fatalf("instance cap = %v", err)
	}
}

func TestUsageIsMeteredBeforeErrorsAndUnknownSpendIsNotFree(t *testing.T) {
	s, rt, led := newTestService(t, nil)
	rt.reply = func(Job) (Result, Outcome, error) {
		return Result{StopReason: "error"}, Outcome{Dispatched: true, Usage: &Usage{Input: 50, Cost: UsageCost{Total: 0.002}}}, nil
	}
	if _, err := s.Run(context.Background(), completeCall(owner)); code(err) != CodeProviderError {
		t.Fatalf("err = %v", err)
	}
	rt.reply = func(Job) (Result, Outcome, error) { return Result{}, Outcome{Dispatched: true}, ErrProvider }
	if _, err := s.Run(context.Background(), completeCall(owner)); code(err) != CodeProviderError {
		t.Fatalf("err = %v", err)
	}
	rt.reply = func(Job) (Result, Outcome, error) { return Result{}, Outcome{}, ErrRuntimeBusy }
	if _, err := s.Run(context.Background(), completeCall(owner)); code(err) != CodeRateLimited {
		t.Fatalf("busy = %v", err)
	}
	if len(led.rows) != 2 || led.rows[0].CostMicros != 2000 || led.rows[1].CostSource != store.CostSourceNone {
		t.Fatalf("rows = %+v", led.rows)
	}
}

func TestStructuredOutputIsValidatedWithOneRepair(t *testing.T) {
	s, rt, led := newTestService(t, nil)
	payload := json.RawMessage(`{"messages":[{"role":"user","content":"Tag it"}],"schema":{"type":"object","properties":{"tags":{"type":"array","items":{"type":"string"},"maxItems":3}},"required":["tags"],"additionalProperties":false}}`)
	answers := []string{"Sure, here you go", "```json\n{\"tags\":[\"a\",\"b\"]}\n```"}
	rt.reply = func(Job) (Result, Outcome, error) {
		text := answers[0]
		answers = answers[1:]
		return Result{StopReason: "stop", Text: text}, Outcome{Dispatched: true, Usage: &Usage{Input: 1}}, nil
	}
	out, err := s.Run(context.Background(), Call{ArtifactID: "a", ViewerID: owner, Kind: KindComplete, Payload: payload})
	if err != nil || string(out) != `{"tags":["a","b"]}` {
		t.Fatalf("out = %s, %v", out, err)
	}
	if len(led.rows) != 2 {
		t.Fatalf("a repaired call is two metered calls; rows = %d", len(led.rows))
	}
	if !strings.Contains(string(rt.jobs[1].Context), "did not match the required JSON Schema") {
		t.Fatalf("repair context = %s", rt.jobs[1].Context)
	}
	// Two bad answers: invalid_output, never the unvalidated text.
	rt.reply = func(Job) (Result, Outcome, error) {
		return Result{StopReason: "stop", Text: `{"tags":"no"}`}, Outcome{Dispatched: true}, nil
	}
	if _, err := s.Run(context.Background(), Call{ArtifactID: "a", ViewerID: owner, Kind: KindComplete, Payload: payload}); code(err) != CodeInvalidOutput {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidPayloadsAreRefusedWithoutAProviderCall(t *testing.T) {
	s, rt, _ := newTestService(t, nil)
	for _, tc := range []struct {
		kind    Kind
		payload string
	}{
		{KindComplete, `{}`},
		{KindComplete, `{"messages":[{"role":"system","content":"x"}]}`},
		{KindComplete, `{"messages":[{"role":"user","content":"x"}],"schema":{"type":"object","patternProperties":{}}}`},
		{KindComplete, `{"messages":[{"role":"user","content":"` + strings.Repeat("x", MaxCompleteTextBytes+1) + `"}]}`},
		{KindComplete, `{"messages":[{"role":"user","content":"x"}],"tools":[]}`},
		{KindDecide, `{"state":"text","questions":{"q":{"type":"bool","instructions":"i","criteria":{"true":"a","false":"b"}}}}`},
		{KindDecide, `{"state":{},"questions":{"q":{"type":"rank","instructions":"i","criteria":[]}}}`},
		{KindDecide, `{"state":{},"questions":{"q":{"type":"choice","instructions":"i","criteria":{"only":"one"}}}}`},
		{KindDecide, `{"state":{},"questions":{"q":{"type":"score","instructions":"i","criteria":["a"]}}}`},
		{KindDecide, `{"state":{"t":"` + strings.Repeat("x", MaxDecideStateBytes) + `"},"questions":{"q":{"type":"bool","instructions":"i","criteria":{"true":"a","false":"b"}}}}`},
		{"summon", `{}`},
	} {
		_, err := s.Run(context.Background(), Call{ArtifactID: "a", ViewerID: owner, Kind: tc.kind, Payload: json.RawMessage(tc.payload)})
		if code(err) != CodeInvalidRequest {
			t.Errorf("%s %.80s = %v", tc.kind, tc.payload, err)
		}
	}
	if rt.calls() != 0 {
		t.Fatalf("invalid payloads reached the provider %d times", rt.calls())
	}
}

func decideCall(viewer int64) Call {
	return Call{ArtifactID: "art-1", ViewerID: viewer, Kind: KindDecide, Payload: json.RawMessage(`{"state":{"text":"x"},"questions":{
		"ok":{"type":"bool","instructions":"Is it ok?","criteria":{"true":"yes","false":"no"}},
		"pick":{"type":"choice","instructions":"Pick","criteria":{"a":"first","b":"second"}},
		"rate":{"type":"score","instructions":"Rate","criteria":["low","mid","high"]}}}`)}
}

func TestDecideReturnsTypedAnswersAndIsMeteredUnderItsOwnKind(t *testing.T) {
	s, rt, led := newTestService(t, nil)
	rt.reply = func(j Job) (Result, Outcome, error) {
		if j.Kind != KindDecide || j.Model != "judge-1" {
			t.Errorf("job = %+v", j)
		}
		return Result{StopReason: "stop", Answers: json.RawMessage(`{
			"ok":{"type":"bool","probability":0.8},
			"pick":{"type":"choice","choice":"b","probabilities":{"a":0.1,"b":0.9},"confidence":0.7},
			"rate":{"type":"score","score":1.4,"confidence":0.6}}`)}, Outcome{Dispatched: true, Usage: &Usage{Input: 5}}, nil
	}
	out, err := s.Run(context.Background(), decideCall(owner))
	if err != nil {
		t.Fatal(err)
	}
	var a map[string]map[string]any
	_ = json.Unmarshal(out, &a)
	if a["ok"]["probability"] != 0.8 || a["pick"]["choice"] != "b" || a["pick"]["confidence"] != 0.7 || a["rate"]["score"] != 1.4 {
		t.Fatalf("answers = %s", out)
	}
	if led.rows[0].Kind != "decide" {
		t.Fatalf("row = %+v", led.rows[0])
	}
	// An answer outside the question's shape is a provider error, metered.
	rt.reply = func(Job) (Result, Outcome, error) {
		return Result{StopReason: "stop", Answers: json.RawMessage(`{"ok":{"probability":0.5},"pick":{"choice":"z","confidence":1,"probabilities":{"a":0.5,"b":0.5}},"rate":{"score":1,"confidence":1}}`)}, Outcome{Dispatched: true}, nil
	}
	if _, err := s.Run(context.Background(), decideCall(owner)); code(err) != CodeProviderError {
		t.Fatalf("bad answer = %v", err)
	}
	if len(led.rows) != 2 {
		t.Fatalf("rows = %d", len(led.rows))
	}
}

func TestDecideRequiresNumericAnswersAndPreservesExplicitZeros(t *testing.T) {
	const (
		boolAnswer   = `{"probability":0}`
		choiceAnswer = `{"choice":"a","confidence":0,"probabilities":{"a":1,"b":0,"extra":0}}`
		scoreAnswer  = `{"score":0,"confidence":0}`
		wantZeros    = `{"ok":{"probability":0,"type":"bool"},"pick":{"choice":"a","confidence":0,"probabilities":{"a":1,"b":0},"type":"choice"},"rate":{"confidence":0,"score":0,"type":"score"}}`
	)
	for _, tc := range []struct {
		name     string
		question string
		answer   string
	}{
		{name: "explicit zeros"},
		{"missing bool probability", "ok", `{}`},
		{"null bool probability", "ok", `{"probability":null}`},
		{"out of range bool probability", "ok", `{"probability":1.1}`},
		{"missing choice confidence", "pick", `{"choice":"a","probabilities":{"a":1,"b":0}}`},
		{"null choice confidence", "pick", `{"choice":"a","confidence":null,"probabilities":{"a":1,"b":0}}`},
		{"missing probability map", "pick", `{"choice":"a","confidence":0}`},
		{"null probability map", "pick", `{"choice":"a","confidence":0,"probabilities":null}`},
		{"missing selected probability", "pick", `{"choice":"a","confidence":0,"probabilities":{"b":0}}`},
		{"missing unselected probability", "pick", `{"choice":"a","confidence":0,"probabilities":{"a":1}}`},
		{"null choice probability", "pick", `{"choice":"a","confidence":0,"probabilities":{"a":1,"b":null}}`},
		{"string choice probability", "pick", `{"choice":"a","confidence":0,"probabilities":{"a":1,"b":"0"}}`},
		{"out of range choice probability", "pick", `{"choice":"a","confidence":0,"probabilities":{"a":1,"b":-0.1}}`},
		{"missing score", "rate", `{"confidence":0}`},
		{"null score", "rate", `{"score":null,"confidence":0}`},
		{"off-scale score", "rate", `{"score":2.1,"confidence":0}`},
		{"missing score confidence", "rate", `{"score":0}`},
		{"null score confidence", "rate", `{"score":0,"confidence":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answers := map[string]json.RawMessage{
				"ok": json.RawMessage(boolAnswer), "pick": json.RawMessage(choiceAnswer), "rate": json.RawMessage(scoreAnswer),
			}
			if tc.question != "" {
				answers[tc.question] = json.RawMessage(tc.answer)
			}
			raw, err := json.Marshal(answers)
			if err != nil {
				t.Fatal(err)
			}
			s, rt, led := newTestService(t, nil)
			rt.reply = func(Job) (Result, Outcome, error) {
				return Result{StopReason: "stop", Answers: raw}, Outcome{
					Dispatched: true,
					Usage:      &Usage{Input: 5, Cost: UsageCost{Total: 0.002}},
				}, nil
			}
			out, err := s.Run(context.Background(), decideCall(recipient))
			if tc.question == "" {
				if err != nil || string(out) != wantZeros {
					t.Errorf("run = %s, %v, want %s", out, err, wantZeros)
				}
			} else {
				var e *Error
				if out != nil || !errors.As(err, &e) || e.Code != CodeProviderError ||
					e.Message != "The AI service returned an answer that did not fit the questions." {
					t.Errorf("run = %s, %v, want the fixed provider error and no output", out, err)
				}
			}
			if rt.calls() != 1 || len(led.rows) != 1 {
				t.Fatalf("provider calls = %d, usage rows = %d, want one each", rt.calls(), len(led.rows))
			}
			row := led.rows[0]
			if row.OwnerID != recipient || row.Kind != "decide" || row.Source != store.UsageSourceInference ||
				row.InputTokens != 5 || row.CostMicros != 2000 {
				t.Errorf("unexpected usage row: %+v", row)
			}
		})
	}
}

func TestDecideIsUnavailableWithoutAClassifierAndNeverFallsBackToChat(t *testing.T) {
	s, rt, _ := newTestService(t, func(c *Config) { c.DecisionModel = "" })
	if a := s.Availability(context.Background(), owner); !a.Complete || a.Decide {
		t.Fatalf("availability = %+v", a)
	}
	if _, err := s.Run(context.Background(), decideCall(owner)); code(err) != CodeUnavailable {
		t.Fatalf("decide = %v", err)
	}
	if rt.calls() != 0 {
		t.Fatal("decide fell back to a provider call")
	}
}

func TestAvailabilityNeverNamesProvidersOrModels(t *testing.T) {
	s, _, _ := newTestService(t, nil)
	a := s.Availability(context.Background(), owner)
	b, _ := json.Marshal(a)
	for _, secret := range []string{"openrouter", "chat-1", "judge-1", "platform-key"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("availability %s names %q", b, secret)
		}
	}
	if !a.Complete || !a.Decide || a.Payer != "platform" {
		t.Fatalf("availability = %+v", a)
	}
	off, _, _ := newTestService(t, func(c *Config) { c.Enabled = false })
	if a := off.Availability(context.Background(), owner); a.Complete || a.Decide || a.Reason != "disabled" {
		t.Fatalf("disabled = %+v", a)
	}
}

func TestGuardrailScreensPlatformCallsAndFailsClosed(t *testing.T) {
	s, rt, led := newTestService(t, func(c *Config) {
		c.Guardrail = &Guardrail{Provider: "openrouter", Model: "jev", APIKey: "guard-key"}
	})
	rt.screenReply = func(ScreenJob) (ScreenVerdict, error) {
		return ScreenVerdict{Usage: &Usage{Input: 3}}, nil
	}
	if _, err := s.Run(context.Background(), completeCall(owner)); code(err) != CodeInvalidRequest {
		t.Fatalf("blocked = %v", err)
	}
	if rt.calls() != 0 || len(led.rows) != 1 || led.rows[0].Source != store.UsageSourceGuardrail {
		t.Fatalf("calls=%d rows=%+v", rt.calls(), led.rows)
	}
	rt.screenReply = func(ScreenJob) (ScreenVerdict, error) {
		return ScreenVerdict{Dispatched: true}, ErrProvider
	}
	if _, err := s.Run(context.Background(), completeCall(owner)); code(err) != CodeProviderError {
		t.Fatalf("screen failure = %v", err)
	}
	if rt.calls() != 0 {
		t.Fatal("a failed screen let the call through")
	}
	if len(led.rows) != 2 || led.rows[1].CostSource != store.CostSourceNone {
		t.Fatalf("failed screen's unknown spend was not recorded: %+v", led.rows)
	}
}

func TestRateLimitsRefuseWithoutSpending(t *testing.T) {
	s, rt, _ := newTestService(t, func(c *Config) { c.Rates = RateConfig{PerViewerPerMinute: 1, Burst: 2} })
	for i := 0; i < 2; i++ {
		if _, err := s.Run(context.Background(), completeCall(owner)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Run(context.Background(), completeCall(owner))
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeRateLimited || e.RetryAfter <= 0 {
		t.Fatalf("third call = %v", err)
	}
	if rt.calls() != 2 {
		t.Fatalf("calls = %d", rt.calls())
	}
}

func TestStartupRequiresAChatModelInPlatformMode(t *testing.T) {
	s, _, _ := newTestService(t, func(c *Config) { c.ChatModel = "" })
	if err := s.CheckStartup(context.Background()); err == nil || !strings.Contains(err.Error(), "INFERENCE_MODEL") {
		t.Fatalf("err = %v", err)
	}
	s, _, _ = newTestService(t, func(c *Config) { c.DecisionModel = "chat-1" })
	if err := s.CheckStartup(context.Background()); err == nil || !strings.Contains(err.Error(), "classifier") {
		t.Fatalf("err = %v", err)
	}
	s, _, _ = newTestService(t, nil)
	if err := s.CheckStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Buckets that have refilled are forgotten, so the limiter's memory tracks
// recent callers rather than everyone who ever called.
func TestRateLimiterForgetsIdleBuckets(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := newRateLimiter(RateConfig{}, func() time.Time { return now })
	for i := range 50 {
		release, err := r.admit(int64(i), fmt.Sprintf("a%d", i), KindComplete)
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if len(r.viewers) != 50 || len(r.artifacts) != 50 {
		t.Fatalf("buckets: %d viewers, %d artifacts", len(r.viewers), len(r.artifacts))
	}
	now = now.Add(5 * time.Minute)
	release, err := r.admit(999, "fresh", KindComplete)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if len(r.viewers) != 1 || len(r.artifacts) != 1 {
		t.Fatalf("after idling: %d viewers, %d artifacts", len(r.viewers), len(r.artifacts))
	}
}

// Every paid call checks the allowance first, so a first answer that used the
// last of it cannot be followed by a paid repair.
func TestRepairIsRefusedOnceTheAllowanceIsSpent(t *testing.T) {
	cap := int64(1000) // micro-dollars; the fake's first answer costs exactly this
	s, rt, _ := newTestService(t, func(c *Config) { c.Caps.OwnerMicrosPerMonth = &cap })
	payload := json.RawMessage(`{"messages":[{"role":"user","content":"Tag it"}],"schema":{"type":"object","properties":{"tags":{"type":"array"}},"required":["tags"]}}`)
	rt.reply = func(Job) (Result, Outcome, error) {
		return Result{StopReason: "stop", Text: "not json"}, Outcome{Dispatched: true, Usage: &Usage{Input: 5, Cost: UsageCost{Total: 0.001}}}, nil
	}
	_, err := s.Run(context.Background(), Call{ArtifactID: "a", ViewerID: owner, Kind: KindComplete, Payload: payload})
	if code(err) != CodeLimitReached {
		t.Fatalf("err = %v, want limit_reached before the repair", err)
	}
	if rt.calls() != 1 {
		t.Fatalf("provider calls = %d, want 1: the repair must not start past the allowance", rt.calls())
	}
}

// A usage report of all zeros is Pi's placeholder for a call that failed
// before the provider reported: unknown spend, never a known zero. A cost the
// provider reported itself keeps that provenance.
func TestUsageDecodingKeepsUnknownAndProviderCostApart(t *testing.T) {
	if u := decodeUsage(json.RawMessage(`{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"cost":{"total":0}}`)); u != nil {
		t.Fatalf("zero-filled usage = %+v, want nil (unknown)", u)
	}
	u := decodeUsage(json.RawMessage(`{"input":12,"output":3,"cost":{"total":0.0004,"totalFromProvider":true}}`))
	if u == nil || !u.Cost.TotalFromProvider || u.Input != 12 {
		t.Fatalf("usage = %+v, want provider-reported cost", u)
	}
	if u := decodeUsage(json.RawMessage(`{"input":12,"output":3,"cost":{"total":0.0004}}`)); u == nil || u.Cost.TotalFromProvider {
		t.Fatalf("usage = %+v, want a catalog estimate", u)
	}
	if u := decodeUsage(json.RawMessage(`{"cost":{"total":0,"totalFromProvider":true}}`)); u == nil || u.Cost != (UsageCost{TotalFromProvider: true}) {
		t.Fatalf("usage = %+v, want a provider-reported zero cost", u)
	}
}
