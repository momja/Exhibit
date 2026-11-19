package inference

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/momja/Exhibit/internal/agent"
)

//go:embed ext/inference.ts
var extensionSource []byte

// The Pi worker (Exh-urrg's decision). One warm `pi --mode rpc` process,
// loaded with ext/inference.ts and nothing else, serves every artifact AI call.
// It is deliberately separate from agent.Manager: inference calls are short,
// independent and many, and must neither consume nor be refused by the agent's
// per-owner conversation slots.
//
// Supervision rules, each of which a test pins:
//   - A call carries its own key and deadline; the worker never holds a key.
//   - A call in flight when the worker dies is reported as an unknown outcome
//     (ErrProvider, Dispatched) and never replayed: it may already have been
//     billed, and running it again could bill twice.
//   - A per-call cancel goes to the extension; a cancel the worker does not
//     honor within cancelGrace kills the whole generation (a wedged worker
//     fails closed rather than holding admission slots forever).
//   - Restarts back off exponentially; after maxConsecutiveFailures the
//     circuit opens for circuitOpenFor and calls fail fast as unavailable.
//   - Admission is bounded (MaxActive); a full worker answers busy at once.
//   - IO is bounded: an envelope over maxEnvelope is refused before sending;
//     a reply line over maxReplyLine kills the generation.

const (
	// MaxActive bounds concurrent calls per worker; it matches MAX_ACTIVE in
	// ext/inference.ts.
	MaxActive              = 16
	maxEnvelope            = 262_144
	maxReplyLine           = 4 << 20
	cancelGrace            = 2 * time.Second
	readyTimeout           = 30 * time.Second
	maxConsecutiveFailures = 5
	circuitOpenFor         = 60 * time.Second
	signalPrefix           = "exhibit_inference:"
)

// WorkerConfig configures the Pi worker.
type WorkerConfig struct {
	// PiBin is the pinned Pi Bolt executable (PI_BIN).
	PiBin string
	// WorkDir holds the materialized extension and each generation's
	// scratch home. Created if missing.
	WorkDir string
	// MockURL, when set, sends every provider request to this server
	// (INFERENCE_MOCK_URL; tests and demos only).
	MockURL string
	// Backoff is the first restart delay; zero is 500ms.
	Backoff time.Duration
}

// Worker is the supervised Pi process, implementing Runtime.
type Worker struct {
	cfg     WorkerConfig
	extPath string
	slots   chan struct{}
	seq     atomic.Uint64

	mu          sync.Mutex
	gen         *generation
	starting    chan struct{} // closed when an in-progress start finishes
	failures    int
	nextStartAt time.Time
	closed      bool
}

// NewWorker materializes the extension and returns an unstarted worker.
// Start warms it; a call also starts it on demand.
func NewWorker(cfg WorkerConfig) (*Worker, error) {
	if cfg.Backoff <= 0 {
		cfg.Backoff = 500 * time.Millisecond
	}
	extDir := filepath.Join(cfg.WorkDir, "ext")
	if err := agent.MaterializeGuard(extDir); err != nil {
		return nil, fmt.Errorf("materialize inference guard: %w", err)
	}
	extPath := filepath.Join(extDir, "inference.ts")
	if err := os.WriteFile(extPath, extensionSource, 0o644); err != nil {
		return nil, fmt.Errorf("materialize inference extension: %w", err)
	}
	return &Worker{cfg: cfg, extPath: extPath, slots: make(chan struct{}, MaxActive)}, nil
}

// Start brings the worker up and waits for it to answer. An error leaves the
// worker usable: the next call tries again, subject to backoff.
func (w *Worker) Start(ctx context.Context) error {
	_, err := w.current(ctx)
	return err
}

// Close stops the worker for good.
func (w *Worker) Close() {
	w.mu.Lock()
	w.closed = true
	g := w.gen
	w.gen = nil
	w.mu.Unlock()
	if g != nil {
		g.fail()
	}
}

