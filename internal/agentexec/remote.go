package agentexec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Endpoint is supervisor-only connection/lifecycle data supplied by the backend.
// Headers supports a hosting gateway in addition to worker bearer auth. Neither
// is included in StartConfig, a child environment, a URL, or an error message.
type Endpoint struct {
	URL             string
	Token           string
	Headers         http.Header
	Transport       http.RoundTripper
	Client          *http.Client // Optional gateway client; copied with bounded timeout and redirects disabled.
	AllowInsecure   bool         // Explicit opt-in for a trusted private HTTP gateway.
	AllowUnassigned bool         // Explicit local-standalone service mode, never a Sprite default.
	Active          func(context.Context) error
	// Idle is called with a complete checkpoint BEFORE releasing the active
	// hold. It may durably acknowledge that checkpoint and release a task hold.
	Idle    func(context.Context, Snapshot) error
	Release func(context.Context) error
	// LoadSnapshot revalidates the central assignment and loads its acknowledged
	// snapshot after a cold restart/replacement. It is used only for a NEW,
	// explicit prompt after confirmed idle, never to replay unfinished work.
	LoadSnapshot func(context.Context, Identity) (RestoreState, error)
}

type RestoreState struct {
	SessionFile string
	ArtifactID  string // Current server-side create binding, not a tool-result id.
}

// Provisioner must acquire/revalidate an already centrally reserved assignment.
// It must not manufacture a new attempt when the previous outcome is uncertain.
type Provisioner interface {
	Acquire(context.Context, Identity) (Endpoint, error)
}
type ProvisionFunc func(context.Context, Identity) (Endpoint, error)

func (f ProvisionFunc) Acquire(ctx context.Context, id Identity) (Endpoint, error) { return f(ctx, id) }

// Remote never looks up Pi, opens core scratch, or falls back to a local runner.
type Remote struct {
	Endpoint    Endpoint // Static local-standalone endpoint when Provisioner is nil.
	Provisioner Provisioner
	Transport   http.RoundTripper
}

type RemoteProcess struct {
	endpoint       Endpoint
	client         *http.Client
	base           string
	out            *io.PipeReader
	writer         *io.PipeWriter
	done           chan struct{}
	lifetime       context.Context
	cancel         context.CancelFunc
	once           sync.Once
	cleanupMu      sync.Mutex
	reclaimer      *remoteReclaimer
	commandMu      sync.Mutex
	mu             sync.Mutex
	err            error
	cursor         uint64
	pumping        bool
	generation     uint64
	stopping       bool
	closeRequested bool
	closeID        string
	selection      []string
	hasSelection   bool
	final          *Snapshot
	quiescent      bool
	safeRestore    bool
	config         StartConfig
	provisioner    Provisioner
	acknowledged   chan struct{}
	leasePending   bool
}

// UncertainError means a command may have run. Reserve a new fenced attempt
// from a central snapshot after telling the caller; never automatically retry
// that command or silently resume its unfinished prompt.
type UncertainError struct{ Operation string }

func (e *UncertainError) Error() string {
	return "worker " + e.Operation + " outcome is uncertain; unfinished commands were not replayed"
}

type HTTPError struct {
	Status        int
	Code, Message string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("worker %s (%d): %s", e.Code, e.Status, e.Message)
}

