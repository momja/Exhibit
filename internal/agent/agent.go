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
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
)

//go:embed ext/exhibit.ts ext/edit.ts ext/guard.ts
var extFS embed.FS

// Config for the Manager.
type Config struct {
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
}

// providerEnv maps a provider name to the env var pi reads its key from.
var providerEnv = map[string]string{
	"anthropic":    "ANTHROPIC_API_KEY",
	"openai":       "OPENAI_API_KEY",
	"google":       "GEMINI_API_KEY",
	"openrouter":   "OPENROUTER_API_KEY",
	"opencode-go":  "OPENCODE_API_KEY",
	"exhibit-mock": "EXHIBIT_MOCK_API_KEY",
}

// KnownProvider reports whether the manager can route a key to provider.
func KnownProvider(p string) bool { _, ok := providerEnv[p]; return ok }

// MaxOwnerSessions is how many sessions one owner may hold open at once
// (av-99f4). It is what makes the spend-cap overshoot bound finite: at most
// one full run per open session, so at most MaxOwnerSessions runs past the
// budget. Widget-generate sessions count like any other.
const MaxOwnerSessions = 10

// ErrSessionLimit is Create refusing: the owner is at MaxOwnerSessions and
// every one of them is mid-run. Its message is what the user sees, so it says
// what to do — close one, or wait for a running one to finish.
var ErrSessionLimit = fmt.Errorf("this instance keeps at most %d agent conversations open per account, and all of yours are busy — close one or wait for a running one to finish", MaxOwnerSessions)

// Manager owns all live sessions.
type Manager struct {
	cfg       Config
	st        store.Store
	extPath   string
	guardPath string

	mu       sync.Mutex
	sessions map[string]*Session
}

// New materializes the extension (exhibit.ts plus its edit-engine import,
// edit.ts, which pi's jiti loader resolves relative to the extension file)
// under cfg.WorkRoot and starts the idle reaper.
func New(cfg Config, st store.Store) (*Manager, error) {
	if cfg.Credentials == nil {
		return nil, fmt.Errorf("agent manager needs a credential registry")
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 30 * time.Minute
	}
	if err := os.MkdirAll(cfg.WorkRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create agent work root: %w", err)
	}
	src, err := extFS.ReadFile("ext/exhibit.ts")
	if err != nil {
		return nil, err
	}
	extPath := filepath.Join(cfg.WorkRoot, "exhibit.ts")
	if err := os.WriteFile(extPath, src, 0o644); err != nil {
		return nil, fmt.Errorf("materialize exhibit extension: %w", err)
	}
	// edit.ts is exhibit.ts's only relative import (the edit_artifact engine,
	// av-f5i5). It must sit beside the extension or the sidecar fails to load.
	editSrc, err := extFS.ReadFile("ext/edit.ts")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg.WorkRoot, "edit.ts"), editSrc, 0o644); err != nil {
		return nil, fmt.Errorf("materialize exhibit edit engine: %w", err)
	}
	// guard.ts is materialized whether or not a guardrail is configured; it is
	// loaded into a session only when one is (av-gust).
	guardSrc, err := extFS.ReadFile("ext/guard.ts")
	if err != nil {
		return nil, err
	}
	guardPath := filepath.Join(cfg.WorkRoot, "guard.ts")
	if err := os.WriteFile(guardPath, guardSrc, 0o644); err != nil {
		return nil, fmt.Errorf("materialize guard extension: %w", err)
	}
	m := &Manager{cfg: cfg, st: st, extPath: extPath, guardPath: guardPath, sessions: map[string]*Session{}}
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
	// Resume continues a conversation kept earlier instead of starting one
	// (av-b4yh). The session takes the conversation's identity — same id, so the
	// same stored record keeps growing, the same label on the versions it
	// writes, and the same usage ledger entry its spend ceiling counts — and Pi
	// starts from the stored session file, so the model has the conversation as
	// it was. The prompt is the current one, not the stored one: an instruction
	// improved since is in force, and the file's copy is not replayed.
	// Everything else about the session is as for any other: a fresh
	// credential, the owner's current key, the artifact as it is now.
	Resume *store.Transcript
}

