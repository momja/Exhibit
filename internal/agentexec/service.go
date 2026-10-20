package agentexec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ServiceConfig pins a daemon to ONE assignment. Its bearer credential is
// supplied by core/provisioning and never enters Pi or any event/URL.
type ServiceConfig struct {
	ExecutionID  string
	SessionID    string
	Token        string
	Runner       Runner
	ReadyTimeout time.Duration
	// LocalAssignments enables a bounded multi-assignment service for static
	// development/self-hosted endpoints. Authentication remains mandatory.
	LocalAssignments bool
	MaxAssignments   int
	Holds            Holds
	HoldTTL          time.Duration
}

// Holds is implemented by the in-Sprite Tasks API. It needs no management
// credential, starts no goroutines itself, and is optional for local workers.
type Holds interface {
	Acquire(context.Context, string, time.Duration) error
	Renew(context.Context, string, time.Duration) error
	Release(context.Context, string) error
}

type Service struct {
	cfg             ServiceConfig
	opMu            sync.Mutex
	mu              sync.Mutex
	process         Process
	startHash       [32]byte
	started         bool
	ready           bool
	readinessFailed bool
	closed          bool
	running         bool
	waiting         bool
	commands        map[string][32]byte
	promptCommands  map[string]bool
	pending         map[string]time.Time
	events          []Event
	eventBytes      int
	cursor          uint64
	changed         chan struct{}
	readDone        chan struct{}
	finalSnapshot   string
	finalErr        error
	expires         time.Time
	turnTimeout     time.Duration
	turnStarted     time.Time
	subscribers     chan struct{}
	requests        chan struct{}
	children        map[string]*Service
	retired         map[string]bool
	holdActive      bool
	holdRenewAt     time.Time
}