func (r Remote) Start(ctx context.Context, cfg StartConfig) (Process, error) {
	endpoint := r.Endpoint
	var err error
	if r.Provisioner != nil {
		endpoint, err = r.Provisioner.Acquire(ctx, cfg.Identity)
		if err != nil {
			return nil, err
		}
	}
	if err = validateEndpoint(endpoint); err != nil {
		return nil, err
	}
	out, writer := io.Pipe()
	lifetime, cancel := context.WithCancel(context.Background())
	p := &RemoteProcess{endpoint: endpoint, client: workerClient(endpoint, r.Transport),
		base: strings.TrimRight(endpoint.URL, "/"), out: out, writer: writer, done: make(chan struct{}), lifetime: lifetime, cancel: cancel}
	p.provisioner = r.Provisioner
	succeeded := false
	defer func() {
		if !succeeded {
			p.terminate(err)
			_ = out.Close()
			if endpoint.Release != nil {
				cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				_ = endpoint.Release(cleanupCtx)
			}
		}
	}()
	if endpoint.Active != nil {
		if err = endpoint.Active(ctx); err != nil {
			return nil, err
		}
	}
	var ready Readiness
	if err = p.request(ctx, http.MethodGet, "/v1/ready", nil, &ready, 1<<20); err != nil {
		return nil, err
	}
	if ready.Version != ProtocolVersion || ready.PiVersion != PiVersion || (!ready.Unassigned && (ready.ExecutionID != cfg.ExecutionID || ready.SessionID != cfg.SessionID)) || (ready.Unassigned && !endpoint.AllowUnassigned) {
		err = errors.New("worker readiness/version/assignment mismatch")
		return nil, err
	}
	for _, capability := range []string{"commands", "idle-disconnect", "snapshot", "selection", "deduplication", "leases", "idle-ack", "final-ack"} {
		found := false
		for _, value := range ready.Capabilities {
			if value == capability {
				found = true
			}
		}
		if !found {
			err = fmt.Errorf("worker lacks required %s capability", capability)
			return nil, err
		}
	}
	p.base += "/v1/executions/" + url.PathEscape(cfg.ExecutionID)
	if cfg.ExpiresAt.IsZero() {
		cfg.ExpiresAt = time.Now().Add(24*time.Hour - time.Second)
	}
	p.config = cfg
	err = p.request(ctx, http.MethodPost, "/start", cfg, new(any), 1<<20)
	if err != nil {
		return nil, err
	}
	succeeded = true
	p.wake()
	return p, nil
}

func (p *RemoteProcess) Stdout() io.Reader { return p.out }

func (p *RemoteProcess) SynchronizeEvents() func() {
	p.mu.Lock()
	if p.acknowledged == nil {
		p.acknowledged = make(chan struct{}, 1)
	}
	ack := p.acknowledged
	p.mu.Unlock()
	return func() {
		select {
		case ack <- struct{}{}:
		case <-p.lifetime.Done():
		}
	}
}

// FailEvents stops delivery without acknowledging the failed checkpoint, so no
// idle callback or daemon hold-release acknowledgment can follow it.
func (p *RemoteProcess) FailEvents(err error) { p.terminate(err) }
func (p *RemoteProcess) Stderr() io.Reader    { return strings.NewReader("") }
func (p *RemoteProcess) Wait() error          { <-p.done; p.mu.Lock(); defer p.mu.Unlock(); return p.err }
func (p *RemoteProcess) WriteSelection(ctx context.Context, selection []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.selection = append([]string(nil), selection...)
	p.hasSelection = true
	return nil
}
func (p *RemoteProcess) Write(data []byte) (int, error) {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	if len(data) > MaxCommandBytes {
		return 0, errors.New("worker command exceeds limit")
	}
	var rpc struct{ ID, Type string }
	if json.Unmarshal(data, &rpc) != nil || rpc.ID == "" {
		return 0, errors.New("worker commands require stable Pi RPC identities")
	}
	p.mu.Lock()
	if p.stopping {
		p.mu.Unlock()
		return 0, errors.New("worker execution stopped")
	}
	var selection *[]string
	if p.hasSelection {
		value := p.selection
		selection = &value
		p.hasSelection = false
		p.selection = nil
	}
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(p.lifetime, 30*time.Second)
	defer cancel()
	if err := p.ensureAttached(ctx, rpc.Type); err != nil {
		p.terminate(err)
		return 0, err
	}
	if p.endpoint.Active != nil {
		if err := p.endpoint.Active(ctx); err != nil {
			return 0, err
		}
	}
	command := Command{ID: rpc.ID, Deadline: time.Now().Add(2*time.Minute - time.Second), RPC: bytes.TrimSpace(data), Selection: selection}
	if err := p.request(ctx, http.MethodPost, "/commands", command, new(any), 1<<20); err != nil {
		// A failed response is never retried. Terminating the transport forces
		// explicit interrupted-session recovery and central lease fencing.
		p.terminate(err)
		return 0, err
	}
	p.wake()
	return len(data), nil
}

func (p *RemoteProcess) wake() {
	p.mu.Lock()
	p.generation++
	if !p.pumping {
		p.pumping = true
		go p.pump()
	}
	p.mu.Unlock()
}

