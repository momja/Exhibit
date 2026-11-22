package inference

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
)

func configureTestGuardrail(cfg *Config) {
	cfg.Guardrail = &Guardrail{Provider: "openrouter", Model: "guard-1", APIKey: "guard-key"}
}

func structuredColorCall() Call {
	call := completeCall(recipient)
	call.Payload = json.RawMessage(`{
		"system":"Use the paint catalog.",
		"messages":[{"role":"assistant","content":"The brush is ready."},{"role":"user","content":"Suggest a color."}],
		"schema":{"type":"object","required":["shade"],"description":"A watercolor shade."}
	}`)
	return call
}

func TestGuardrailScreensStructuredRepairBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		allow     bool
		screenErr error
		wantCode  string
	}{
		{name: "allowed", allow: true},
		{name: "refused", wantCode: CodeInvalidRequest},
		{name: "failed", screenErr: ErrProvider, wantCode: CodeProviderError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, rt, ledger := newTestService(t, configureTestGuardrail)
			call := structuredColorCall()
			const firstAnswer = `{"color":"blue"}`
			const repairedAnswer = `{"shade":"blue"}`
			rt.reply = func(job Job) (Result, Outcome, error) {
				attempt := rt.calls()
				if len(rt.screens) != attempt || string(rt.screens[attempt-1].Context) != string(job.Context) {
					t.Fatal("provider job was dispatched without screening its context first")
				}
				answer := firstAnswer
				if attempt == 2 {
					answer = repairedAnswer
				}
				return Result{Text: answer}, Outcome{Dispatched: true}, nil
			}
			rt.screenReply = func(ScreenJob) (ScreenVerdict, error) {
				if len(rt.screens) == 1 {
					return ScreenVerdict{Allowed: true, Dispatched: true, Usage: &Usage{Input: 3}}, nil
				}
				return ScreenVerdict{Allowed: tc.allow, Dispatched: true}, tc.screenErr
			}
			out, err := s.Run(context.Background(), call)
			if tc.allow && (err != nil || string(out) != repairedAnswer) || !tc.allow && code(err) != tc.wantCode {
				t.Fatalf("repair = %s, %v", out, err)
			}
			if len(rt.screens) != 2 {
				t.Fatalf("screens = %d, want 2", len(rt.screens))
			}
			var repair struct {
				SystemPrompt string `json:"systemPrompt"`
				Messages     []struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(rt.screens[1].Context, &repair); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(repair.SystemPrompt, "Use the paint catalog.") ||
				!strings.Contains(repair.SystemPrompt, "A watercolor shade.") || len(repair.Messages) != 4 {
				t.Fatalf("incomplete repair context: %+v", repair)
			}
			if reply, _ := json.Marshal([]map[string]string{{"type": "text", "text": firstAnswer}}); repair.Messages[2].Role != "assistant" ||
				string(repair.Messages[2].Content) != string(reply) || repair.Messages[3].Role != "user" ||
				!strings.Contains(string(repair.Messages[3].Content), `value is missing \"shade\"`) {
				t.Fatalf("repair screen missed the first answer or correction: %s", rt.screens[1].Context)
			}

			wantSources := []string{store.UsageSourceGuardrail, store.UsageSourceInference, store.UsageSourceGuardrail}
			wantCalls := 1
			if tc.allow {
				wantSources = append(wantSources, store.UsageSourceInference)
				wantCalls++
			}
			if rt.calls() != wantCalls || len(ledger.rows) != len(wantSources) {
				t.Fatalf("provider calls = %d, rows = %+v", rt.calls(), ledger.rows)
			}
			for i, row := range ledger.rows {
				if row.Source != wantSources[i] || row.PaidBy != store.PaidByPlatform || row.OwnerID != call.ViewerID ||
					row.ArtifactID != call.ArtifactID || row.SessionID != ledger.rows[0].SessionID {
					t.Errorf("row %d = %+v", i, row)
				}
			}
			if ledger.rows[0].InputTokens != 3 || ledger.rows[2].CostSource != store.CostSourceNone {
				t.Fatalf("screen usage = %+v and %+v", ledger.rows[0], ledger.rows[2])
			}
		})
	}
}

func TestSpendCapsGateEveryGuardrailAndInferenceDispatch(t *testing.T) {
	for _, limit := range []string{"owner", "instance"} {
		for _, tc := range []struct {
			spentAt   string
			wantOrder []string
		}{
			{spentAt: "prior spend"},
			{spentAt: "initial screen", wantOrder: []string{"initial screen"}},
			{spentAt: "first answer", wantOrder: []string{"initial screen", "first answer"}},
			{spentAt: "repair screen", wantOrder: []string{"initial screen", "first answer", "repair screen"}},
		} {
			t.Run(limit+"/"+tc.spentAt, func(t *testing.T) {
				cap := int64(1000)
				s, rt, ledger := newTestService(t, func(cfg *Config) {
					configureTestGuardrail(cfg)
					if limit == "owner" {
						cfg.Caps.OwnerMicrosPerMonth = &cap
					} else {
						cfg.Caps.InstanceMicrosPerMonth = &cap
					}
				})
				call := structuredColorCall()
				if tc.spentAt == "prior spend" {
					ledger.rows = append(ledger.rows, store.AgentUsage{OwnerID: call.ViewerID, PaidBy: store.PaidByPlatform, CostMicros: cap})
				}
				var order []string
				usageAt := func(stage string) *Usage {
					order = append(order, stage)
					if stage == tc.spentAt {
						return &Usage{Cost: UsageCost{Total: 0.001}}
					}
					return nil
				}
				rt.screenReply = func(ScreenJob) (ScreenVerdict, error) {
					stage := "initial screen"
					if len(rt.screens) > 1 {
						stage = "repair screen"
					}
					return ScreenVerdict{Allowed: true, Dispatched: true, Usage: usageAt(stage)}, nil
				}
				rt.reply = func(Job) (Result, Outcome, error) {
					return Result{Text: `{"color":"blue"}`}, Outcome{Dispatched: true, Usage: usageAt("first answer")}, nil
				}
				_, err := s.Run(context.Background(), call)
				var refusal *Error
				if !errors.As(err, &refusal) || refusal.Code != CodeLimitReached || refusal.Limit != limit {
					t.Fatalf("cap refusal = %v, want limit_reached for %s", err, limit)
				}
				if !slices.Equal(order, tc.wantOrder) {
					t.Fatalf("dispatches = %v, want %v", order, tc.wantOrder)
				}
			})
		}
	}
}