func NewService(cfg ServiceConfig) (*Service, error) {
	if (!cfg.LocalAssignments && (cfg.ExecutionID == "" || cfg.SessionID == "")) || len(cfg.Token) < 32 || cfg.Runner == nil {
		return nil, errors.New("worker needs assigned execution/session, at least 32-byte authentication, and a runner")
	}
	if strings.ContainsAny(cfg.ExecutionID+cfg.SessionID, "/?#\r\n") {
		return nil, errors.New("invalid assigned identity")
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = 20 * time.Second
	}
	if cfg.MaxAssignments <= 0 {
		cfg.MaxAssignments = 32
	}
	if cfg.MaxAssignments > 32 {
		return nil, errors.New("at most 32 local worker assignments are supported")
	}
	if cfg.HoldTTL == 0 {
		cfg.HoldTTL = time.Minute
	}
	if cfg.HoldTTL < 3*time.Second || cfg.HoldTTL > 5*time.Minute || cfg.HoldTTL%time.Second != 0 {
		return nil, errors.New("worker hold TTL must be whole seconds from 3 seconds to 5 minutes")
	}
	return &Service{cfg: cfg, changed: make(chan struct{}), commands: make(map[string][32]byte), promptCommands: make(map[string]bool), pending: make(map[string]time.Time), subscribers: make(chan struct{}, 4), requests: make(chan struct{}, 8), children: make(map[string]*Service), retired: make(map[string]bool)}, nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(VersionHeader, ProtocolVersion)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close") // A completed request must not keep a Sprite awake.
	w.Header().Set("Content-Type", "application/json")
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.cfg.Token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "worker authentication required")
		return
	}
	if r.Header.Get(VersionHeader) != ProtocolVersion {
		writeError(w, http.StatusUpgradeRequired, "incompatible_version", "unsupported Exhibit agent protocol version")
		return
	}
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
	default:
		writeError(w, 429, "request_limit", "worker concurrent request capacity reached")
		return
	}
	if s.cfg.LocalAssignments {
		s.serveAssignments(w, r)
		return
	}
	base := "/v1/executions/" + url.PathEscape(s.cfg.ExecutionID)
	if (r.URL.EscapedPath() == "/v1/ready" || r.URL.EscapedPath() == base+"/ready") && r.Method == http.MethodGet {
		s.mu.Lock()
		started, closed := s.started, s.closed
		s.mu.Unlock()
		writeJSON(w, Readiness{Version: ProtocolVersion, PiVersion: PiVersion, ExecutionID: s.cfg.ExecutionID, SessionID: s.cfg.SessionID,
			Started: started, Closed: closed,
			Capabilities: []string{"commands", "idle-disconnect", "snapshot", "selection", "deduplication", "leases", "idle-ack", "final-ack"}})
		return
	}
	path := strings.TrimPrefix(r.URL.EscapedPath(), base)
	if r.URL.EscapedPath() != base+path || !strings.HasPrefix(r.URL.EscapedPath(), base+"/") {
		writeError(w, 404, "assignment_not_found", "this worker does not own that execution")
		return
	}
	switch {
	case r.Method == http.MethodPost && path == "/start":
		s.start(w, r)
	case r.Method == http.MethodPost && path == "/commands":
		s.command(w, r)
	case r.Method == http.MethodGet && path == "/events":
		s.eventBatch(w, r)
	case r.Method == http.MethodPost && path == "/lease":
		s.setLease(w, r)
	case r.Method == http.MethodPost && path == "/release":
		if err := s.Reclaim(r.Context()); err != nil {
			writeError(w, 409, "reclaim_failed", "closed worker snapshot must be acknowledged before reclamation")
			return
		}
		writeJSON(w, map[string]bool{"released": true})
	case r.Method == http.MethodPost && path == "/idle":
		var ack struct {
			Cursor uint64 `json:"cursor"`
		}
		if !decodeJSON(w, r, &ack, 1024) {
			return
		}
		s.opMu.Lock()
		s.mu.Lock()
		idle := (!s.running || s.waiting) && len(s.pending) == 0 && ack.Cursor == s.cursor
		s.mu.Unlock()
		if !idle {
			s.opMu.Unlock()
			writeError(w, 409, "idle_changed", "worker received more events or commands before idle acknowledgment")
			return
		}
		err := s.releaseHold(r.Context())
		s.opMu.Unlock()
		if err != nil {
			writeError(w, 503, "hold_release_failed", "worker idle hold release failed")
			return
		}
		writeJSON(w, map[string]bool{"idle": true})
	case r.Method == http.MethodGet && path == "/snapshot":
		file, err := s.snapshot(r.Context())
		if err != nil {
			writeError(w, 503, "snapshot_failed", "worker snapshot unavailable")
			return
		}
		s.mu.Lock()
		snapshot := Snapshot{SessionFile: file, Cursor: s.cursor, Closed: s.closed, Settled: !s.running && !s.waiting && len(s.pending) == 0}
		s.mu.Unlock()
		writeJSON(w, snapshot)
	case r.Method == http.MethodPost && path == "/close":
		var command struct {
			ID string `json:"id"`
		}
		if !decodeJSON(w, r, &command, 1024) {
			return
		}
		if command.ID == "" {
			writeError(w, 400, "invalid_command", "close requires a stable command identity")
			return
		}
		if err := s.Shutdown(r.Context()); err != nil {
			writeError(w, 503, "close_failed", "worker final snapshot unavailable")
			return
		}
		s.mu.Lock()
		result := Snapshot{SessionFile: s.finalSnapshot, Cursor: s.cursor, Closed: true, Settled: !s.running && !s.waiting}
		s.mu.Unlock()
		writeJSON(w, result)
	default:
		writeError(w, 404, "not_found", "unknown worker operation")
	}
}

func (s *Service) setLease(w http.ResponseWriter, r *http.Request) {
	var update LeaseUpdate
	if !decodeJSON(w, r, &update, 2048) {
		return
	}
	if update.ID == "" || len(update.ID) > 200 || !update.ExpiresAt.After(time.Now()) || update.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
		writeError(w, 400, "invalid_lease", "assigned lease must have stable identity and expire within 24 hours")
		return
	}
	data, _ := json.Marshal(update)
	hash := sha256.Sum256(data)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if existing, ok := s.commands[update.ID]; ok {
		s.mu.Unlock()
		if existing != hash {
			writeError(w, 409, "command_conflict", "lease identity already used")
			return
		}
		writeJSON(w, map[string]bool{"renewed": true})
		return
	}
	if s.closed || !s.ready || (s.running && !s.waiting && !time.Now().Before(s.expires)) {
		s.mu.Unlock()
		writeError(w, 409, "execution_unavailable", "worker execution lease is no longer active")
		return
	}
	if len(s.commands) >= MaxCommands {
		s.mu.Unlock()
		writeError(w, 429, "command_limit", "execution command capacity reached")
		return
	}
	p, previous := s.process, s.expires
	s.mu.Unlock()
	if update.ExpiresAt.Before(previous) {
		writeError(w, 409, "stale_lease", "worker lease cannot be moved backwards")
		return
	}
	if lease, ok := p.(LeaseProcess); ok {
		if err := lease.SetLease(r.Context(), update.ExpiresAt); err != nil {
			writeError(w, 503, "lease_failed", "unable to update Pi lease deadline")
			return
		}
	}
	s.mu.Lock()
	s.expires = update.ExpiresAt
	s.commands[update.ID] = hash
	s.mu.Unlock()
	writeJSON(w, map[string]bool{"renewed": true})
}

