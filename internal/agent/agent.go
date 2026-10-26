// Package agent manages Pi sidecar processes (Exh-m4ym, av-q3wo). Each chat
// session spawns one `pi --mode rpc` subprocess — Mario Zechner's agent
// harness speaking strict JSONL over stdin/stdout — loaded with only the
// exhibit tools extension, so everything the model saves flows through the
// exhibit HTTP API (the single write path). The user's decrypted provider key
// is handed to the subprocess through its environment and never appears in
// argv, page JS, or the datastore.
//
// A session is a mixture of two kinds of text and keeps them apart on purpose
// (av-e0yj). Instructions — the system prompt and the user's own messages —
// are authored by Exhibit and by the person at the keyboard. Everything else
// (artifact sources, artifact titles, stored state, picked page elements) is
// untrusted: URL ingest stores remote pages verbatim, so a hostile page can end
// up writing it. Untrusted text never occupies the system or user role and
// never gets spliced into a sentence; it reaches the model only as the result
// of a tool the model calls, which the conversation's own structure marks as
// data. Containment, not that marking, is the actual wall: the session
// authenticates with an agentscope credential that reaches exactly one
// artifact.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
)

// Config for the Manager.
type Config struct {
	// Runner selects execution transport. Nil retains the in-process local mode.
	// A remote Runner requires neither WorkRoot nor PiBin on the coordinator.
	Runner agentexec.Runner
	// MaxSessions bounds concurrent starts/processes and retained failed drains
	// across all owners. Zero defaults to 32; the per-owner limit remains ten.
	MaxSessions int
	// ReserveExecution runs after local admission and before provisioning/spawn.
	// It lets the durable coordinator reserve/fence this attempt and replace the
	// tool token with a narrowly scoped execution grant. A failed startup also
	// releases its reservation. Successful attempts release after final checkpoint.
	ReserveExecution func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error)

	PiBin      string // pi executable, e.g. "pi"
	WorkRoot   string // scratch root; per-session cwd + the materialized extension
	APIBaseURL string // exhibit app origin the extension calls back into
	// Credentials mints each session's scoped API token. Required: the
	// sidecar authenticates with a per-session grant, never the operator's
	// service token (av-e0yj).
	Credentials  *agentscope.Registry
	MockLLMURL   string // when set, sessions may use the "exhibit-mock" provider
	IdleTimeout  time.Duration
	SystemPrompt string // optional override of the role prompt; empty uses the default
	// ThinkingLevel is the instance baseline for new and resumed sessions.
	// Empty defaults to medium. BYOK owners may override it in CreateOpts.
	ThinkingLevel string
	// HideModelIdentity strips the provider/model identifiers out of Pi's
	// event stream and persisted transcripts (av-siqf). It is set when the
	// instance supplies the credential itself, where the model is not the
	// user's to know; a BYOK instance leaves it off, because there the
	// identifiers describe a key the caller typed. See redact.go.
	HideModelIdentity bool
	// Guardrail, when set, loads ext/guard.ts into every session to screen
	// each user message for usage-policy violations on its own model (av-gust).
	Guardrail *Guardrail
	// Caps are the operator's spend ceilings (av-99f4), enforced for
	// platform-paid sessions only. The zero value enforces nothing.
	Caps SpendCaps
	// MaxRequestBodyBytes is the largest request body the exhibit API
	// accepts (av-ombn), handed to the extension as EXHIBIT_MAX_BODY_BYTES so
	// a tool refuses an oversized write before sending it, with a message the
	// model can act on. The API's 413 stays the enforcement; this only makes
	// the failure legible. Zero passes nothing, and the extension then learns
	// of the limit from the 413 alone.
	MaxRequestBodyBytes int64
}

// providerEnv maps a provider name to the env var pi reads its key from.
var providerEnv = agentexec.ProviderEnvironment

// KnownProvider reports whether the manager can route a key to provider.
func KnownProvider(p string) bool { _, ok := providerEnv[p]; return ok }

// MaxOwnerSessions is how many sessions one owner may hold open at once
// (av-99f4). It is what makes the spend-cap overshoot bound finite: at most
// one full run per open session, so at most MaxOwnerSessions runs past the
// budget. Widget-generate sessions count like any other.
const MaxOwnerSessions = 10

// MaxInstanceSessions is the default instance ceiling, including startup.
const MaxInstanceSessions = 32

var ErrInstanceSessionLimit = errors.New("this instance has reached its agent process limit — close a conversation or try again shortly")

// ErrSessionLimit is Create refusing: the owner is at MaxOwnerSessions and
// every one of them is mid-run. Its message is what the user sees, so it says
// what to do — close one, or wait for a running one to finish.
var ErrSessionLimit = fmt.Errorf("this instance keeps at most %d agent conversations open per account, and all of yours are busy — close one or wait for a running one to finish", MaxOwnerSessions)

// Manager owns all live sessions.
type Manager struct {
	cfg Config
	st  store.Store

	mu             sync.Mutex
	sessions       map[string]*Session
	processOwners  map[int64]int
	processCount   int
	starting       map[string]bool // conversation identities reserved before loading history
	shutdown       bool
	drainErr       error
	stopReaper     chan struct{}
	recoveries     map[string]*Session
	erasedOwners   map[int64]bool
	erasureTargets map[int64]map[string]agentexec.Reclaimer
}

// ExecutionReservation belongs to the core coordinator. Checkpoint must be
// idempotent and fenced by the identity passed to ReserveExecution. Release is
// invoked only after the final checkpoint succeeds and process output drains.
type ExecutionReservation struct {
	Grant         *agentscope.Grant
	Checkpoint    func(context.Context, ExecutionCheckpoint) error
	Release       func(context.Context) error
	ExpiresAt     time.Time
	TurnTimeout   time.Duration
	BeginClose    func(context.Context) error
	StartupFailed func(context.Context) error
	Started       func(agentexec.Process)
	BeforePrompt  func(context.Context) (time.Time, error)
	Resume        *ResumeSnapshot
	UsageBaseline *store.UsageTotals
}

// ResumeSnapshot is loaded by the durable reservation, after it fences the
// conversation. It replaces a direct transcript read for assigned continuations.
type ResumeSnapshot struct {
	SessionFile    string
	Title          string
	Kind           string
	Revision       int64
	PreviousStatus string
	Interrupted    bool
}

const InterruptedResumeNotice = "The previous run was interrupted. Its last successful saves remain saved. This conversation resumes from its last acknowledged snapshot; unfinished work has not been replayed."

// New constructs the durable coordinator. Remote mode does no local filesystem
// preparation and performs no Pi/Node lookup.
func New(cfg Config, st store.Store) (*Manager, error) {
	if cfg.Credentials == nil {
		return nil, fmt.Errorf("agent manager needs a credential registry")
	}
	if cfg.MaxSessions < 0 {
		return nil, fmt.Errorf("agent maximum sessions must be positive (or zero for the default)")
	}
	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = MaxInstanceSessions
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 30 * time.Minute
	}
	if cfg.ThinkingLevel == "" {
		cfg.ThinkingLevel = DefaultThinkingLevel
	}
	m := &Manager{cfg: cfg, st: st, sessions: map[string]*Session{}, recoveries: map[string]*Session{}, erasedOwners: map[int64]bool{}, erasureTargets: map[int64]map[string]agentexec.Reclaimer{}, stopReaper: make(chan struct{})}
	go m.reap()
	return m, nil
}

// ImageContent is one inline image attached to a prompt, in Pi's RPC shape.
type ImageContent struct {
	Type     string `json:"type"` // always "image"
	Data     string `json:"data"` // base64, no data: prefix
	MimeType string `json:"mimeType"`
}