// Only an explicit new prompt after confirmed idle may reconstruct a cold
// worker. Background stats, event delivery failures, and unfinished prompts
// never enter this recovery path.
func (p *RemoteProcess) ensureAttached(ctx context.Context, kind string) error {
	if kind != "prompt" && kind != "steer" && kind != "extension_ui_response" {
		return nil
	}
	p.mu.Lock()
	idle, cfg, endpoint, safe := p.quiescent, p.config, p.endpoint, p.safeRestore
	p.mu.Unlock()
	if !idle {
		return nil
	}
	if p.provisioner != nil {
		var err error
		endpoint, err = p.provisioner.Acquire(ctx, cfg.Identity)
		if err != nil {
			return err
		}
		if err := validateEndpoint(endpoint); err != nil {
			return err
		}
		client := workerClient(endpoint, nil)
		p.mu.Lock()
		previous := p.client
		p.endpoint, p.client, p.base = endpoint, client, strings.TrimRight(endpoint.URL, "/")+"/v1/executions/"+url.PathEscape(cfg.ExecutionID)
		p.mu.Unlock()
		previous.CloseIdleConnections()
	}
	if endpoint.Active != nil {
		if err := endpoint.Active(ctx); err != nil {
			return err
		}
	}
	var ready Readiness
	if err := p.request(ctx, http.MethodGet, "/ready", nil, &ready, 1<<20); err != nil {
		return err
	}
	if ready.Version != ProtocolVersion || ready.PiVersion != PiVersion || ready.ExecutionID != cfg.ExecutionID || (ready.SessionID != "" && ready.SessionID != cfg.SessionID) || ready.Closed || (ready.Unassigned && !endpoint.AllowUnassigned) {
		return errors.New("worker assignment changed or expired; reserve a new attempt")
	}
	if !ready.Started {
		if !safe {
			return &UncertainError{Operation: "cold restart during an unfinished tool"}
		}
		if endpoint.LoadSnapshot == nil {
			return errors.New("worker restarted; a central snapshot loader is required before a new prompt")
		}
		restored, err := endpoint.LoadSnapshot(ctx, cfg.Identity)
		if err != nil {
			return err
		}
		cfg.SessionSnapshot, cfg.TargetArtifactID = restored.SessionFile, restored.ArtifactID
		if err := p.request(ctx, http.MethodPost, "/start", cfg, new(any), 1<<20); err != nil {
			return err
		}
		p.mu.Lock()
		p.cursor = 0
		p.config = cfg
		p.mu.Unlock()
	} else {
		p.mu.Lock()
		pending := p.leasePending
		p.mu.Unlock()
		if pending {
			if err := p.request(ctx, http.MethodPost, "/lease", LeaseUpdate{ID: uuid.NewString(), ExpiresAt: cfg.ExpiresAt}, new(any), 1<<20); err != nil {
				return err
			}
		}
	}
	p.mu.Lock()
	p.quiescent = false
	p.leasePending = false
	p.mu.Unlock()
	return nil
}