func (s *Service) start(w http.ResponseWriter, r *http.Request) {
	var cfg StartConfig
	if !decodeJSON(w, r, &cfg, 2*MaxSessionBytes+(1<<20)) {
		return
	}
	if cfg.ExecutionID != s.cfg.ExecutionID || cfg.SessionID != s.cfg.SessionID {
		writeError(w, 403, "wrong_assignment", "execution identity does not match this worker")
		return
	}
	if cfg.ToolToken == "" || cfg.ToolToken == s.cfg.Token || cfg.APIKey == "" || cfg.APIBaseURL == "" || len(cfg.SessionSnapshot) > MaxSessionBytes || len(cfg.SystemPrompt) > 256<<10 {
		writeError(w, 400, "invalid_configuration", "scoped execution configuration required")
		return
	}
	now := time.Now()
	if !cfg.ExpiresAt.After(now) || cfg.ExpiresAt.After(now.Add(24*time.Hour)) || cfg.TurnTimeout < 0 || cfg.TurnTimeout > time.Hour {
		writeError(w, 400, "invalid_deadline", "execution expiry must be within 24 hours and turn timeout at most one hour")
		return
	}
	data, _ := json.Marshal(cfg)
	hash := sha256.Sum256(data)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.started || s.closed {
		match, ready := s.started && s.startHash == hash, s.ready && !s.closed
		s.mu.Unlock()
		if !match {
			writeError(w, 409, "assignment_conflict", "execution already started with different configuration")
			return
		}
		if !ready {
			writeError(w, 409, "execution_unavailable", "execution is stopped or startup was uncertain; reserve a new attempt")
			return
		}
		writeJSON(w, map[string]bool{"ready": true})
		return
	}
	s.started, s.startHash = true, hash // Never silently replay an uncertain startup.
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.ReadyTimeout)
	defer cancel()
	p, err := s.cfg.Runner.Start(ctx, cfg)
	if err != nil {
		writeError(w, 503, "start_failed", "unable to start assigned Pi runtime")
		return
	}
	s.mu.Lock()
	s.process, s.readDone, s.expires = p, make(chan struct{}), cfg.ExpiresAt
	s.turnTimeout = cfg.TurnTimeout
	if s.turnTimeout == 0 {
		s.turnTimeout = 15 * time.Minute
	}
	readyID := "worker-ready-" + s.cfg.ExecutionID
	s.pending[readyID] = time.Now().Add(s.cfg.ReadyTimeout)
	s.mu.Unlock()
	go s.readOutput(p.Stdout())
	// Stderr can carry provider errors containing credentials or conversation
	// contents. Drain it without retaining it or exposing it to clients/logs.
	go io.Copy(io.Discard, p.Stderr())
	go func() {
		_ = p.Wait()
		<-s.readDone
		s.mu.Lock()
		s.closed = true
		s.signalLocked()
		s.mu.Unlock()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.Shutdown(cleanupCtx)
	}()
	go s.watchdog(p)
	line, _ := json.Marshal(map[string]string{"type": "get_state", "id": readyID})
	if _, err := p.Write(append(line, '\n')); err != nil {
		_ = p.Close()
		writeError(w, 503, "readiness_failed", "Pi readiness command failed")
		return
	}
	for {
		s.mu.Lock()
		_, pending := s.pending[readyID]
		closed, changed := s.closed || s.readinessFailed, s.changed
		if !pending && !closed {
			s.ready = true
		}
		s.mu.Unlock()
		if closed {
			_ = p.Close()
			writeError(w, 503, "readiness_failed", "Pi exited before readiness")
			return
		}
		if !pending {
			writeJSON(w, map[string]bool{"ready": true})
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			_ = p.Close()
			writeError(w, 503, "readiness_timeout", "Pi did not become ready before the startup deadline")
			return
		}
	}
}

