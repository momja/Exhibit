package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/mockllm"
	"github.com/momja/Exhibit/internal/store"
)

// These tests run the real Pi worker: the pinned binary, ext/inference.ts,
// Pi's model registry and its OpenRouter adapters, against internal/mockllm.
// They need a Pi executable (PI_BIN, else `pi` on PATH) and skip without one.
// What they cannot prove is live provider compatibility, which has only been
// checked by hand for complete; see docs/inference.md.

const (
	testChatModel     = "openai/gpt-4.1-nano"
	testDecisionModel = "typesafe/jev-1.14"
)

func piForTest(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("PI_BIN")
	if bin == "" {
		bin = "pi"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skip("no Pi executable; set PI_BIN to the pinned Pi Bolt binary")
	}
	return path
}

// mockProvider wraps mockllm, recording each request's key and counting
// in-flight requests.
type mockProvider struct {
	*httptest.Server
	mu    sync.Mutex
	calls int
	keys  []string
	// sessions is each request's X-Session-Id, which Pi derives from the
	// call's sessionId option (OpenCode Go's x-opencode-session likewise).
	sessions []string
	bodies   []string
	leaks    atomic.Int32
	blocker  chan struct{}
}

func newMockProvider(t *testing.T) *mockProvider {
	m := &mockProvider{}
	h := mockllm.Handler()
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		auth := r.Header.Get("Authorization")
		text := string(body)
		if strings.Contains(text, "tenant-A") && auth != "Bearer key-A" || strings.Contains(text, "tenant-B") && auth != "Bearer key-B" {
			m.leaks.Add(1)
		}
		m.mu.Lock()
		m.calls++
		m.keys = append(m.keys, auth)
		m.sessions = append(m.sessions, r.Header.Get("X-Session-Id"))
		m.bodies = append(m.bodies, text)
		m.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *mockProvider) count() int { m.mu.Lock(); defer m.mu.Unlock(); return m.calls }

func startTestWorker(t *testing.T, mockURL string) *Worker {
	t.Helper()
	w, err := NewWorker(WorkerConfig{PiBin: piForTest(t), WorkDir: t.TempDir(), MockURL: mockURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	return w
}

func deadline(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestWorkerResolvesCatalogModels(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	for _, tc := range []struct {
		kind  Kind
		model string
		want  bool
	}{
		{KindComplete, testChatModel, true},
		{KindDecide, testDecisionModel, true},
		{KindDecide, testChatModel, false}, // a chat model is never a classifier
		{KindComplete, "no/such-model", false},
	} {
		got, err := w.Resolve(deadline(t, 10*time.Second), "openrouter", tc.kind, tc.model)
		if err != nil || got != tc.want {
			t.Errorf("Resolve(%s, %s) = %v, %v; want %v", tc.kind, tc.model, got, err, tc.want)
		}
	}
	if m.count() != 0 {
		t.Fatalf("resolving reached the provider %d times", m.count())
	}
}

func completeJob(key, text string) Job {
	req := &completeRequest{Messages: []completeMessage{{Role: "user", Content: text}}, MaxTokens: 64}
	return Job{Kind: KindComplete, Provider: "openrouter", Model: testChatModel, APIKey: key, Context: req.context(), MaxTokens: 64}
}

func TestWorkerCompleteRoundTripReportsUsage(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	res, out, err := w.Run(deadline(t, 20*time.Second), completeJob("key-A", "Summarize this note please"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(res.Text, "Summary: Summarize this note") || res.StopReason != "stop" {
		t.Fatalf("result = %+v", res)
	}
	if !out.Dispatched || out.Usage == nil || out.Usage.Input != 100 {
		t.Fatalf("outcome = %+v usage=%+v", out, out.Usage)
	}
	if m.keys[0] != "Bearer key-A" {
		t.Fatalf("provider saw %q", m.keys[0])
	}
}

// OpenCode Go answers 400 MissingSessionID to a request with no session id, and
// Pi sets that header only from options.sessionId, which a direct registry call
// never has unless the extension supplies it. The first live check failed on
// exactly this while every mock-backed test passed. The id must also be fresh
// per call, so it never ties two viewers' or artifacts' calls together.
func TestWorkerSendsEachProviderCallItsOwnSessionID(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	for _, key := range []string{"key-A", "key-B"} {
		if _, _, err := w.Run(deadline(t, 20*time.Second), completeJob(key, "Summarize this note please")); err != nil {
			t.Fatalf("run %s: %v", key, err)
		}
	}
	if len(m.sessions) != 2 || m.sessions[0] == "" || m.sessions[1] == "" {
		t.Fatalf("provider requests carried session ids %q; every call needs one", m.sessions)
	}
	if m.sessions[0] == m.sessions[1] {
		t.Fatalf("two calls shared session id %q", m.sessions[0])
	}
}

func TestWorkerScreensThroughMockAndPreservesUsageOnFailure(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	for _, tc := range []struct {
		model   string
		text    string
		wantErr bool
	}{
		{model: testChatModel, text: "Summarize a calendar event"},
		{model: testDecisionModel, text: "Summarize a calendar event"},
		{model: testChatModel, text: "mock-guard-garbage", wantErr: true},
	} {
		before := m.count()
		verdict, err := w.Screen(deadline(t, 20*time.Second), ScreenJob{
			Provider: "openrouter", Model: tc.model, APIKey: "test-guard-key", Context: completeJob("", tc.text).Context,
		})
		if (err != nil) != tc.wantErr || !verdict.Dispatched || verdict.Usage == nil || m.count() != before+1 {
			t.Errorf("screen %s (%q) = %+v, %v, %d mock requests; want usage and one mock request",
				tc.model, tc.text, verdict, err, m.count()-before)
		}
	}
}

func TestWorkerArtifactGuardrailFailsClosedWithoutRetrying(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = io.WriteString(w, `{"answers":{"violates":{"type":"bool","probability":-0.1}},"usage":{"input_tokens":100}}`)
				} else {
					_, _ = io.WriteString(w, `{"error":{"message":"Mock rate limit"}}`)
				}
			}))
			defer server.Close()
			worker := startTestWorker(t, server.URL)
			verdict, err := worker.Screen(deadline(t, 10*time.Second), ScreenJob{
				Provider: "openrouter", Model: testDecisionModel, APIKey: "test-guard-key",
				Context: completeJob("", "Suggest a color.").Context,
			})
			if err == nil || verdict.Allowed || !verdict.Dispatched || requests.Load() != 1 {
				t.Fatalf("screen verdict = %+v, err = %v, mock requests = %d", verdict, err, requests.Load())
			}
		})
	}
}