func validateEndpoint(endpoint Endpoint) error {
	parsed, err := url.Parse(endpoint.URL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || len(endpoint.Token) < 32 {
		return errors.New("invalid worker endpoint or authentication")
	}
	if parsed.Scheme == "http" && !endpoint.AllowInsecure && parsed.Hostname() != "localhost" && !net.ParseIP(parsed.Hostname()).IsLoopback() {
		return errors.New("worker endpoint requires HTTPS or an explicit trusted private HTTP configuration")
	}
	return nil
}

func workerClient(endpoint Endpoint, fallback http.RoundTripper) *http.Client {
	transport := endpoint.Transport
	if transport == nil && endpoint.Client != nil {
		transport = endpoint.Client.Transport
	}
	if transport == nil {
		transport = fallback
	}
	if transport == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.DisableKeepAlives = true
		t.MaxConnsPerHost = 4
		transport = t
	}
	client := http.Client{}
	if endpoint.Client != nil {
		client = *endpoint.Client
	}
	client.Transport = transport
	if client.Timeout <= 0 || client.Timeout > 35*time.Second {
		client.Timeout = 35 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("worker redirects are forbidden") }
	return &client
}
func (p *RemoteProcess) pump() {
	for {
		p.mu.Lock()
		cursor, generation := p.cursor, p.generation
		p.mu.Unlock()
		var batch EventBatch
		err := p.request(p.lifetime, http.MethodGet, "/events?after="+strconv.FormatUint(cursor, 10), nil, &batch, MaxQueueBytes+(1<<20))
		if err != nil {
			p.terminate(err)
			return
		}
		for _, event := range batch.Events {
			if event.Sequence != cursor+1 || len(event.Line) > MaxEventBytes {
				p.terminate(&UncertainError{Operation: "event delivery"})
				return
			}
			if _, err := p.writer.Write(append(bytes.Clone(event.Line), '\n')); err != nil {
				p.terminate(err)
				return
			}
			p.mu.Lock()
			ack := p.acknowledged
			p.mu.Unlock()
			if ack != nil {
				select {
				case <-ack:
				case <-p.lifetime.Done():
					return
				}
			}
			cursor = event.Sequence
		}
		if cursor != batch.Cursor {
			p.terminate(&UncertainError{Operation: "event cursor"})
			return
		}
		p.mu.Lock()
		p.cursor = cursor
		p.mu.Unlock()
		if batch.Closed {
			p.terminate(nil)
			return
		}
		if batch.Idle {
			// Serialize idle callbacks against command submission. A task hold
			// must never be released underneath the next accepted prompt.
			p.commandMu.Lock()
			p.mu.Lock()
			current := p.generation
			p.mu.Unlock()
			if current == generation && p.endpoint.Idle != nil {
				ctx, cancel := context.WithTimeout(p.lifetime, 30*time.Second)
				var snapshot Snapshot
				err = p.request(ctx, http.MethodGet, "/snapshot", nil, &snapshot, 2*MaxSessionBytes+(1<<20))
				if err == nil {
					err = p.endpoint.Idle(ctx, snapshot)
				}
				cancel()
				if err != nil {
					p.commandMu.Unlock()
					p.terminate(err)
					return
				}
			}
			if current == generation {
				ctx, cancel := context.WithTimeout(p.lifetime, 10*time.Second)
				err = p.request(ctx, http.MethodPost, "/idle", map[string]uint64{"cursor": cursor}, new(any), 1<<20)
				cancel()
				if err != nil {
					p.commandMu.Unlock()
					var changed *HTTPError
					if errors.As(err, &changed) && changed.Code == "idle_changed" {
						continue
					}
					p.terminate(err)
					return
				}
			}
			p.mu.Lock()
			if p.generation == generation {
				p.quiescent = true
				p.safeRestore = batch.Settled
				p.pumping = false
				p.mu.Unlock()
				p.commandMu.Unlock()
				p.closeConnections()
				return // No timer, health poll, stdout socket, or background reconnect.
			}
			p.mu.Unlock()
			p.commandMu.Unlock()
		}
	}
}

func (p *RemoteProcess) Snapshot(ctx context.Context) (string, error) {
	p.mu.Lock()
	final := p.final
	failed := p.err
	p.mu.Unlock()
	if final != nil {
		return final.SessionFile, nil
	}
	if failed != nil {
		return "", failed
	}
	var snapshot Snapshot
	if err := p.request(ctx, http.MethodGet, "/snapshot", nil, &snapshot, 2*MaxSessionBytes+(1<<20)); err != nil {
		return "", err
	}
	return snapshot.SessionFile, nil
}
func (p *RemoteProcess) Close() error { return p.Kill() }

func (p *RemoteProcess) SetLease(ctx context.Context, deadline time.Time) error {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	if !deadline.After(time.Now()) || deadline.After(time.Now().Add(24*time.Hour)) {
		return errors.New("worker lease must expire within 24 hours")
	}
	p.mu.Lock()
	stopped, quiescent := p.stopping, p.quiescent
	p.mu.Unlock()
	if stopped {
		return errors.New("worker execution stopped")
	}
	if quiescent {
		p.mu.Lock()
		p.config.ExpiresAt = deadline
		p.leasePending = true
		p.mu.Unlock()
		return nil
	}
	if err := p.request(ctx, http.MethodPost, "/lease", LeaseUpdate{ID: uuid.NewString(), ExpiresAt: deadline}, new(any), 1<<20); err != nil {
		return err
	}
	p.mu.Lock()
	p.config.ExpiresAt = deadline
	p.mu.Unlock()
	return nil
}

func (p *RemoteProcess) Kill() error {
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	p.mu.Lock()
	if p.closeRequested {
		p.mu.Unlock()
		return nil
	}
	p.closeRequested = true
	p.stopping = true
	if p.closeID == "" {
		p.closeID = uuid.NewString()
	}
	closeID := p.closeID
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var snapshot Snapshot
	if err := p.request(ctx, http.MethodPost, "/close", map[string]string{"id": closeID}, &snapshot, 2*MaxSessionBytes+(1<<20)); err != nil {
		p.mu.Lock()
		p.closeRequested = false
		p.mu.Unlock()
		p.terminate(err)
		return err
	}
	p.mu.Lock()
	p.final = &snapshot
	terminated := p.lifetime.Err() != nil
	p.mu.Unlock()
	if !terminated {
		p.wake() // Drain final usage/events before Wait reports execution ended.
	}
	return nil
}
func (p *RemoteProcess) Cleanup() error {
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()
	if p.reclaimer == nil {
		if err := p.Close(); err != nil {
			return err
		}
		p.mu.Lock()
		p.reclaimer = newRemoteReclaimer(p.endpoint, p.client, p.base, true, p.closeID)
		p.mu.Unlock()
	}
	err := p.reclaimer.Cleanup()
	p.terminate(err)
	_ = p.out.Close()
	return err
}