func (s *Service) command(w http.ResponseWriter, r *http.Request) {
	var command Command
	if !decodeJSON(w, r, &command, MaxCommandBytes) {
		return
	}
	var rpc struct{ Type, ID string }
	if json.Unmarshal(command.RPC, &rpc) != nil || command.ID == "" || len(command.ID) > 200 || rpc.ID != command.ID {
		writeError(w, 400, "invalid_command", "command identity must match its Pi RPC identity")
		return
	}
	switch rpc.Type {
	case "prompt", "steer", "abort", "get_state", "get_session_stats", "compact", "extension_ui_response":
	default:
		writeError(w, 400, "unsupported_command", "unsupported Pi RPC operation")
		return
	}
	canonical, _ := json.Marshal(struct {
		RPC       json.RawMessage
		Selection *[]string
	}{command.RPC, command.Selection})
	hash := sha256.Sum256(canonical)
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if existing, ok := s.commands[command.ID]; ok {
		s.mu.Unlock()
		if existing != hash {
			writeError(w, 409, "command_conflict", "command identity was already used with different content")
			return
		}
		writeJSON(w, map[string]bool{"accepted": true})
		return
	}
	if s.closed || !s.ready {
		s.mu.Unlock()
		writeError(w, 409, "execution_unavailable", "execution is not ready; do not replay unfinished commands")
		return
	}
	if len(s.commands) >= MaxCommands || len(s.pending) >= 16 {
		s.mu.Unlock()
		writeError(w, 429, "command_limit", "execution command capacity reached")
		return
	}
	now := time.Now()
	if !command.Deadline.After(now) || command.Deadline.After(now.Add(2*time.Minute+time.Second)) {
		s.mu.Unlock()
		writeError(w, 400, "invalid_deadline", "command deadline must be within two minutes")
		return
	}
	p := s.process
	needsHold := rpc.Type == "prompt" || rpc.Type == "steer" || rpc.Type == "extension_ui_response"
	if needsHold && !time.Now().Before(s.expires) {
		s.mu.Unlock()
		writeError(w, 409, "lease_expired", "renew the assigned lease before accepting more work")
		return
	}
	s.mu.Unlock()
	if needsHold {
		if err := s.acquireHold(r.Context()); err != nil {
			writeError(w, 503, "hold_unavailable", "unable to acquire active worker hold; command was not accepted")
			return
		}
		setProcessActive(p, true)
	}
	s.mu.Lock()
	s.commands[command.ID] = hash
	if rpc.Type == "prompt" || rpc.Type == "steer" {
		s.promptCommands[command.ID] = true
		s.running = true
		s.turnStarted = now
	}
	if rpc.Type != "extension_ui_response" {
		s.pending[command.ID] = command.Deadline
	}
	if rpc.Type == "extension_ui_response" {
		s.waiting = false
		s.turnStarted = now
	}
	s.signalLocked()
	s.mu.Unlock()
	if command.Selection != nil {
		if err := p.WriteSelection(r.Context(), *command.Selection); err != nil {
			_ = p.Close()
			writeError(w, 503, "command_uncertain", "command selection failed; execution stopped")
			return
		}
	}
	if _, err := p.Write(append(bytes.Clone(command.RPC), '\n')); err != nil {
		_ = p.Close()
		writeError(w, 503, "command_uncertain", "command delivery failed; execution stopped without replay")
		return
	}
	writeJSON(w, map[string]bool{"accepted": true})
}