func TestWorkerArtifactGuardrailScreensFullContext(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	for _, tc := range []struct {
		name         string
		kind         Kind
		payload      string
		wantCode     string
		wantKeys     []string
		marker       string
		markerScreen int
	}{
		{
			name: "completion schema", kind: KindComplete,
			payload:  `{"prompt":"Suggest a color.","schema":{"type":"string","description":"mock-policy-violation"}}`,
			wantCode: CodeInvalidRequest, wantKeys: []string{"Bearer guard-key"}, marker: "mock-policy-violation",
		},
		{
			name: "classifier criteria", kind: KindDecide,
			payload:  `{"state":{"color":"blue"},"questions":{"blue":{"type":"bool","instructions":"Is the paint blue?","criteria":{"true":"mock-policy-violation","false":"A different shade"}}}}`,
			wantCode: CodeInvalidRequest, wantKeys: []string{"Bearer guard-key"}, marker: "mock-policy-violation",
		},
		{
			// The mock only fills properties declared in the schema. It
			// omits shade on both calls and triggers one repair.
			name: "structured repair", kind: KindComplete, payload: string(structuredColorCall().Payload),
			wantCode: CodeInvalidOutput, wantKeys: []string{"Bearer guard-key", "Bearer platform-key", "Bearer guard-key", "Bearer platform-key"},
			marker: "did not match the required JSON Schema", markerScreen: 2,
		},
		{
			// Go escapes ampersands in JSON. The escaped context fits in the
			// worker's envelope, but wrapping it in a JSON string would not.
			name: "large escaped context", kind: KindComplete,
			payload:  `{"prompt":"` + strings.Repeat("&", 40<<10) + ` End of color sample."}`,
			wantKeys: []string{"Bearer guard-key", "Bearer platform-key"}, marker: "End of color sample.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, ledger := newTestService(t, func(cfg *Config) {
				configureTestGuardrail(cfg)
				cfg.Guardrail.Model = testChatModel
				cfg.ChatModel, cfg.DecisionModel, cfg.Runtime = testChatModel, testDecisionModel, w
			})
			before := m.count()
			_, err := s.Run(context.Background(), Call{
				ArtifactID: "art-1", ViewerID: recipient, Kind: tc.kind, Payload: json.RawMessage(tc.payload),
			})
			if code(err) != tc.wantCode || tc.wantCode == "" && err != nil {
				t.Fatalf("Run: %v, want %s", err, tc.wantCode)
			}
			m.mu.Lock()
			keys, bodies := slices.Clone(m.keys[before:]), slices.Clone(m.bodies[before:])
			m.mu.Unlock()
			if !slices.Equal(keys, tc.wantKeys) || len(ledger.rows) != len(tc.wantKeys) {
				t.Fatalf("provider keys = %v, ledger rows = %+v", keys, ledger.rows)
			}
			if !strings.Contains(bodies[tc.markerScreen], tc.marker) {
				t.Fatalf("guard request is missing %q", tc.marker)
			}
			for i, key := range keys {
				if row := ledger.rows[i]; key == "Bearer guard-key" && (row.Source != store.UsageSourceGuardrail || row.InputTokens == 0) {
					t.Errorf("guardrail usage = %+v", row)
				}
			}
		})
	}
}