func (p *RemoteProcess) ErasureCleanup() Reclaimer {
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()
	p.terminate(nil)
	_ = p.out.Close()
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reclaimer == nil {
		p.reclaimer = newRemoteReclaimer(p.endpoint, p.client, p.base, p.closeRequested, p.closeID)
	}
	// No transcript, provider key, tool grant, selection, or restore callback
	// belongs in the retained owner-erasure target.
	p.final = nil
	p.config = StartConfig{}
	p.selection = nil
	p.provisioner = nil
	p.endpoint = Endpoint{}
	return p.reclaimer
}

// remoteReclaimer contains no execution payload. Each acknowledged phase is
// recorded independently so transient errors never consume the cleanup target.
type remoteReclaimer struct {
	mu                         sync.Mutex
	endpoint                   Endpoint
	client                     *http.Client
	base, closeID, releaseID   string
	stopped, released, cleaned bool
}

func newRemoteReclaimer(endpoint Endpoint, client *http.Client, base string, stopped bool, closeID string) *remoteReclaimer {
	endpoint.Active, endpoint.Idle, endpoint.LoadSnapshot = nil, nil, nil
	if closeID == "" {
		closeID = uuid.NewString()
	}
	return &remoteReclaimer{endpoint: endpoint, client: client, base: base, stopped: stopped, closeID: closeID, releaseID: uuid.NewString()}
}
func (r *remoteReclaimer) Cleanup() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cleaned {
		return nil
	}
	defer r.client.CloseIdleConnections()
	if !r.stopped {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		// Decode and discard; deleted history is never retained by this capability.
		err := requestWorker(ctx, r.client, r.endpoint, r.base, http.MethodPost, "/close", map[string]string{"id": r.closeID}, new(any), 2*MaxSessionBytes+(1<<20))
		cancel()
		if err != nil {
			return err
		}
		r.stopped = true
	}
	if !r.released {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := requestWorker(ctx, r.client, r.endpoint, r.base, http.MethodPost, "/release", map[string]string{"id": r.releaseID}, new(any), 1<<20)
		cancel()
		if err != nil {
			return err
		}
		r.released = true
	}
	if r.endpoint.Release != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := r.endpoint.Release(ctx)
		cancel()
		if err != nil {
			return err
		}
	}
	r.cleaned = true
	r.endpoint = Endpoint{}
	return nil
}
func (p *RemoteProcess) terminate(err error) {
	p.once.Do(func() {
		p.mu.Lock()
		p.err = err
		p.stopping = true
		p.mu.Unlock()
		p.cancel()
		_ = p.writer.CloseWithError(err)
		p.closeConnections()
		close(p.done)
	})
}

func (p *RemoteProcess) request(ctx context.Context, method, path string, input, output any, limit int64) error {
	p.mu.Lock()
	base, endpoint, client := p.base, p.endpoint, p.client
	p.mu.Unlock()
	return requestWorker(ctx, client, endpoint, base, method, path, input, output, limit)
}
func requestWorker(ctx context.Context, client *http.Client, endpoint Endpoint, base, method, path string, input, output any, limit int64) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return errors.New("invalid worker request")
	}
	req.Header = endpoint.Headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	req.Header.Set(VersionHeader, ProtocolVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Close = true
	resp, err := client.Do(req)
	if err != nil {
		return &UncertainError{Operation: method + " request"}
	}
	defer resp.Body.Close()
	if resp.Header.Get(VersionHeader) != ProtocolVersion {
		return errors.New("incompatible worker protocol response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure WireError
		if json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&failure) != nil {
			return errors.New("worker rejected request")
		}
		return &HTTPError{Status: resp.StatusCode, Code: failure.Code, Message: failure.Message}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return &UncertainError{Operation: "response read"}
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errors.New("invalid worker response")
	}
	return nil
}

func (p *RemoteProcess) closeConnections() {
	p.mu.Lock()
	client := p.client
	p.mu.Unlock()
	client.CloseIdleConnections()
}