func (s *Service) readOutput(reader io.Reader) {
	defer close(s.readDone)
	r := bufio.NewReaderSize(reader, 64<<10)
	for {
		line, large, err := ReadEventLine(r)
		if !large && len(line) > 0 && json.Valid(line) {
			var event struct {
				Type, ID, Method, Title string
				Success                 bool
				Data                    struct {
					Disposition string `json:"disposition"`
				}
			}
			_ = json.Unmarshal(line, &event)
			s.mu.Lock()
			s.cursor++
			line = bytes.Clone(bytes.TrimSpace(line))
			s.events = append(s.events, Event{Sequence: s.cursor, Line: line})
			s.eventBytes += len(line)
			for len(s.events) > MaxEvents || s.eventBytes > MaxQueueBytes {
				s.eventBytes -= len(s.events[0].Line)
				s.events[0] = Event{}
				s.events = s.events[1:]
			}
			switch event.Type {
			case "response":
				delete(s.pending, event.ID)
				if event.ID == "worker-ready-"+s.cfg.ExecutionID && !event.Success {
					s.readinessFailed = true
				}
				if s.promptCommands[event.ID] && (!event.Success || event.Data.Disposition == "handled") {
					s.running = false
					s.waiting = false
					s.turnStarted = time.Time{}
					setProcessActive(s.process, false)
				}
			case "agent_start":
				s.running = true
				s.waiting = false
				s.turnStarted = time.Now()
				setProcessActive(s.process, true)
			case "agent_settled":
				s.running = false
				s.waiting = false
				s.turnStarted = time.Time{}
				setProcessActive(s.process, false)
			case "extension_ui_request":
				if event.Method == "input" && event.Title == "exhibit:questions:v1" {
					s.waiting = true
					setProcessActive(s.process, false)
				}
			}
			s.signalLocked()
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// ReadEventLine discards oversized JSONL through the terminator, bounded even
// when the subprocess writes an endless record without a newline.
func ReadEventLine(reader *bufio.Reader) ([]byte, bool, error) {
	var line []byte
	large := false
	for {
		fragment, err := reader.ReadSlice('\n')
		if !large {
			if len(line)+len(fragment) > MaxEventBytes {
				line = nil
				large = true
			} else {
				line = append(line, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, large, err
	}
}

func (s *Service) eventBatch(w http.ResponseWriter, r *http.Request) {
	select {
	case s.subscribers <- struct{}{}:
		defer func() { <-s.subscribers }()
	default:
		writeError(w, 429, "subscriber_limit", "too many worker event readers")
		return
	}
	after, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	if err != nil {
		writeError(w, 400, "invalid_cursor", "event cursor required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	for {
		s.mu.Lock()
		if !s.started {
			s.mu.Unlock()
			writeError(w, 409, "not_started", "execution has not started")
			return
		}
		if after > s.cursor || (len(s.events) > 0 && after < s.events[0].Sequence-1) {
			s.mu.Unlock()
			writeError(w, 409, "events_lost", "event cursor no longer retained; execution outcome is uncertain")
			return
		}
		batch := EventBatch{Cursor: s.cursor, Idle: (!s.running || s.waiting) && len(s.pending) == 0, Settled: !s.running && len(s.pending) == 0, Closed: s.closed}
		for _, event := range s.events {
			if event.Sequence > after {
				batch.Events = append(batch.Events, event)
			}
		}
		changed := s.changed
		s.mu.Unlock()
		if len(batch.Events) > 0 || batch.Idle || batch.Closed || ctx.Err() != nil {
			writeJSON(w, batch)
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
		}
	}
}

func (s *Service) snapshot(ctx context.Context) (string, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	p, final, finalErr := s.process, s.finalSnapshot, s.finalErr
	s.mu.Unlock()
	if p == nil {
		if finalErr != nil {
			return "", finalErr
		}
		return final, nil
	}
	return p.Snapshot(ctx)
}

// Shutdown stops Pi and captures complete records. Reclaim is separate: core
// must durably acknowledge the final snapshot and release authority first.
func (s *Service) Shutdown(ctx context.Context) error {
	if s.cfg.LocalAssignments {
		s.mu.Lock()
		s.closed = true
		all := make([]*Service, 0, len(s.children))
		for _, child := range s.children {
			all = append(all, child)
		}
		s.mu.Unlock()
		var failures []error
		for _, child := range all {
			failures = append(failures, child.Shutdown(ctx))
		}
		return errors.Join(failures...)
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	p, readDone := s.process, s.readDone
	if p == nil {
		s.closed = true
		err := s.finalErr
		s.signalLocked()
		s.mu.Unlock()
		return errors.Join(err, s.releaseHold(ctx))
	}
	s.mu.Unlock()
	_ = p.Close()
	select {
	case <-readDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	_ = p.Wait()
	file, err := p.Snapshot(ctx)
	cleanupErr := s.releaseHold(ctx)
	if err == nil {
		err = cleanupErr
	}
	s.mu.Lock()
	s.finalSnapshot, s.finalErr, s.closed = file, err, true
	s.signalLocked()
	s.mu.Unlock()
	return err
}

// Reclaim is an authenticated supervisor acknowledgment, never a tool command.
func (s *Service) Reclaim(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	p, closed, finalErr := s.process, s.closed, s.finalErr
	s.mu.Unlock()
	if !closed {
		return errors.New("execution is still active")
	}
	if finalErr != nil {
		return finalErr
	}
	if p != nil {
		if err := p.Cleanup(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.process = nil
	s.mu.Unlock()
	return nil
}

func setProcessActive(p Process, active bool) {
	if state, ok := p.(interface{ SetActive(bool) }); ok {
		state.SetActive(active)
	}
}

func (s *Service) watchdog(p Process) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.readDone:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			expired := (s.running && !s.waiting && !now.Before(s.expires)) || (s.running && !s.waiting && !s.turnStarted.IsZero() && now.Sub(s.turnStarted) > s.turnTimeout)
			for _, deadline := range s.pending {
				if !now.Before(deadline) {
					expired = true
				}
			}
			s.mu.Unlock()
			if !expired {
				s.opMu.Lock()
				s.mu.Lock()
				renew := s.holdActive && s.running && !s.waiting && !now.Before(s.holdRenewAt)
				s.mu.Unlock()
				if renew {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					ttl, err := s.activeHoldTTL()
					if err == nil {
						err = s.cfg.Holds.Renew(ctx, s.holdName(), ttl)
					}
					cancel()
					if err != nil {
						expired = true
					} else {
						s.mu.Lock()
						s.holdRenewAt = now.Add(s.cfg.HoldTTL / 3)
						s.mu.Unlock()
					}
				}
				s.opMu.Unlock()
			}
			if expired {
				_ = p.Close()
				return
			}
		}
	}
}

func (s *Service) holdName() string {
	sum := sha256.Sum256([]byte(s.cfg.ExecutionID))
	return fmt.Sprintf("exhibit-%x", sum[:16])
}

// Caller serializes hold transitions with opMu; readOutput never performs I/O.
func (s *Service) acquireHold(ctx context.Context) error {
	if s.cfg.Holds == nil {
		return nil
	}
	s.mu.Lock()
	active := s.holdActive
	s.mu.Unlock()
	if active {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ttl, err := s.activeHoldTTL()
	if err != nil {
		return err
	}
	if err := s.cfg.Holds.Acquire(ctx, s.holdName(), ttl); err != nil {
		return err
	}
	s.mu.Lock()
	s.holdActive = true
	s.holdRenewAt = time.Now().Add(s.cfg.HoldTTL / 3)
	s.mu.Unlock()
	return nil
}

func (s *Service) activeHoldTTL() (time.Duration, error) {
	s.mu.Lock()
	remaining := time.Until(s.expires)
	s.mu.Unlock()
	ttl := s.cfg.HoldTTL
	if remaining < ttl {
		ttl = remaining.Truncate(time.Second)
	}
	if ttl < time.Second {
		return 0, errors.New("assigned lease has less than one second remaining")
	}
	return ttl, nil
}
func (s *Service) releaseHold(ctx context.Context) error {
	if s.cfg.Holds == nil {
		return nil
	}
	s.mu.Lock()
	active := s.holdActive
	s.mu.Unlock()
	if !active {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.cfg.Holds.Release(ctx, s.holdName()); err != nil {
		return err
	}
	s.mu.Lock()
	s.holdActive = false
	s.mu.Unlock()
	return nil
}

func (s *Service) signalLocked() { close(s.changed); s.changed = make(chan struct{}) }
func writeJSON(w http.ResponseWriter, value any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	writeJSON(w, WireError{Code: code, Message: message})
}
func decodeJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, 413, "payload_too_large", "worker payload exceeds limit")
		} else {
			writeError(w, 400, "invalid_json", "invalid worker request")
		}
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_json", "expected one JSON value")
		return false
	}
	return true
}

// CheckPiVersion is used by the executable before accepting assignments.
func CheckPiVersion(ctx context.Context, bin string) error {
	cmd := exec.CommandContext(ctx, bin, "--version")
	data, err := cmd.Output()
	if err != nil {
		return errors.New("unable to check Pi runtime version")
	}
	if strings.TrimSpace(string(data)) != PiVersion {
		return fmt.Errorf("worker requires Pi %s", PiVersion)
	}
	return nil
}