// current returns a live generation, starting one if needed. Concurrent
// callers share one start.
func (w *Worker) current(ctx context.Context) (*generation, error) {
	for {
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return nil, ErrRuntimeUnavailable
		}
		if g := w.gen; g != nil && !g.dead() {
			w.mu.Unlock()
			return g, nil
		}
		if ch := w.starting; ch != nil {
			w.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return nil, ErrRuntimeUnavailable
			}
		}
		if now := time.Now(); now.Before(w.nextStartAt) {
			w.mu.Unlock()
			return nil, ErrRuntimeUnavailable
		}
		ch := make(chan struct{})
		w.starting = ch
		w.mu.Unlock()

		startCtx, cancel := context.WithTimeout(context.Background(), readyTimeout)
		g, err := w.spawn(startCtx)
		cancel()

		w.mu.Lock()
		w.starting = nil
		close(ch)
		if err != nil {
			w.noteFailureLocked()
			w.mu.Unlock()
			slog.Error("inference worker failed to start", slog.String("err", err.Error()))
			return nil, ErrRuntimeUnavailable
		}
		w.gen = g
		w.mu.Unlock()
		go w.watch(g)
		return g, nil
	}
}

// noteFailureLocked records a failed generation: exponential backoff, and
// the circuit opens after too many in a row.
func (w *Worker) noteFailureLocked() {
	w.failures++
	delay := w.cfg.Backoff << min(w.failures-1, 6)
	if w.failures >= maxConsecutiveFailures {
		delay = circuitOpenFor
		slog.Error("inference worker circuit open", slog.Int("consecutive_failures", w.failures))
	}
	w.nextStartAt = time.Now().Add(delay)
}

// watch waits for a generation to end. A generation that served calls and
// then died is a failure for backoff purposes only if it died young.
func (w *Worker) watch(g *generation) {
	<-g.done
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.gen == g {
		w.gen = nil
	}
	if w.closed {
		return
	}
	if time.Since(g.started) < 30*time.Second {
		w.noteFailureLocked()
	} else {
		w.failures = 0
	}
	slog.Warn("inference worker exited; the next call starts a new one")
}

// spawn starts one generation and waits until it answers a ping.
func (w *Worker) spawn(ctx context.Context) (*generation, error) {
	home, err := os.MkdirTemp(w.cfg.WorkDir, "gen-")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(w.cfg.PiBin, "--mode", "rpc", "--offline", "--no-session", "--no-tools",
		"--no-extensions", "--no-mcp", "--no-skills", "--no-prompt-templates", "--no-themes",
		"--no-context-files", "--no-approve", "-e", w.extPath)
	cmd.Dir = home
	// No inherited provider keys, auth store or user extensions: HOME is the
	// generation's own scratch directory.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "LANG=" + os.Getenv("LANG"),
		"TMPDIR=" + os.TempDir(), "PI_CODING_AGENT_DIR=" + filepath.Join(home, "agent"), "PI_OFFLINE=1"}
	if w.cfg.MockURL != "" {
		cmd.Env = append(cmd.Env, "EXHIBIT_INFERENCE_MOCK_URL="+w.cfg.MockURL)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		os.RemoveAll(home)
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(home)
		return nil, err
	}
	// Pi's stderr can carry provider error text; it is not copied to logs.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		os.RemoveAll(home)
		return nil, err
	}
	g := &generation{cmd: cmd, stdin: stdin, started: time.Now(), home: home,
		queue: make(chan []byte, MaxActive*2), done: make(chan struct{}), pending: map[string]chan reply{}}
	go g.writeLoop()
	go g.readLoop(stdout)
	pctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if _, err := g.call(pctx, envelope{ID: w.nextID(), Op: "ping"}); err != nil {
		g.fail()
		return nil, fmt.Errorf("worker readiness: %w", err)
	}
	w.mu.Lock()
	w.failures = 0
	w.mu.Unlock()
	return g, nil
}

func (w *Worker) nextID() string { return fmt.Sprint(w.seq.Add(1)) }