// CreateOpts describes a new session.
type CreateOpts struct {
	OwnerID  int64
	Provider string
	Model    string
	APIKey   string // decrypted, handed to the subprocess env only
	// ThinkingLevel is the BYOK owner's preference; empty uses the instance
	// baseline. Platform-paid sessions always use the instance baseline.
	ThinkingLevel string
	// ArtifactID non-empty means modify mode: the session is scoped to that
	// artifact, and the agent reads its source with get_artifact — nothing about
	// the artifact is put in the prompt, because a URL-ingested artifact carries
	// the remote page's title and markup verbatim and untrusted text reaches the
	// model only as a tool result. Empty means create mode — the session binds
	// to whatever its first create returns.
	ArtifactID string
	// WidgetOnly scopes the session to building this artifact's gallery
	// widget and nothing else (av-fafu) — the one-shot sessions behind the
	// edit page's "Generate widget" button. It exists because the ordinary
	// edit-an-artifact instruction tells the model to save with
	// write_artifact, which is exactly the wrong thing here: the artifact's
	// own source must not change.
	WidgetOnly bool
	// PlatformPaid marks a session running on the instance's own credential
	// (av-siqf) rather than a BYO key: it is who the provider bills, and so
	// the one question av-99f4's enforcement keys off (a BYO-key session
	// spends its owner's money and is never limited here). Recorded on the
	// usage rows as paid_by either way (av-2yws).
	PlatformPaid bool
	// ResumeSessionID continues a conversation kept earlier instead of starting one
	// (av-b4yh). The session takes the conversation's identity — same id, so the
	// same stored record keeps growing, the same label on the versions it
	// writes, and the same usage ledger entry its spend ceiling counts — and Pi
	// starts from the stored session file, so the model has the conversation as
	// it was. The prompt is the current one, not the stored one: an instruction
	// improved since is in force, and the file's copy is not replayed.
	// Everything else about the session is as for any other: a fresh
	// credential, the owner's current key, the artifact as it is now.
	ResumeSessionID string
}

// ErrSessionLive means a resume named a conversation that already has a live
// session. The API answers a request for a live conversation with that session,
// so this is only what losing a race for the same conversation looks like.
var ErrSessionLive = errors.New("that conversation is already running")

// Create decrypted-key session: spawns the pi subprocess and starts its reader.
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*Session, error) {
	_, ok := providerEnv[opts.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
	if opts.Provider == "exhibit-mock" && m.cfg.MockLLMURL == "" {
		return nil, fmt.Errorf("mock provider is not enabled on this server")
	}
	id := opts.ResumeSessionID
	if id == "" {
		id = uuid.NewString()
	}
	if opts.ResumeSessionID != "" {
		if !m.reserveConversation(id) {
			return nil, ErrSessionLive
		}
		defer m.releaseConversationStart(id)
	}
	releaseProcess, err := m.reserveProcess(ctx, opts.OwnerID)
	if err != nil {
		return nil, err
	}
	spawned := false
	defer func() {
		if !spawned {
			releaseProcess()
		}
	}()
	// The core issues the artifact grant. A central execution reservation can
	// fence/replace it before the runner acquires any infrastructure or process.
	grant, err := m.cfg.Credentials.Issue(opts.OwnerID, opts.ArtifactID, id)
	if err != nil {
		return nil, err
	}
	identity := agentexec.Identity{ExecutionID: uuid.NewString(), SessionID: id, OwnerID: opts.OwnerID, ArtifactID: opts.ArtifactID, Resume: opts.ResumeSessionID != ""}
	var reservation *ExecutionReservation
	defer func() {
		if !spawned {
			m.cfg.Credentials.Revoke(grant)
			if reservation != nil && reservation.StartupFailed != nil {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := reservation.StartupFailed(releaseCtx); err != nil {
					slog.Warn("release failed execution reservation", slog.String("execution_id", identity.ExecutionID))
				}
			}
		}
	}()
	if m.cfg.ReserveExecution != nil {
		reservation, err = m.cfg.ReserveExecution(ctx, identity, grant)
		if err != nil {
			return nil, err
		}
	}
	if reservation != nil && reservation.Grant != nil && reservation.Grant != grant {
		replacement := reservation.Grant
		if replacement.SessionID() != id || replacement.Scope() != (agentscope.Scope{OwnerID: opts.OwnerID, ArtifactID: opts.ArtifactID}) {
			m.cfg.Credentials.Revoke(replacement)
			return nil, fmt.Errorf("execution reservation returned an incompatible scoped grant")
		}
		m.cfg.Credentials.Revoke(grant)
		grant = replacement
	}
	var resume *store.Transcript
	var recorded store.UsageTotals
	title := ""
	var resumeMetadata *ResumeSnapshot
	if opts.ResumeSessionID != "" {
		// Both durable and process-local identity are now reserved. Never load a
		// continuation snapshot or its accounting baseline before fencing admission.
		if reservation != nil && reservation.Resume != nil {
			metadata := *reservation.Resume
			resumeMetadata = &metadata
			resume = &store.Transcript{SessionID: id, ArtifactID: opts.ArtifactID, Title: metadata.Title, SessionFile: metadata.SessionFile, Resumable: metadata.SessionFile != ""}
		} else {
			resume, err = m.st.GetTranscript(ctx, opts.OwnerID, opts.ArtifactID, id)
		}
		if err != nil {
			return nil, fmt.Errorf("read the conversation to resume: %w", err)
		}
		if resume == nil || !resume.Resumable {
			return nil, fmt.Errorf("conversation is no longer available to resume")
		}
		title = resume.Title
		if reservation != nil && reservation.UsageBaseline != nil {
			recorded = *reservation.UsageBaseline
		} else {
			recorded, err = m.st.SessionAgentSpend(ctx, opts.OwnerID, id)
		}
		if err != nil {
			return nil, fmt.Errorf("read the conversation's recorded usage: %w", err)
		}
	}
	token := grant.Token()
	config := agentexec.StartConfig{Identity: identity, Provider: opts.Provider, Model: opts.Model, APIKey: opts.APIKey,
		APIBaseURL: m.cfg.APIBaseURL, ToolToken: token, TargetArtifactID: opts.ArtifactID,
		SystemPrompt: buildSystemPrompt(m.cfg.SystemPrompt, opts), MockLLMURL: m.cfg.MockLLMURL,
		WidgetOnly: opts.WidgetOnly, MaxRequestBodyBytes: m.cfg.MaxRequestBodyBytes}
	resumeNotice := ""
	if resume != nil {
		config.SessionSnapshot = resume.SessionFile
		resumeNotice = resumedNotice
		if resumeMetadata != nil && (resumeMetadata.Interrupted || resumeMetadata.Kind == "interrupted" || resumeMetadata.PreviousStatus == "interrupted") {
			resumeNotice += "\n\n" + InterruptedResumeNotice
		}
	}
	if guard := m.cfg.Guardrail; guard != nil {
		config.Guardrail = &agentexec.Guardrail{Provider: guard.Provider, Model: guard.Model, APIKey: guard.APIKey}
	}
	if reservation != nil {
		config.ExpiresAt = reservation.ExpiresAt
		config.TurnTimeout = reservation.TurnTimeout
	}
	config.ThinkingLevel = m.cfg.ThinkingLevel
	if !opts.PlatformPaid && opts.ThinkingLevel != "" {
		config.ThinkingLevel = opts.ThinkingLevel
	}
	runner := m.cfg.Runner
	if runner == nil {
		runner = agentexec.Local{PiBin: m.cfg.PiBin, WorkRoot: m.cfg.WorkRoot}
	}
	process, err := runner.Start(ctx, config)
	if err != nil {
		return nil, err
	}
	spawned = true
	var workDir, sessionFile string
	var cmd *exec.Cmd // compatibility metadata for local in-process test helpers
	if local, ok := process.(*agentexec.LocalProcess); ok {
		workDir, sessionFile, cmd = local.WorkDir, local.SessionPath, local.Command
	}
	s := &Session{
		ID:                id,
		OwnerID:           opts.OwnerID,
		provider:          opts.Provider,
		model:             opts.Model,
		paidBy:            paidByFor(opts.PlatformPaid),
		workDir:           workDir,
		sessionFile:       sessionFile,
		title:             title,
		resumeNotice:      resumeNotice,
		usageRecorded:     recorded,
		readDone:          make(chan struct{}),
		processExited:     make(chan struct{}),
		grant:             grant,
		hideModelIdentity: m.cfg.HideModelIdentity,
		mgr:               m,
		process:           process,
		executionIdentity: identity,
		reservation:       reservation,
		retired:           make(chan struct{}),
		resumeMetadata:    resumeMetadata,
		cmd:               cmd,
		stdin:             process,
		subs:              map[*eventSubscriber]struct{}{},
		pending:           map[string]chan rpcResponse{},
		done:              make(chan struct{}),
		lastActive:        time.Now(),
		prepared:          true,
	}
	s.preparationTimer = time.AfterFunc(PreparedSessionLifetime, s.expirePreparation)
	if synchronizer, ok := process.(agentexec.EventSynchronizer); ok {
		s.eventHandled = synchronizer.SynchronizeEvents()
	}
	// Publish the session before its process-exit cleanup can retire it. That
	// cleanup takes the same mutex, including when Pi exits during startup.
	m.mu.Lock()
	go s.readLoop(process.Stdout())
	go s.drainStderr(process.Stderr())
	go func() {
		_ = process.Wait()
		<-s.readDone
		releaseProcess()
		close(s.processExited)
		s.finish()
	}()

	if _, live := m.sessions[id]; live {
		m.mu.Unlock()
		s.kill()
		return nil, ErrSessionLive
	}
	m.sessions[id] = s
	shuttingDown := m.shutdown
	ownerErased := m.erasedOwners[opts.OwnerID]
	if ownerErased {
		s.erased.Store(true)
	}
	m.mu.Unlock()
	if shuttingDown || ownerErased {
		s.kill()
		return nil, fmt.Errorf("agent manager shutting down")
	}
	if reservation != nil && reservation.Started != nil {
		reservation.Started(process)
	}
	if s.InterruptionNotice() != "" {
		event, _ := json.Marshal(map[string]string{"type": "exhibit_execution_interrupted", "message": InterruptedResumeNotice})
		s.broadcast(event)
	}
	slog.InfoContext(ctx, "agent session started",
		slog.String("session_id", id),
		slog.String("provider", opts.Provider),
		slog.String("model", opts.Model),
		slog.String("artifact_id", opts.ArtifactID),
		slog.Bool("resumed", resume != nil),
	)
	return s, nil
}

// reserveConversation excludes both a process draining its final writes and a
// competing startup. The global mutex protects only registry operations, never
// database reads, process startup, or waiting for persistence.
func (m *Manager) reserveConversation(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shutdown || m.sessions[id] != nil || m.recoveries[id] != nil || m.starting[id] {
		return false
	}
	if m.starting == nil {
		m.starting = make(map[string]bool)
	}
	m.starting[id] = true
	return true
}

func (m *Manager) releaseConversationStart(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.starting, id)
}