// ErrSessionLive means a resume named a conversation that already has a live
// session. The API answers a request for a live conversation with that session,
// so this is only what losing a race for the same conversation looks like.
var ErrSessionLive = errors.New("that conversation is already running")

// Create decrypted-key session: spawns the pi subprocess and starts its reader.
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*Session, error) {
	envKey, ok := providerEnv[opts.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
	if opts.Provider == "exhibit-mock" && m.cfg.MockLLMURL == "" {
		return nil, fmt.Errorf("mock provider is not enabled on this server")
	}
	// The session limit (av-99f4), enforced here for both creators. At the
	// limit the owner's oldest *idle* session is evicted — the chat page
	// closes its session on pagehide, but a user who reloads before that
	// lands must not be locked out for the idle timeout — and a new session
	// is refused only when every one of them is mid-run.
	if !m.admitOwner(opts.OwnerID) {
		return nil, ErrSessionLimit
	}

	id := uuid.New().String()
	title := ""
	var recorded store.UsageTotals
	if opts.Resume != nil {
		id, title = opts.Resume.SessionID, opts.Resume.Title
		// Pi's own totals count the whole conversation, the turns before this
		// process included, and every settle reconciles the ledger against
		// them (usage.go). The ledger already holds those turns, so this
		// process's count of what is recorded starts from them — from zero the
		// first settle would record them a second time.
		var err error
		if recorded, err = m.st.SessionAgentSpend(ctx, opts.OwnerID, id); err != nil {
			return nil, fmt.Errorf("read the conversation's recorded usage: %w", err)
		}
	}
	// The scratch directory belongs to this process, not to the conversation: a
	// conversation can be resumed again while the process that last ran it is
	// still being cleaned up, and the two must not share a directory.
	workDir := filepath.Join(m.cfg.WorkRoot, uuid.New().String())
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	// The credential is what actually confines this session: it resolves to
	// (owner, artifact) and the API refuses everything else. The subprocess
	// never sees the operator's service token (av-e0yj).
	grant, err := m.cfg.Credentials.Issue(opts.OwnerID, opts.ArtifactID, id)
	if err != nil {
		return nil, err
	}
	spawned := false
	defer func() {
		if !spawned {
			m.cfg.Credentials.Revoke(grant) // no live subprocess ⇒ no live token
			_ = os.RemoveAll(workDir)       // and nothing to keep the directory for
		}
	}()
	sysPrompt := buildSystemPrompt(m.cfg.SystemPrompt, opts)
	sessionFile := filepath.Join(workDir, sessionFileName)
	resumeNotice := ""
	if opts.Resume != nil {
		// What the database holds is the conversation as of its last settled
		// turn, which is the right place to resume from: a turn that was still
		// running when the last session ended never finished, and is not
		// something to continue.
		if err := os.WriteFile(sessionFile, []byte(opts.Resume.SessionFile), 0o600); err != nil {
			return nil, fmt.Errorf("restore session file: %w", err)
		}
		resumeNotice = resumedNotice
	}

	args := []string{
		"--mode", "rpc",
		// Pi records the conversation in its own session file, which is what
		// the service keeps (persistTranscript) and what a resumed session
		// starts from.
		"--session", sessionFile,
		"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-themes", "--no-context-files",
		"--no-builtin-tools",
		"-e", m.extPath,
		"--provider", opts.Provider,
		"--system-prompt", sysPrompt,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if m.cfg.Guardrail != nil {
		args = append(args, "-e", m.guardPath)
	}

	cmd := exec.Command(m.cfg.PiBin, args...) //nolint:gosec // args are server-constructed
	// Every session runs from the same directory, and it is the root. A session
	// file records the directory it ran in, and Pi refuses to resume one whose
	// directory no longer exists — which a per-session directory never survives:
	// it is removed when the session ends (removeScratch), and a conversation is
	// resumed long after. The session has no use for a working directory of its
	// own anyway: it has no built-in tools and loads no context files.
	cmd.Dir = sessionCwd
	// Minimal environment: enough for node + jiti, the exhibit callback
	// contract, and exactly one provider key. Deliberately NOT os.Environ():
	// the server's own env must not leak other credentials into a session.
	// HOME is pinned to the session workdir so pi cannot read the operator's
	// ~/.pi/agent/auth.json — stored logins there would otherwise take
	// precedence over the BYO key and silently bill the operator's account.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + workDir,
		"LANG=" + os.Getenv("LANG"),
		"TMPDIR=" + os.TempDir(),
		"EXHIBIT_API_URL=" + m.cfg.APIBaseURL,
		// Scoped to this session's artifact, not the service token.
		"EXHIBIT_TOKEN=" + grant.Token(),
		// The tools' target, so none of them needs an id parameter.
		"EXHIBIT_ARTIFACT_ID=" + opts.ArtifactID,
		"EXHIBIT_SESSION_ID=" + id,
		// Where get_selection finds the elements the user picked (Prompt writes
		// it): one definition of the path, so the two sides cannot disagree.
		"EXHIBIT_SELECTION_FILE=" + filepath.Join(workDir, selectionFile),
		envKey + "=" + opts.APIKey,
	}
	if m.cfg.MockLLMURL != "" {
		cmd.Env = append(cmd.Env, "EXHIBIT_MOCK_LLM_URL="+m.cfg.MockLLMURL)
	}
	if m.cfg.Guardrail != nil {
		cmd.Env = append(cmd.Env, m.cfg.Guardrail.env()...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pi: %w", err)
	}
	spawned = true

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
		grant:             grant,
		hideModelIdentity: m.cfg.HideModelIdentity,
		mgr:               m,
		cmd:               cmd,
		stdin:             stdin,
		subs:              map[chan []byte]struct{}{},
		pending:           map[string]chan rpcResponse{},
		done:              make(chan struct{}),
		lastActive:        time.Now(),
	}
	go s.readLoop(stdout)
	go s.drainStderr(stderr)
	go func() {
		_ = cmd.Wait()
		s.finish()
	}()

	m.mu.Lock()
	if _, live := m.sessions[id]; live {
		m.mu.Unlock()
		s.kill()
		return nil, ErrSessionLive
	}
	m.sessions[id] = s
	// Two Creates can pass admitOwner at once; the loser gives up its slot
	// here, so "at most MaxOwnerSessions" holds even under a race.
	others, evicted := m.ownerSessionsLocked(opts.OwnerID, id)
	if others >= MaxOwnerSessions {
		if evicted != nil {
			delete(m.sessions, evicted.ID)
		}
	}
	m.mu.Unlock()
	if others >= MaxOwnerSessions {
		if evicted != nil {
			evicted.kill()
		} else {
			m.Close(opts.OwnerID, id)
			return nil, ErrSessionLimit
		}
	}
	slog.InfoContext(ctx, "agent session started",
		slog.String("session_id", id),
		slog.String("provider", opts.Provider),
		slog.String("model", opts.Model),
		slog.String("artifact_id", opts.ArtifactID),
		slog.Bool("resumed", opts.Resume != nil),
	)
	return s, nil
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
	return s
}

// Close terminates one of ownerID's sessions and forgets it. Scoped for the
// same reason Get is, and more sharply: closing kills a subprocess, so an
// unscoped Close is a one-request denial of service against anybody whose
// session id leaks. Another owner's id is a no-op.
func (m *Manager) Close(ownerID int64, id string) {
	m.mu.Lock()
	s := m.sessions[id]
	if s != nil && s.OwnerID == ownerID {
		delete(m.sessions, id)
	} else {
		s = nil
	}
	m.mu.Unlock()
	if s != nil {
		s.kill()
	}
}

// admitOwner enforces MaxOwnerSessions for one more session of ownerID,
// making room by evicting the owner's oldest idle session at the limit. It
// says no only when every session is mid-run: a busy conversation is not
// ours to kill, while an idle one costs its owner nothing to lose — and the
// alternative (refusing outright) would lock a reload-storming user out for
// the whole idle timeout.
func (m *Manager) admitOwner(ownerID int64) bool {
	m.mu.Lock()
	count, oldest := m.ownerSessionsLocked(ownerID, "")
	if count < MaxOwnerSessions {
		m.mu.Unlock()
		return true
	}
	if oldest == nil {
		m.mu.Unlock()
		return false
	}
	delete(m.sessions, oldest.ID)
	m.mu.Unlock()
	// What reap does for a stale session: the credential dies with the
	// process, and finish() flushes whatever it had spent.
	oldest.kill()
	return true
}

// ownerSessionsLocked counts ownerID's open sessions and finds the oldest
// idle one (not streaming) among them, excluding skipID. Caller holds m.mu.
func (m *Manager) ownerSessionsLocked(ownerID int64, skipID string) (count int, oldestIdle *Session) {
	var oldestAt time.Time
	for _, s := range m.sessions {
		if s.OwnerID != ownerID || s.ID == skipID {
			continue
		}
		count++
		s.mu.Lock()
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
	for range time.Tick(time.Minute) {
		cutoff := time.Now().Add(-m.cfg.IdleTimeout)
		m.mu.Lock()
		var stale []*Session
		for id, s := range m.sessions {
			s.mu.Lock()
			idle := s.lastActive.Before(cutoff) && !s.streaming
			s.mu.Unlock()
			if idle {
				stale = append(stale, s)
				delete(m.sessions, id)
			}
		}
		m.mu.Unlock()
		for _, s := range stale {
			slog.Info("reaping idle agent session", slog.String("session_id", s.ID))
			s.kill()
		}
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
	persistMu  sync.Mutex
	persisting sync.WaitGroup
	// readDone closes when everything the subprocess printed has been handled.
	readDone chan struct{}
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

	mgr   *Manager
	cmd   *exec.Cmd
	stdin io.WriteCloser

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
	resumeNotice string
	subs         map[chan []byte]struct{}
	backlog      [][]byte
	pending      map[string]chan rpcResponse
	streaming    bool
	closed       bool
	lastActive   time.Time
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

// maxBacklog bounds replayed events for late SSE subscribers.
const maxBacklog = 4096

// Done is closed when the subprocess exits.
func (s *Session) Done() <-chan struct{} { return s.done }

// Subscribe returns a channel receiving every event line (replaying the
// backlog first) and an unsubscribe func.
func (s *Session) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 1024)
	s.mu.Lock()
	for _, ev := range s.backlog {
		select {
		case ch <- ev:
		default:
		}
	}
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// selectionFile is the file, in the session's work directory, that carries the
// elements the user selected to the subprocess. The extension's get_selection
// tool is told its path through EXHIBIT_SELECTION_FILE.
const selectionFile = "selection.json"

// sessionFileName is Pi's session file in the work directory.
const sessionFileName = "session.jsonl"

// sessionCwd is the working directory of every session's subprocess.
const sessionCwd = "/"

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

	// The between-turns gate (av-99f4), inside promptMu so two concurrent
	// prompts cannot both pass it. It runs before the send because a session
	// over budget must not pay for the guardrail screen either.
	if err := s.capBeforePrompt(); err != nil {
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
	if s.title == "" {
		s.title = ConversationTitle(message)
	}
	s.resumeNotice = ""
}

// writeSelection makes selection.json say exactly what the user selected with
// this prompt: the file is replaced, or removed when nothing was selected, so
// get_selection can never return the previous prompt's elements for a prompt
// that selected none. Written to a temporary name and renamed, so the
// subprocess never reads half a file.
func (s *Session) writeSelection(selection []string) error {
	path := filepath.Join(s.workDir, selectionFile)
	if len(selection) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear selection: %w", err)
		}
		return nil
	}
	b, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write selection: %w", err)
	}
	return os.Rename(tmp, path)
}