func TestWorkerClassifyRoundTrip(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	p, err := prepareDecide(json.RawMessage(`{"state":{"text":"The server is down and customers cannot log in"},"questions":{
		"urgent":{"type":"bool","instructions":"Is this urgent?","criteria":{"true":"Server down, customers blocked","false":"Routine question"}},
		"area":{"type":"choice","instructions":"Which area?","criteria":{"billing":"Invoices and payments","outage":"Server down or unreachable"}},
		"severity":{"type":"score","instructions":"How severe?","criteria":["cosmetic","annoying","customers blocked"]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var got json.RawMessage
	meter := func(o Outcome) {
		if !o.Dispatched || o.Usage == nil || o.Usage.Input != 100 {
			t.Errorf("outcome = %+v", o)
		}
	}
	got, err = runDecide(deadline(t, 20*time.Second), w, Job{Kind: KindDecide, Provider: "openrouter", Model: testDecisionModel, APIKey: "key-A"}, p, meter, nil)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	var answers map[string]map[string]any
	_ = json.Unmarshal(got, &answers)
	if answers["urgent"]["probability"].(float64) < 0.5 || answers["area"]["choice"] != "outage" || answers["severity"]["score"].(float64) != 2 {
		t.Fatalf("answers = %s", got)
	}
}

// Overlapping calls with different keys each reach the provider with their
// own key, and nothing is shared between them.
func TestWorkerConcurrentCallsKeepTheirOwnKeys(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key, tenant := "key-A", "tenant-A"
			if i%2 == 1 {
				key, tenant = "key-B", "tenant-B"
			}
			res, _, err := w.Run(deadline(t, 30*time.Second), completeJob(key, "mock-inference reply "+tenant))
			if err == nil && res.Text != "OK "+tenant {
				err = errors.New("wrong answer " + res.Text)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if m.leaks.Load() != 0 {
		t.Fatalf("%d requests carried another caller's key", m.leaks.Load())
	}
}

func TestWorkerProviderErrorIsRedactedAndMetered(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	res, out, err := w.Run(deadline(t, 20*time.Second), completeJob("key-A", "mock-inference-http-error"))
	if err == nil && res.StopReason == "stop" {
		t.Fatalf("an upstream 401 succeeded: %+v", res)
	}
	if !out.Dispatched {
		t.Fatal("a call that reached the provider was not marked dispatched")
	}
	if strings.Contains(res.Text, "test-key-A") {
		t.Fatal("upstream error text reached the result")
	}
}

func TestWorkerCancelsOneCallIndependently(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	slow, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := w.Run(slow, completeJob("key-A", "mock-inference-slow"))
		done <- err
	}()
	res, _, err := w.Run(deadline(t, 20*time.Second), completeJob("key-B", "mock-inference tenant-B"))
	if err != nil || res.Text != "OK tenant-B" {
		t.Fatalf("concurrent call: %v %+v", err, res)
	}
	if err := <-done; !errors.Is(err, ErrProvider) {
		t.Fatalf("canceled call: %v", err)
	}
	// The worker survived the cancel.
	if _, _, err := w.Run(deadline(t, 20*time.Second), completeJob("key-A", "mock-inference again")); err != nil {
		t.Fatalf("after cancel: %v", err)
	}
}

func TestWorkerRestartsAfterDeathWithoutReplay(t *testing.T) {
	m := newMockProvider(t)
	w := startTestWorker(t, m.URL)
	w.mu.Lock()
	g := w.gen
	w.mu.Unlock()
	g.fail()
	<-g.done
	// Let watch() observe the death; the next call starts a new generation
	// after the first backoff.
	time.Sleep(700 * time.Millisecond)
	res, _, err := w.Run(deadline(t, 60*time.Second), completeJob("key-A", "mock-inference tenant-A"))
	if err != nil || res.Text != "OK tenant-A" {
		t.Fatalf("after restart: %v %+v", err, res)
	}
	if m.count() != 1 {
		t.Fatalf("provider saw %d calls; a restart must not replay anything", m.count())
	}
}

func TestWorkerAdmissionIsBounded(t *testing.T) {
	w := &Worker{slots: make(chan struct{}, 1)}
	release, err := w.admit()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.admit(); !errors.Is(err, ErrRuntimeBusy) {
		t.Fatalf("second admit = %v", err)
	}
	release()
	if _, err := w.admit(); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

func TestWorkerCircuitOpensAfterRepeatedStartFailures(t *testing.T) {
	w, err := NewWorker(WorkerConfig{PiBin: "/nonexistent/pi", WorkDir: t.TempDir(), Backoff: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxConsecutiveFailures; i++ {
		_ = w.Start(context.Background())
		w.mu.Lock()
		if i < maxConsecutiveFailures-1 {
			w.nextStartAt = time.Time{}
		}
		w.mu.Unlock()
	}
	w.mu.Lock()
	open := time.Until(w.nextStartAt)
	w.mu.Unlock()
	if open < circuitOpenFor-5*time.Second {
		t.Fatalf("circuit open for %v after %d failures", open, maxConsecutiveFailures)
	}
	if _, _, err := w.Run(deadline(t, time.Second), Job{Kind: KindComplete}); !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("run with circuit open = %v", err)
	}
}