// Get returns one of ownerID's live sessions, or nil.
//
// The owner is a *parameter* rather than something the caller compares
// afterwards, for the reason av-ep8k made owner_id a predicate inside every
// Store query instead of a pre-check in every handler: a predicate a caller can
// forget is a predicate that gets forgotten. This registry is the one piece of
// per-owner state in the service that SQL never sees, so it was the one place
// av-ep8k's sweep could not reach — and it held a *live, credentialed* object,
// which is worse than a row. A stranger holding a session id could steer
// somebody else's agent, and the tool calls that followed ran on the victim's
// own scoped credential (§5.1), writing into the victim's artifact.
//
// A session belonging to another owner is therefore not "found and refused", it
// is simply not found: that is what lets the routes above answer a stranger
// exactly as they answer a session id that was never issued, rather than
// becoming an oracle over which ids are live.
//
// Owner 0 — a request nobody attributed (api.noOwner) — matches nothing here
// for the same reason it matches no row: owner ids start at 1.
func (m *Manager) Get(ownerID int64, id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || s.OwnerID != ownerID {
		return nil
	}
	s.mu.Lock()
	closed := s.closed || s.stopping
	s.mu.Unlock()
	if closed {
		return nil
	}
	return s
}

// Close terminates one of ownerID's sessions. Its identity stays reserved until
// the reader and final persistence finish. Scoped for the
// same reason Get is, and more sharply: closing kills a subprocess, so an
// unscoped Close is a one-request denial of service against anybody whose
// session id leaks. Another owner's id is a no-op.
func (m *Manager) Close(ownerID int64, id string) {
	m.mu.Lock()
	s := m.sessions[id]
	if s != nil && s.OwnerID == ownerID {
		s.markStopping()
	} else {
		s = nil
	}
	m.mu.Unlock()
	if s != nil {
		s.kill()
	}
}

// reserveProcess counts startup and live processes under one mutex. Capacity
// is returned only after Wait reaps the process, before final persistence.
func (m *Manager) reserveProcess(ctx context.Context, ownerID int64) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.Lock()
		if m.shutdown || m.erasedOwners[ownerID] {
			m.mu.Unlock()
			return nil, fmt.Errorf("agent manager shutting down")
		}
		if m.processOwners == nil {
			m.processOwners = make(map[int64]int)
		}
		recoveryCount := 0
		for _, recovery := range m.recoveries {
			if recovery.OwnerID == ownerID {
				recoveryCount++
			}
		}
		if m.processOwners[ownerID]+recoveryCount < MaxOwnerSessions && m.processCount+len(m.recoveries) < m.cfg.MaxSessions {
			m.processOwners[ownerID]++
			m.processCount++
			m.mu.Unlock()
			return func() {
				m.mu.Lock()
				m.processOwners[ownerID]--
				if m.processOwners[ownerID] == 0 {
					delete(m.processOwners, ownerID)
				}
				m.processCount--
				m.mu.Unlock()
			}, nil
		}
		refusal := ErrSessionLimit
		if m.processOwners[ownerID]+recoveryCount < MaxOwnerSessions {
			refusal = ErrInstanceSessionLimit
		}
		_, oldest := m.ownerSessionsLocked(ownerID, "")
		if oldest == nil {
			m.mu.Unlock()
			return nil, refusal
		}
		oldest.mu.Lock()
		if oldest.streaming || oldest.closed || oldest.stopping {
			oldest.mu.Unlock()
			m.mu.Unlock()
			continue
		}
		oldest.stopping = true
		oldest.mu.Unlock()
		m.mu.Unlock()
		oldest.kill()
		select {
		case <-oldest.processExited:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// ownerSessionsLocked counts ownerID's open sessions and finds the oldest
// idle one (not streaming) among them, excluding skipID. Caller holds m.mu.
func (m *Manager) ownerSessionsLocked(ownerID int64, skipID string) (count int, oldestIdle *Session) {
	var oldestAt time.Time
	for _, s := range m.sessions {
		if s.OwnerID != ownerID || s.ID == skipID {
			continue
		}
		s.mu.Lock()
		if s.closed || s.stopping {
			s.mu.Unlock()
			continue
		}
		count++
		idle := !s.streaming
		last := s.lastActive
		s.mu.Unlock()
		if !idle {
			continue
		}
		if oldestIdle == nil || last.Before(oldestAt) {
			oldestIdle, oldestAt = s, last
		}
	}
	return count, oldestIdle
}

// reap closes sessions idle longer than the configured timeout.
func (m *Manager) reap() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reapIdle(time.Now().Add(-m.cfg.IdleTimeout))
		case <-m.stopReaper:
			return
		}
	}
}