// Abort asks pi to stop the current run.
func (s *Session) Abort(ctx context.Context) error {
	_, err := s.roundTrip(ctx, map[string]any{"type": "abort"})
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
	reader := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			s.handleLine(line)
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) handleLine(line []byte) {
	var probe struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		ToolName string `json:"toolName"`
		IsError  bool   `json:"isError"`
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

	if verb, detail, ok := guardSignalOf(line); ok {
		s.handleGuardSignal(verb, detail)
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
			Role     string          `json:"role"`
			Provider string          `json:"provider"`
			Model    string          `json:"model"`
			Usage    json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(probe.Message, &msg) == nil {
			if u, ok := decodeUsage(msg.Usage); ok {
				s.noteMessageUsage(msg.Role, msg.Provider, msg.Model, u)
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
		if artifactID := s.ArtifactID(); artifactID != "" {
			s.persisting.Add(1)
			go func() {
				defer s.persisting.Done()
				s.persistTranscript(artifactID)
			}()
		}
	case "tool_execution_end":
		var res struct {
			Details map[string]any `json:"details"`
		}
		if !probe.IsError && json.Unmarshal(probe.Result, &res) == nil {
			switch res.Details["exhibit"] {
			case "artifact_saved":
				s.noteArtifactSaved(res.Details)
			case "state_changed":
				s.noteStateChanged(res.Details)
			case "widget_saved":
				s.noteWidgetSaved(res.Details)
			}
		}
	}

	s.broadcast(s.redact(bytes.Clone(line)))
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

// persistTranscript stores the conversation with the artifact, tied to the
// version the artifact is at (av-y7td). Runs after each settled turn, so what is
// kept tracks the conversation as it grows, and is exactly what Pi would start
// from if the conversation were resumed: its own session file, read back.
func (s *Session) persistTranscript(artifactID string) {
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	file, err := readSessionFile(s.sessionFile)
	if err != nil {
		slog.Warn("transcript read failed", slog.String("session_id", s.ID), slog.String("err", err.Error()))
		return
	}
	if file == "" {
		return // Pi has written nothing yet
	}
	s.mu.Lock()
	title := s.title
	s.mu.Unlock()
	// The session's owner, not the artifact's: a transcript can only attach to
	// an artifact this session's owner actually holds, so an artifact id the
	// model invented (or lifted from another library) fails with ErrNotFound
	// instead of writing across the tenant boundary.
	err = s.mgr.st.SaveTranscript(ctx, s.OwnerID, store.Transcript{
		ArtifactID: artifactID, SessionID: s.ID, Title: title, SessionFile: file,
	})
	if err != nil {
		slog.Warn("transcript save failed", slog.String("session_id", s.ID), slog.String("err", err.Error()))
	}
}

// maxSessionFileBytes bounds what is kept of one conversation. A session file
// holds every tool result in full — each read of the artifact is a copy of it —
// so it grows with the artifact as well as with the talk, and a conversation
// that outgrows this stops being kept rather than growing the database without
// limit. The live session is unaffected.
const maxSessionFileBytes = 64 << 20

// readSessionFile returns Pi's session file as written so far. Pi appends one
// line per entry, so a read that races an append can end partway through a
// line, and half a line is not an entry: only whole lines are returned.
func readSessionFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		return "", nil
	}
	if end+1 > maxSessionFileBytes {
		return "", fmt.Errorf("session file is %d bytes, over the %d kept", end+1, maxSessionFileBytes)
	}
	return string(b[:end+1]), nil
}

func (s *Session) broadcast(line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backlog = append(s.backlog, line)
	if len(s.backlog) > maxBacklog {
		s.backlog = s.backlog[len(s.backlog)-maxBacklog:]
	}
	for ch := range s.subs {
		select {
		case ch <- line:
		default: // slow subscriber: drop rather than block the read loop
		}
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
	s.streaming = false
	s.mu.Unlock()
	// The credential dies with the process that held it.
	s.mgr.cfg.Credentials.Revoke(s.grant)
	close(s.done)
	ev, _ := json.Marshal(map[string]string{"type": "exhibit_session_closed"})
	s.broadcast(ev)
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
	if err := os.RemoveAll(s.workDir); err != nil {
		slog.Warn("remove session scratch directory", slog.String("session_id", s.ID), slog.String("err", err.Error()))
	}
}

func (s *Session) kill() {
	s.mgr.cfg.Credentials.Revoke(s.grant)
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
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