// admit takes an admission slot or refuses at once.
func (w *Worker) admit() (func(), error) {
	select {
	case w.slots <- struct{}{}:
		return func() { <-w.slots }, nil
	default:
		return nil, ErrRuntimeBusy
	}
}

// Resolve implements Runtime.
func (w *Worker) Resolve(ctx context.Context, provider string, kind Kind, model string) (bool, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	g, err := w.current(ctx)
	if err != nil {
		return false, err
	}
	r, err := g.call(ctx, envelope{ID: w.nextID(), Op: "resolve", Provider: provider, Kind: string(kind), Model: model})
	if err != nil {
		return false, err
	}
	var res struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(r.Result, &res) != nil {
		return false, ErrProvider
	}
	return res.OK, nil
}

// Run implements Runtime.
func (w *Worker) Run(ctx context.Context, job Job) (Result, Outcome, error) {
	if _, ok := ctx.Deadline(); !ok {
		return Result{}, Outcome{}, errors.New("inference: a call needs a wall-clock deadline")
	}
	release, err := w.admit()
	if err != nil {
		return Result{}, Outcome{}, err
	}
	defer release()
	g, err := w.current(ctx)
	if err != nil {
		return Result{}, Outcome{}, err
	}
	op := "complete"
	if job.Kind == KindDecide {
		op = "classify"
	}
	env := envelope{ID: w.nextID(), Op: op, Provider: job.Provider, Model: job.Model, APIKey: job.APIKey,
		Context: job.Context, MaxTokens: job.MaxTokens}
	r, err := g.call(ctx, env)
	out := Outcome{Dispatched: true, Usage: decodeUsage(r.Usage)}
	switch {
	case errors.Is(err, errNotSent):
		return Result{}, Outcome{}, ErrRuntimeBusy
	case err != nil:
		return Result{}, out, ErrProvider
	}
	switch r.Error {
	case "":
	case "unavailable":
		return Result{}, Outcome{}, ErrNoModel
	case "busy":
		return Result{}, Outcome{}, ErrRuntimeBusy
	case "invalid_request":
		return Result{}, Outcome{}, fmt.Errorf("%w: worker refused the envelope", ErrProvider)
	default:
		return Result{}, out, ErrProvider
	}
	var res struct {
		StopReason string          `json:"stopReason"`
		Text       string          `json:"text"`
		Answers    json.RawMessage `json:"answers"`
	}
	if json.Unmarshal(r.Result, &res) != nil {
		return Result{}, out, ErrProvider
	}
	return Result{StopReason: res.StopReason, Text: res.Text, Answers: res.Answers}, out, nil
}

// Screen implements Runtime.
func (w *Worker) Screen(ctx context.Context, job ScreenJob) (ScreenVerdict, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}
	release, err := w.admit()
	if err != nil {
		return ScreenVerdict{}, err
	}
	defer release()
	g, err := w.current(ctx)
	if err != nil {
		return ScreenVerdict{}, err
	}
	r, err := g.call(ctx, envelope{ID: w.nextID(), Op: "screen", Provider: job.Provider, Model: job.Model, APIKey: job.APIKey, Text: job.Text})
	v := ScreenVerdict{Usage: decodeUsage(r.Usage)}
	if err != nil {
		return v, ErrProvider
	}
	if r.Error != "" {
		return v, fmt.Errorf("%w: screen %s", ErrProvider, r.Error)
	}
	var res struct {
		Allowed bool `json:"allowed"`
	}
	if json.Unmarshal(r.Result, &res) != nil {
		return v, ErrProvider
	}
	v.Allowed = res.Allowed
	return v, nil
}

// envelope is one request line, carried as an RPC prompt's message.
type envelope struct {
	ID        string          `json:"id"`
	Op        string          `json:"op"`
	Provider  string          `json:"provider,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	Model     string          `json:"model,omitempty"`
	APIKey    string          `json:"apiKey,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
	MaxTokens int             `json:"maxTokens,omitempty"`
	Text      string          `json:"text,omitempty"`
}