// Shutdown refuses new work, stops all executions, and waits for final snapshots,
// persistence, reservation release, and scratch cleanup. The caller bounds drain.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if !m.shutdown {
		m.shutdown = true
		close(m.stopReaper)
	}
	all := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.mu.Unlock()
	for _, s := range all {
		go s.kill()
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		m.mu.Lock()
		empty := len(m.sessions) == 0 && m.processCount == 0 && len(m.starting) == 0
		m.mu.Unlock()
		if empty {
			m.mu.Lock()
			err := m.drainErr
			m.mu.Unlock()
			return err
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *Manager) reapIdle(cutoff time.Time) {
	m.mu.Lock()
	var stale []*Session
	for _, s := range m.sessions {
		s.mu.Lock()
		// A model waiting on a person is not doing work. If the browser
		// disappears without delivering pagehide, this session must still
		// expire instead of holding a subprocess and an owner slot forever.
		idle := !s.closed && !s.stopping && s.lastActive.Before(cutoff) && (!s.streaming || s.hasPendingQuestions())
		if idle {
			s.stopping = true
		}
		s.mu.Unlock()
		if idle {
			stale = append(stale, s)
		}
	}
	m.mu.Unlock()
	for _, s := range stale {
		slog.Info("reaping idle agent session", slog.String("session_id", s.ID))
		s.kill()
	}
}

// PreparedSessionLifetime bounds unclaimed sidecars even when the browser
// disappears before it receives their startup response.
const PreparedSessionLifetime = 90 * time.Second

func (s *Session) expirePreparation() {
	s.mu.Lock()
	expired := s.prepared && !s.closed && !s.stopping
	if expired && s.promptInFlight {
		s.preparationExpired = true
		s.mu.Unlock()
		return
	}
	if expired {
		s.stopping = true
	}
	s.mu.Unlock()
	if expired {
		s.kill()
	}
}

// Finish admission after the input guard has answered. A prompt admitted near
// the preparation deadline gets to finish screening, but a refusal cannot
// extend that deadline or claim the normal idle lifetime.
func (s *Session) finishPromptAdmission() {
	s.mu.Lock()
	s.promptInFlight = false
	expired := s.prepared && s.preparationExpired
	s.mu.Unlock()
	if expired {
		s.expirePreparation()
	}
}

// Session is one live pi subprocess plus its event fanout.
type Session struct {
	ID      string
	OwnerID int64

	// provider and model are the configured credential's, kept for the usage
	// ledger (av-2yws) — rows record what spent the money even though
	// platform mode strips it from everything a user sees. paidBy is who the
	// provider bills: PaidByPlatform for the instance's credential, PaidByUser
	// for a BYO key.
	provider string
	model    string
	paidBy   string

	// workDir is the session's private scratch directory — the subprocess's
	// HOME, its session file, and where the server hands it the one thing that
	// cannot travel in a prompt: the elements the user selected
	// (selection.json). It is removed when the session ends.
	workDir string
	// sessionFile is where Pi records the conversation, in workDir.
	sessionFile string
	// persistMu serializes persistTranscript, so two settles close together
	// cannot store the older read after the newer one; persisting counts the
	// ones in flight, so the scratch directory outlives them.
	persistMu          sync.Mutex
	checkpointSequence uint64 // guarded by persistMu
	persisting         sync.WaitGroup
	// readDone closes when everything the subprocess printed has been handled.
	readDone chan struct{}
	// processExited closes after Wait returns and its admission slot is released.
	processExited chan struct{}
	// grant is the session's API credential and the single source of truth
	// for which artifact it may touch. In create mode it starts unbound and
	// the API's create handler binds it — the session never derives its
	// artifact from tool output, which the model's arguments shape.
	grant *agentscope.Grant
	// hideModelIdentity is the manager's platform-mode setting, copied at
	// construction so the two seams that publish Pi's protocol read it off
	// the session itself rather than reaching back through the manager
	// (av-siqf, redact.go).
	hideModelIdentity bool

	process               agentexec.Process
	eventHandled          func()
	executionIdentity     agentexec.Identity
	reservation           *ExecutionReservation
	resumeMetadata        *ResumeSnapshot
	beginCloseMu          sync.Mutex
	beginCloseDone        bool
	beginCloseErr         error
	drainMu               sync.Mutex
	retainedErr           error
	erased                atomic.Bool
	drainComplete         bool
	terminalStopped       bool
	terminalCheckpoint    *ExecutionCheckpoint
	terminalCheckpointAck bool
	authorityReleased     bool
	retired               chan struct{}
	mgr                   *Manager
	cmd                   *exec.Cmd
	stdin                 io.WriteCloser

	writeMu sync.Mutex // serializes stdin writes
	// promptMu keeps one prompt in flight at a time, from send to response.
	// Pi runs a prompt's input hook (the guardrail screen) before answering
	// it, and the guard's signal names no prompt, so serializing is what ties
	// guardBlocked to the one prompt that set it (av-gust). A second send
	// waits at most one screen.
	promptMu sync.Mutex

	mu    sync.Mutex // guards everything below
	title string     // the conversation's first prompt, shortened
	// resumeNotice is the sentence the first prompt of a resumed conversation
	// carries (prompt.go); it is held until a prompt has actually landed.
	resumeNotice       string
	subs               map[*eventSubscriber]struct{}
	backlog            [][]byte
	backlogBytes       int
	pending            map[string]chan rpcResponse
	questions          map[string]QuestionRequest
	streaming          bool
	interrupted        bool
	eventFailure       error
	closed             bool
	stopping           bool // unavailable to callers, but still draining its final writes
	lastActive         time.Time
	prepared           bool
	promptInFlight     bool
	preparationExpired bool
	preparationTimer   *time.Timer
	// guardBlocked records that the guard extension handled the prompt in
	// flight instead of running it (av-gust): the model never saw it. Pi emits
	// that signal before the prompt's response, so Prompt can read it once the
	// response arrives; promptMu guarantees there is only one prompt it can
	// belong to.
	guardBlocked bool

	// Usage accounting (av-2yws). usageCur is the cumulative usage of the
	// assistant response in flight (Pi reports it per response, streaming);
	// usageRecorded is the sum of durable ledger rows for this session's own
	// model/tool/compaction spend — guardrail rows excluded, since Pi's
	// totals never count them.
	usageCur       Usage
	usageCurActive bool
	usageRecorded  store.UsageTotals
	// reconcileMu keeps one settle reconciliation running at a time, so each
	// one's stats snapshot is taken after the previous one's gap row landed.
	reconcileMu sync.Mutex

	// Spend cap state (av-99f4). turnTimer is the per-turn wall-clock
	// ceiling, the one hard stop besides the instance ceiling.
	turnTimer *time.Timer

	done chan struct{}
}

// rpcResponse is one correlated Pi response plus the session's recorded usage
// at the moment the read loop reached it. Pi writes its stdout in order, so
// every usage event it emitted before answering has been recorded by then, and
// none it emitted after has: that snapshot is the baseline a get_session_stats
// answer can be compared against (av-2yws).
type rpcResponse struct {
	line     json.RawMessage
	recorded store.UsageTotals
}

// ArtifactID is the artifact this session is scoped to, or "" while a
// create-mode session has yet to save anything.
func (s *Session) ArtifactID() string { return s.grant.Scope().ArtifactID }

// Done is closed when the subprocess exits.
func (s *Session) Done() <-chan struct{} { return s.done }

// InterruptionNotice lets core render the prior execution's status without
// exposing worker files or inventing an automatic retry.
func (s *Session) InterruptionNotice() string {
	if r := s.resumeMetadata; r != nil && (r.Interrupted || r.Kind == "interrupted" || r.PreviousStatus == "interrupted") {
		return InterruptedResumeNotice
	}
	return ""
}
func (s *Session) SetLease(ctx context.Context, deadline time.Time) error {
	if lease, ok := s.process.(agentexec.LeaseProcess); ok {
		return lease.SetLease(ctx, deadline)
	}
	return errors.New("execution runner does not support lease renewal")
}

// Subscribe replays retained events, then streams live events. Excess clients
// receive a closed channel. Unsubscribe releases its queue and worker.
func (s *Session) Subscribe() (<-chan []byte, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := newEventSubscriber()
	if len(s.subs) >= maxSubscribers {
		close(sub.events)
		return sub.events, func() {}
	}
	for _, ev := range s.backlog {
		sub.enqueue(ev)
	}
	s.subs[sub] = struct{}{}
	go sub.run(s.done)
	return sub.events, func() {
		s.mu.Lock()
		sub.stop()
		// Release retained bytes before making capacity available to a new tab.
		<-sub.finished
		delete(s.subs, sub)
		s.mu.Unlock()
	}
}

// selectionFile is the file, in the session's work directory, that carries the
// elements the user selected to the subprocess. The extension's get_selection
// tool is told its path through EXHIBIT_SELECTION_FILE.
const selectionFile = agentexec.SelectionFile

// Prompt sends a user prompt, optionally with images and the elements the user
// selected in the artifact preview. If the agent is mid-stream the message is
// queued as a steering message.
//
// message is the user's own words and travels as-is. A selection is untrusted —
// an element's markup is the artifact's own, which a URL ingest took verbatim
// from a remote page — so none of it goes into the prompt: it is handed to the
// subprocess through a file, the prompt says only that elements were selected
// (a fixed sentence), and the model reads them with get_selection, which
// returns them as a tool result.
func (s *Session) Prompt(ctx context.Context, message string, images []ImageContent, selection []string) error {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.mu.Lock()
	waiting := s.hasPendingQuestions()
	s.mu.Unlock()
	if waiting {
		return ErrQuestionPending
	}

	s.mu.Lock()
	if s.closed || s.stopping {
		s.mu.Unlock()
		return fmt.Errorf("agent session closed")
	}
	s.promptInFlight = true
	s.mu.Unlock()
	defer s.finishPromptAdmission()

	// The between-turns gate (av-99f4), inside promptMu so two concurrent
	// prompts cannot both pass it. It runs before the send because a session
	// over budget must not pay for the guardrail screen either.
	if err := s.capBeforePrompt(); err != nil {
		return err
	}
	if err := s.prepareExecution(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	steer := s.streaming
	s.lastActive = time.Now()
	s.guardBlocked = false
	s.mu.Unlock()
	// The artifact versions the agent writes while answering this are labelled
	// with it (api.versionProvenance reads it off the grant).
	s.grant.SetPrompt(message)

	if err := s.writeSelection(selection); err != nil {
		return err
	}
	text := message
	if len(selection) > 0 {
		text += "\n\n" + selectionNotice(len(selection))
	}
	s.mu.Lock()
	notice := s.resumeNotice
	s.mu.Unlock()
	if notice != "" {
		text += "\n\n" + notice
	}

	cmd := map[string]any{"type": "prompt", "message": text}
	if len(images) > 0 {
		cmd["images"] = images
	}
	if steer {
		cmd["streamingBehavior"] = "steer"
	}

	resp, err := s.roundTrip(ctx, cmd)
	if err != nil {
		return err
	}
	var r struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return err
	}
	if !r.Success {
		return fmt.Errorf("prompt rejected: %s", r.Error)
	}
	s.promptLanded(message)
	return nil
}

// promptLanded records what a prompt reaching the agent settles: the
// conversation takes its name from the first one, and a resumed conversation
// has now been told it was resumed. A prompt the guardrail refused never
// reached the agent, so it settles neither — its words are not in the stored
// conversation, and the sentence it carried was never read.
func (s *Session) promptLanded(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.guardBlocked {
		return
	}
	s.claimPreparationLocked()
	if s.title == "" {
		s.title = ConversationTitle(message)
	}
	s.resumeNotice = ""
}

// The caller holds s.mu. agent_start also claims the session: its accepted
// input must survive even if the HTTP caller disconnects before RPC replies.
func (s *Session) claimPreparationLocked() {
	s.prepared = false
	if s.preparationTimer != nil {
		s.preparationTimer.Stop()
		s.preparationTimer = nil
	}
}

// writeSelection makes selection.json say exactly what the user selected with
// this prompt: the file is replaced, or removed when nothing was selected, so
// get_selection can never return the previous prompt's elements for a prompt
// that selected none. Written to a temporary name and renamed, so the
// subprocess never reads half a file.
func (s *Session) writeSelection(selection []string) error {
	if s.process == nil {
		return agentexec.WriteSelection(s.workDir, selection)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.process.WriteSelection(ctx, selection)
}

func (s *Session) snapshot(ctx context.Context) (string, error) {
	if s.process != nil {
		return s.process.Snapshot(ctx)
	}
	return readSessionFile(s.sessionFile)
}

// Abort asks pi to stop the current run.
func (s *Session) Abort(ctx context.Context) error {
	_, err := s.roundTrip(ctx, map[string]any{"type": "abort"})
	s.cancelQuestions("")
	return err
}

// roundTrip sends one RPC command and waits for its correlated response.
func (s *Session) roundTrip(ctx context.Context, cmd map[string]any) (json.RawMessage, error) {
	resp, err := s.call(ctx, cmd)
	return resp.line, err
}

// call is roundTrip keeping the recorded-usage snapshot beside the response.
func (s *Session) call(ctx context.Context, cmd map[string]any) (rpcResponse, error) {
	id := uuid.New().String()
	cmd["id"] = id
	ch := make(chan rpcResponse, 1)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return rpcResponse{}, fmt.Errorf("session closed")
	}
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	line, err := json.Marshal(cmd)
	if err != nil {
		return rpcResponse{}, err
	}
	s.writeMu.Lock()
	_, err = s.stdin.Write(append(line, '\n'))
	s.writeMu.Unlock()
	if err != nil {
		return rpcResponse{}, fmt.Errorf("write to pi: %w", err)
	}

	timeout := time.NewTimer(2 * time.Minute)
	defer timeout.Stop()
	select {
	case resp := <-ch:
		return resp, nil
	case <-s.done:
		return rpcResponse{}, fmt.Errorf("agent process exited")
	case <-timeout.C:
		return rpcResponse{}, fmt.Errorf("timed out waiting for pi response")
	case <-ctx.Done():
		return rpcResponse{}, ctx.Err()
	}
}

// readLoop consumes pi's stdout: correlates responses, tracks streaming
// state, detects artifact saves, and broadcasts every event to subscribers.
func (s *Session) readLoop(stdout io.Reader) {
	defer close(s.readDone)
	reader := bufio.NewReaderSize(stdout, 64<<10)
	for {
		line, oversized, err := readEventLine(reader)
		if oversized {
			slog.Warn("discarded oversized pi event", slog.String("session_id", s.ID), slog.Int("limit_bytes", maxEventLineBytes))
		} else if len(line) > 0 {
			s.handleLine(bytes.TrimRight(line, "\r\n"))
		}
		s.mu.Lock()
		failure := s.eventFailure
		s.mu.Unlock()
		if failure != nil {
			message := "Execution stopped because its durable checkpoint could not be acknowledged. Unfinished work was not replayed."
			event, _ := json.Marshal(map[string]string{"type": "extension_error", "error": message})
			s.broadcast(event)
			if transport, ok := s.process.(interface{ FailEvents(error) }); ok {
				transport.FailEvents(errors.New(message))
			}
			s.kill()
			return
		}
		if (oversized || len(line) > 0) && s.eventHandled != nil {
			s.eventHandled()
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) handleLine(line []byte) {
	var probe struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		ToolName   string `json:"toolName"`
		ToolCallID string `json:"toolCallId"`
		IsError    bool   `json:"isError"`
		// Usage, Message and Result are raw, and that is load-bearing. This
		// one probe parses every line Pi emits, and shapes collide across
		// event types: "message" is a *string* on an extension_ui_request
		// notification and an *object* on message_end, and a tool's result
		// can carry anything at all. A typed field here made json.Unmarshal
		// fail on the whole line, which silently dropped guard signals
		// before guardSignalOf ever saw them. Whatever is not understood
		// must cost the extraction, never the line.
		Usage   json.RawMessage `json:"usage"`
		Message json.RawMessage `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		slog.Debug("unparseable pi output", slog.String("session_id", s.ID), slog.String("line", truncate(string(line), 200)))
		return
	}

	if probe.Type == "extension_ui_request" {
		var message string
		if json.Unmarshal(probe.Message, &message) == nil && message == "exhibit_source:prepared" {
			s.broadcast([]byte(`{"type":"exhibit_source_prepared"}`))
			return
		}
	}
	if verb, detail, ok := guardSignalOf(line); ok {
		s.handleGuardSignal(verb, detail)
		return
	}
	if probe.Type == "extension_ui_request" && s.receiveQuestions(line) {
		return
	}

	if probe.Type == "response" && probe.ID != "" {
		s.mu.Lock()
		ch := s.pending[probe.ID]
		recorded := s.usageRecorded
		s.mu.Unlock()
		if ch != nil {
			// copy: line's backing array is reused by the reader
			ch <- rpcResponse{line: json.RawMessage(bytes.Clone(line)), recorded: recorded}
		}
		return
	}

	switch probe.Type {
	case "agent_start":
		s.mu.Lock()
		s.claimPreparationLocked()
		if s.title == "" {
			s.title = ConversationTitle(s.grant.Prompt())
		}
		s.streaming = true
		s.lastActive = time.Now()
		s.mu.Unlock()
	case "message_update":
		if u, ok := decodeUsage(probe.Usage); ok {
			s.noteStreamingUsage(u)
		}
	case "message_end":
		// The authoritative final message: role decides the row's source,
		// provider/model name the spend, usage is the meter.
		var msg struct {
			Role         string          `json:"role"`
			Provider     string          `json:"provider"`
			Model        string          `json:"model"`
			StopReason   string          `json:"stopReason"`
			ErrorMessage string          `json:"errorMessage"`
			Usage        json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(probe.Message, &msg) == nil {
			if msg.Role == "assistant" && msg.StopReason == "error" {
				// Keep provider diagnostics server-side before hosted events are
				// redacted. Log only the error, never the conversation payload.
				slog.Error("agent provider request failed", slog.String("session_id", s.ID),
					slog.String("provider", msg.Provider), slog.String("model", msg.Model),
					slog.String("err", msg.ErrorMessage))
			}
			if u, ok := decodeUsage(msg.Usage); ok {
				s.noteMessageUsage(msg.Role, msg.Provider, msg.Model, u)
			}
			if msg.Role == "toolResult" && s.reservation != nil {
				s.checkpointEvent("tool")
			}
		}
	case "compaction_end":
		var res struct {
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(probe.Result, &res) == nil {
			if u, ok := decodeUsage(res.Usage); ok {
				s.noteCompactionUsage(u)
			}
		}
	case "turn_start":
		s.armTurnClock()
	case "turn_end":
		s.clearTurnClock()
	case "agent_settled":
		s.cancelQuestions("")
		s.mu.Lock()
		s.streaming = false
		s.lastActive = time.Now()
		s.mu.Unlock()
		s.clearTurnClock()
		// The settle is where the ledger is squared with Pi's own totals —
		// usage sources that emit no event (tool-reported usage, cache
		// warming) are recorded here as the difference (av-2yws). The
		// response in flight is flushed here, on the read loop, so the flush
		// can only ever see this turn's response and never a later one's.
		s.flushInFlightUsage()
		go s.reconcileUsage()
		if s.reservation != nil {
			s.checkpointEvent("settled")
		} else if artifactID := s.ArtifactID(); artifactID != "" {
			s.persisting.Add(1)
			go func() {
				defer s.persisting.Done()
				s.persistTranscript(artifactID)
			}()
		}
	case "tool_execution_end":
		if probe.ToolCallID != "" {
			s.cancelQuestions(probe.ToolCallID)
		}
		var res struct {
			Details map[string]any `json:"details"`
		}
		if !probe.IsError && json.Unmarshal(probe.Result, &res) == nil {
			switch res.Details["exhibit"] {
			case "questions_answered":
				if res.Details["blocked"] == true {
					requestID, _ := res.Details["requestId"].(string)
					s.blockQuestionReply(requestID, probe.ToolCallID)
				}
			case "artifact_saved":
				s.noteArtifactSaved(res.Details)
			case "state_changed":
				s.noteStateChanged(res.Details)
			case "widget_saved":
				s.noteWidgetSaved(res.Details)
			}
		}
	}

	s.broadcast(s.redact(line))
}

func (s *Session) checkpointEvent(kind string) {
	if err := s.persistCheckpoint(s.ArtifactID(), kind); err != nil {
		s.mu.Lock()
		s.eventFailure = err
		s.interrupted = true
		s.mu.Unlock()
	}
}

// redact applies the platform-mode filter to one event line on its way out of
// this process. It is the one place the manager's HideModelIdentity setting is
// consulted.
func (s *Session) redact(doc []byte) []byte {
	if !s.hideModelIdentity {
		return doc
	}
	return redactModelIdentity(doc)
}

// noteArtifactSaved emits the synthetic event the chat UI uses to re-render
// the live preview, after a create/update tool call lands.
//
// The id comes from the session's grant, which the API's create handler bound
// from the row it wrote — not from the tool result, whose contents are shaped
// by model-supplied arguments. A session therefore cannot be talked into
// pointing its own preview, transcript, or scope at somebody else's artifact
// (av-e0yj). The same is true of the two note* functions below: all three read
// s.ArtifactID(), so the session has exactly one notion of what it is working
// on and nothing the model emits can move it.
func (s *Session) noteArtifactSaved(details map[string]any) {
	artifactID := s.ArtifactID()
	if artifactID == "" {
		slog.Warn("agent reported a save with no artifact bound to the session",
			slog.String("session_id", s.ID))
		return
	}
	ev, _ := json.Marshal(map[string]any{
		"type":       "exhibit_artifact_saved",
		"artifactId": artifactID,
		"action":     details["action"],
		"title":      details["title"],
		"renderUrl":  details["renderUrl"],
		"footprint":  details["footprint"],
	})
	s.broadcast(ev)
}

// noteStateChanged emits a synthetic event when a set_state/delete_state tool
// call lands. State is inlined into the artifact document at render time, so
// the preview iframe is stale after an edit until something re-renders it —
// the chat UI reuses the exact htmx swap exhibit_artifact_saved already
// drives (docs/agent.md "Preview re-render") rather than inventing a second
// refresh path.
func (s *Session) noteStateChanged(details map[string]any) {
	artifactID := s.ArtifactID()
	if artifactID == "" {
		return
	}
	ev, _ := json.Marshal(map[string]any{
		"type":       "exhibit_state_changed",
		"artifactId": artifactID,
		"action":     details["action"],
		"key":        details["key"],
	})
	s.broadcast(ev)
}

// noteWidgetSaved is the set_widget counterpart (av-fafu). It stays a distinct
// event rather than reusing exhibit_artifact_saved because the two mean
// different things to the chat UI — the artifact's live preview did not change,
// only the tile beside it — and because a widget save carries its own warning:
// origins the artifact's allowlist does not cover, which the browser will block.
func (s *Session) noteWidgetSaved(details map[string]any) {
	artifactID := s.ArtifactID()
	if artifactID == "" {
		return
	}
	ev, _ := json.Marshal(map[string]any{
		"type":       "exhibit_widget_saved",
		"artifactId": artifactID,
		"widgetUrl":  details["widgetUrl"],
		"unapproved": details["unapproved"],
	})
	s.broadcast(ev)
}

// ExecutionCheckpoint describes complete Pi records and why they were captured.
// Assigned runs use the callback as their only transcript persistence path.
type ExecutionCheckpoint struct {
	SessionFile  string
	Title        string
	Kind         string // "tool", "settled", or "interrupted"
	Sequence     uint64 // Monotonic within this execution attempt, including final flush.
	BaseRevision int64
}

func (s *Session) persistTranscript(artifactID string) {
	_ = s.persistCheckpoint(artifactID, "settled")
}

func (s *Session) persistCheckpoint(artifactID, kind string) error {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	if s.erased.Load() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	checkpoint, err := s.captureCheckpoint(ctx, kind)
	if err == nil {
		err = s.writeCheckpoint(ctx, artifactID, checkpoint)
	}
	if err != nil {
		slog.Warn("execution checkpoint failed", slog.String("session_id", s.ID), slog.String("kind", kind))
	}
	return err
}

// Caller serializes captures with persistMu. A terminal drain retains this DTO
// across lost acknowledgments rather than assigning a new sequence on retry.
func (s *Session) captureCheckpoint(ctx context.Context, kind string) (*ExecutionCheckpoint, error) {
	file, err := s.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	s.checkpointSequence++
	s.mu.Lock()
	title := s.title
	s.mu.Unlock()
	checkpoint := &ExecutionCheckpoint{SessionFile: file, Title: title, Kind: kind, Sequence: s.checkpointSequence}
	if s.resumeMetadata != nil {
		checkpoint.BaseRevision = s.resumeMetadata.Revision
	}
	return checkpoint, nil
}
func (s *Session) writeCheckpoint(ctx context.Context, artifactID string, checkpoint *ExecutionCheckpoint) error {
	if s.erased.Load() {
		return nil
	}
	if s.reservation != nil {
		if s.reservation.Checkpoint == nil {
			return fmt.Errorf("assigned execution has no durable checkpoint callback")
		}
		return s.reservation.Checkpoint(ctx, *checkpoint)
	}
	if checkpoint.SessionFile != "" && artifactID != "" {
		return s.mgr.st.SaveTranscript(ctx, s.OwnerID, store.Transcript{ArtifactID: artifactID, SessionID: s.ID, Title: checkpoint.Title, SessionFile: checkpoint.SessionFile})
	}
	return nil
}

// maxSessionFileBytes bounds what is kept of one conversation. A session file
// holds every tool result in full — each read of the artifact is a copy of it —
// so it grows with the artifact as well as with the talk, and a conversation
// that outgrows this stops being kept rather than growing the database without
// limit. The live session is unaffected.
const maxSessionFileBytes = agentexec.MaxSessionBytes

func readSessionFile(path string) (string, error) { return agentexec.ReadSessionFile(path) }
func readSessionContents(reader io.Reader, limit int64) (string, error) {
	return agentexec.ReadSessionContents(reader, limit)
}

func (s *Session) broadcast(line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(line) > maxEventLineBytes {
		return
	}
	// Own the retained bytes: a trimmed record may otherwise keep a much
	// larger reader allocation alive than the byte budget accounts for.
	line = bytes.Clone(line)
	s.backlog = append(s.backlog, line)
	s.backlogBytes += len(line)
	for len(s.backlog) > maxBacklog || s.backlogBytes > maxEventQueueBytes {
		s.backlogBytes -= len(s.backlog[0])
		s.backlog[0] = nil
		s.backlog = s.backlog[1:]
	}
	for sub := range s.subs {
		sub.enqueue(line)
	}
}

// drainStderr surfaces pi's stderr in the server log.
func (s *Session) drainStderr(stderr io.Reader) {
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		slog.Debug("pi stderr", slog.String("session_id", s.ID), slog.String("line", sc.Text()))
	}
}

// finish marks the session closed after subprocess exit and tells subscribers.
func (s *Session) finish() {
	s.mu.Lock()
	if s.preparationTimer != nil {
		s.preparationTimer.Stop()
		s.preparationTimer = nil
	}
	s.mu.Unlock()
	// Whatever the response in flight had reported is flushed first: a
	// subprocess killed mid-turn never sends message_end, and that spend is
	// exactly the spend this ticket exists to attribute (av-2yws).
	s.clearTurnClock()
	s.flushInFlightUsage()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.interrupted = s.interrupted || s.streaming || s.promptInFlight
	s.streaming = false
	s.mu.Unlock()
	s.cancelQuestions("")
	// The credential dies with the process that held it.
	s.mgr.cfg.Credentials.Revoke(s.grant)
	ev, _ := json.Marshal(map[string]string{"type": "exhibit_session_closed"})
	s.broadcast(ev)
	close(s.done)
	go s.removeScratch()
}

// removeScratch deletes the session's work directory once nothing can still
// need it: the process has exited, everything it printed has been read, and the
// last settled turn is stored. The conversation's home is the database; the
// directory held a working copy, and leaving it behind would keep a conversation
// on disk after the account that owned it was erased.
func (s *Session) removeScratch() {
	<-s.readDone
	s.persisting.Wait()
	_ = s.finalizeExecution()
	if s.retired != nil {
		close(s.retired)
	}
}

// finalizeExecution deliberately retains both the process snapshot and the
// assignment on any failed close/checkpoint/release. RetryDrain is explicit;
// it never replays a prompt and never allows an earlier resume past this file.
func (s *Session) finalizeExecution() error {
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	if s.drainComplete {
		return nil
	}
	if s.erased.Load() {
		return s.finalizeErasure()
	}
	drainErr := s.beginClose()
	if drainErr == nil && !s.terminalStopped {
		if s.process != nil {
			drainErr = s.process.Close()
		}
		if drainErr == nil {
			s.terminalStopped = true
		}
	}
	if drainErr == nil && !s.terminalCheckpointAck && !s.erased.Load() {
		s.persistMu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if s.terminalCheckpoint == nil {
			kind := "settled"
			s.mu.Lock()
			if s.interrupted {
				kind = "interrupted"
			}
			s.mu.Unlock()
			s.terminalCheckpoint, drainErr = s.captureCheckpoint(ctx, kind)
		}
		if drainErr == nil {
			drainErr = s.writeCheckpoint(ctx, s.ArtifactID(), s.terminalCheckpoint)
		}
		if drainErr == nil {
			s.terminalCheckpointAck = true
		}
		cancel()
		s.persistMu.Unlock()
	}
	if s.erased.Load() {
		return s.finalizeErasure()
	}
	if drainErr == nil && !s.authorityReleased {
		if reservation := s.reservation; reservation != nil && reservation.Release != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			drainErr = reservation.Release(ctx)
			cancel()
		}
		if drainErr == nil {
			s.authorityReleased = true
		}
	}
	if drainErr == nil && s.process != nil {
		drainErr = s.process.Cleanup()
	}
	s.mgr.mu.Lock()
	if s.mgr.recoveries == nil {
		s.mgr.recoveries = map[string]*Session{}
	}
	if drainErr != nil {
		s.retainedErr = drainErr
		s.mgr.recoveries[s.ID] = s
		slog.Warn("execution retained for recovery", slog.String("execution_id", s.executionIdentity.ExecutionID))
	} else {
		s.retainedErr = nil
		s.terminalCheckpoint = nil
		s.drainComplete = true
		delete(s.mgr.recoveries, s.ID)
	}
	s.mgr.refreshDrainErrorLocked()
	if s.mgr.sessions[s.ID] == s {
		delete(s.mgr.sessions, s.ID)
	}
	s.mgr.mu.Unlock()
	return drainErr
}

// After account deletion, retain only a cleanup capability, never the Session
// (which owns chat events, snapshots, prompts, grants and policy callbacks).
// Caller holds drainMu and has waited for process/event delivery to stop.
func (s *Session) finalizeErasure() error {
	var target agentexec.Reclaimer
	if s.process != nil {
		target = s.process.ErasureCleanup()
	}
	s.terminalCheckpoint = nil
	s.mu.Lock()
	s.backlog, s.questions = nil, nil
	s.backlogBytes = 0
	s.mu.Unlock()
	s.mgr.mu.Lock()
	defer s.mgr.mu.Unlock()
	if target != nil {
		if s.mgr.erasureTargets == nil {
			s.mgr.erasureTargets = map[int64]map[string]agentexec.Reclaimer{}
		}
		if s.mgr.erasureTargets[s.OwnerID] == nil {
			s.mgr.erasureTargets[s.OwnerID] = map[string]agentexec.Reclaimer{}
		}
		s.mgr.erasureTargets[s.OwnerID][s.ID] = target
	}
	s.retainedErr = nil
	s.drainComplete = true
	delete(s.mgr.recoveries, s.ID)
	if s.mgr.sessions[s.ID] == s {
		delete(s.mgr.sessions, s.ID)
	}
	s.mgr.refreshDrainErrorLocked()
	return nil
}
func (m *Manager) refreshDrainErrorLocked() {
	m.drainErr = nil
	for _, recovery := range m.recoveries {
		m.drainErr = errors.Join(m.drainErr, recovery.retainedErr)
	}
}

// RetainedSnapshot is a trusted coordinator recovery read, owner-scoped like
// Get. It is never a browser/tool transcript API and does not release anything.
func (m *Manager) RetainedSnapshot(ctx context.Context, ownerID int64, id string) (string, error) {
	m.mu.Lock()
	s := m.recoveries[id]
	m.mu.Unlock()
	if s == nil || s.OwnerID != ownerID {
		return "", fmt.Errorf("retained execution not found")
	}
	s.drainMu.Lock()
	defer s.drainMu.Unlock()
	return s.snapshot(ctx)
}

// RetryDrain retries terminal stop/read, persistence, release and reclamation
// from the last acknowledged phase. It never replays model execution.
func (m *Manager) RetryDrain(ctx context.Context, ownerID int64, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	s := m.recoveries[id]
	m.mu.Unlock()
	if s == nil || s.OwnerID != ownerID {
		return fmt.Errorf("retained execution not found")
	}
	return s.finalizeExecution()
}

func (s *Session) kill() {
	s.mu.Lock()
	if s.preparationTimer != nil {
		s.preparationTimer.Stop()
		s.preparationTimer = nil
	}
	s.mu.Unlock()
	s.markStopping()
	if !s.erased.Load() {
		_ = s.beginClose()
	}
	s.mgr.cfg.Credentials.Revoke(s.grant)
	_ = s.stdin.Close()
	if s.process != nil {
		_ = s.process.Kill()
		return
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

// EraseOwner is destructive and MUST be called only after successful durable
// account deletion. It blocks owner admission, revokes/stops live and retained
// executions, and reclaims their snapshots WITHOUT checkpointing deleted data.
// Ordinary close and RetryDrain retain their fail-closed durability behavior.
// On cleanup failure only a payload-free reclaim capability is retained;
// subsequent calls retry it. No final checkpoint or authority Release is used.
func (m *Manager) EraseOwner(ctx context.Context, ownerID int64) error {
	m.mu.Lock()
	if m.erasedOwners == nil {
		m.erasedOwners = map[int64]bool{}
	}
	m.erasedOwners[ownerID] = true
	all := map[*Session]bool{}
	for _, s := range m.sessions {
		if s.OwnerID == ownerID {
			s.erased.Store(true)
			all[s] = false
		}
	}
	for _, s := range m.recoveries {
		if s.OwnerID == ownerID {
			s.erased.Store(true)
			all[s] = true
		}
	}
	m.mu.Unlock()
	for s := range all {
		m.cfg.Credentials.Revoke(s.grant)
		go func(s *Session) { s.kill(); <-s.done; s.persisting.Wait(); _ = s.finalizeExecution() }(s)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		m.mu.Lock()
		remaining := m.processOwners[ownerID]
		for _, s := range m.sessions {
			if s.OwnerID == ownerID {
				remaining++
			}
		}
		for _, s := range m.recoveries {
			if s.OwnerID == ownerID {
				remaining++
			}
		}
		targets := map[string]agentexec.Reclaimer{}
		if remaining == 0 {
			for id, target := range m.erasureTargets[ownerID] {
				targets[id] = target
			}
		}
		m.mu.Unlock()
		if remaining == 0 {
			var failures []error
			for id, target := range targets {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := target.Cleanup(); err != nil {
					failures = append(failures, err)
					continue
				}
				m.mu.Lock()
				delete(m.erasureTargets[ownerID], id)
				if len(m.erasureTargets[ownerID]) == 0 {
					delete(m.erasureTargets, ownerID)
				}
				m.mu.Unlock()
			}
			return errors.Join(failures...)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Session) beginClose() error {
	s.beginCloseMu.Lock()
	defer s.beginCloseMu.Unlock()
	if s.beginCloseDone {
		return nil
	}
	if s.reservation != nil && s.reservation.BeginClose != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.beginCloseErr = s.reservation.BeginClose(ctx)
	}
	if s.beginCloseErr == nil {
		s.beginCloseDone = true
	}
	return s.beginCloseErr
}

func (s *Session) markStopping() {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// paidByFor names the provider-billing side of a session for the usage
// ledger.
func paidByFor(platformPaid bool) string {
	if platformPaid {
		return store.PaidByPlatform
	}
	return store.PaidByUser
}

func (s *Session) prepareExecution(ctx context.Context) error {
	if s.reservation != nil && s.reservation.BeforePrompt != nil {
		deadline, err := s.reservation.BeforePrompt(ctx)
		if err != nil {
			return err
		}
		if !deadline.IsZero() {
			return s.SetLease(ctx, deadline)
		}
	}
	return nil
}