type reply struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
	Usage  json.RawMessage `json:"usage"`
}

// errNotSent: the envelope never left the host, so nothing was dispatched.
var errNotSent = errors.New("not sent")

// generation is one Pi process.
type generation struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	home    string
	started time.Time
	queue   chan []byte
	done    chan struct{}
	once    sync.Once

	mu      sync.Mutex
	pending map[string]chan reply
}

func (g *generation) dead() bool {
	select {
	case <-g.done:
		return true
	default:
		return false
	}
}

func (g *generation) fail() {
	g.once.Do(func() {
		close(g.done)
		if g.cmd.Process != nil {
			_ = g.cmd.Process.Kill()
		}
		_ = g.stdin.Close()
	})
}

func (g *generation) writeLoop() {
	for {
		select {
		case b := <-g.queue:
			if _, err := g.stdin.Write(b); err != nil {
				g.fail()
				return
			}
		case <-g.done:
			return
		}
	}
}

func (g *generation) readLoop(stdout io.Reader) {
	defer func() {
		g.fail()
		_ = g.cmd.Wait() // reap exactly once, after stdout drains
		_ = os.RemoveAll(g.home)
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxReplyLine)
	for scanner.Scan() {
		var event struct {
			Type    string `json:"type"`
			Method  string `json:"method"`
			Message string `json:"message"`
			Command string `json:"command"`
			Success bool   `json:"success"`
			Data    struct {
				Disposition string `json:"disposition"`
			} `json:"data"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return
		}
		// A missing or broken extension must never become an agent turn.
		if event.Type == "response" && event.Command == "prompt" && (!event.Success || event.Data.Disposition != "handled") {
			slog.Error("inference worker did not handle a prompt; killing it")
			return
		}
		if event.Type != "extension_ui_request" || event.Method != "notify" || !strings.HasPrefix(event.Message, signalPrefix) {
			continue
		}
		var r reply
		if json.Unmarshal([]byte(strings.TrimPrefix(event.Message, signalPrefix)), &r) != nil {
			return
		}
		g.mu.Lock()
		ch := g.pending[r.ID]
		delete(g.pending, r.ID)
		g.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	}
}

func (g *generation) send(ctx context.Context, env envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	if len(body) > maxEnvelope {
		return fmt.Errorf("%w: envelope too large", errNotSent)
	}
	line, _ := json.Marshal(map[string]any{"type": "prompt", "id": "p" + env.ID, "message": string(body)})
	line = append(line, '\n')
	select {
	case <-ctx.Done():
		return errNotSent
	case <-g.done:
		return errNotSent
	case g.queue <- line:
		return nil
	}
}

// call sends one envelope and waits for its reply, the deadline or the
// generation's death. On a deadline it cancels that one job; if the worker
// does not answer the cancel within cancelGrace the generation is killed.
func (g *generation) call(ctx context.Context, env envelope) (reply, error) {
	ch := make(chan reply, 1)
	g.mu.Lock()
	g.pending[env.ID] = ch
	g.mu.Unlock()
	defer func() { g.mu.Lock(); delete(g.pending, env.ID); g.mu.Unlock() }()
	if err := g.send(ctx, env); err != nil {
		return reply{}, err
	}
	select {
	case r := <-ch:
		return r, nil
	case <-g.done:
		return reply{}, errors.New("worker exited; outcome unknown")
	case <-ctx.Done():
		grace, cancel := context.WithTimeout(context.Background(), cancelGrace)
		defer cancel()
		_ = g.send(grace, envelope{ID: env.ID, Op: "cancel"})
		select {
		case r := <-ch:
			// Keep any usage the aborted call reported.
			return r, ctx.Err()
		case <-g.done:
			return reply{}, ctx.Err()
		case <-grace.Done():
			slog.Error("inference worker ignored a cancel; killing it")
			g.fail()
			return reply{}, ctx.Err()
		}
	}
}
