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
// (artifact sources, artifact titles, picked page elements) is untrusted: URL
// ingest stores remote pages verbatim, so a hostile page can end up writing
// it. Untrusted text never occupies the system role and never gets spliced
// into a sentence; it travels in a fenced data block whose delimiter carries a
// per-session random nonce, so text inside a block cannot close the fence and
// impersonate an instruction. Containment, not the fence, is the actual wall:
// the session authenticates with an agentscope credential that reaches exactly
// one artifact.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
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

//go:embed ext/exhibit.ts
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

// Manager owns all live sessions.
type Manager struct {
	cfg     Config
	st      store.Store
	extPath string

	mu       sync.Mutex
	sessions map[string]*Session
}

// New materializes the extension under cfg.WorkRoot and starts the idle reaper.
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
	m := &Manager{cfg: cfg, st: st, extPath: extPath, sessions: map[string]*Session{}}
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
	// artifact and its source is inlined into the first prompt. Empty means
	// create mode — the session binds to whatever its first create returns.
	ArtifactID string
	// ArtifactTitle and ArtifactBody are untrusted (a URL-ingested artifact
	// carries the remote page's title and markup verbatim). They reach the
	// model only inside a fenced data block, never in the system prompt.
	ArtifactTitle string
	ArtifactBody  string
	// WidgetOnly scopes the session to building this artifact's gallery
	// widget and nothing else (av-fafu) — the one-shot sessions behind the
	// edit page's "Generate widget" button. It exists because the ordinary
	// edit-an-artifact instruction tells the model to save with
	// update_artifact, which is exactly the wrong thing here: the artifact's
	// own source must not change.
	WidgetOnly bool
}

// Create decrypted-key session: spawns the pi subprocess and starts its reader.
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (*Session, error) {
	envKey, ok := providerEnv[opts.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q", opts.Provider)
	}
	if opts.Provider == "exhibit-mock" && m.cfg.MockLLMURL == "" {
		return nil, fmt.Errorf("mock provider is not enabled on this server")
	}

	id := uuid.New().String()
	workDir := filepath.Join(m.cfg.WorkRoot, id)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, err
	}
	// The credential is what actually confines this session: it resolves to
	// (owner, artifact) and the API refuses everything else. The subprocess
	// never sees the operator's service token (av-e0yj).
	grant, err := m.cfg.Credentials.Issue(opts.OwnerID, opts.ArtifactID)
	if err != nil {
		return nil, err
	}
	spawned := false
	defer func() {
		if !spawned {
			m.cfg.Credentials.Revoke(grant) // no live subprocess ⇒ no live token
		}
	}()
	sysPrompt := buildSystemPrompt(m.cfg.SystemPrompt, nonce, opts)

	args := []string{
		"--mode", "rpc",
		"--no-session",
		"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-themes", "--no-context-files",
		"--no-builtin-tools",
		"-e", m.extPath,
		"--provider", opts.Provider,
		"--system-prompt", sysPrompt,
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	cmd := exec.Command(m.cfg.PiBin, args...) //nolint:gosec // args are server-constructed
	cmd.Dir = workDir
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
		// Fence id for untrusted tool output (get_artifact), matching the
		// contract stated in the system prompt.
		"EXHIBIT_DATA_NONCE=" + nonce,
		"EXHIBIT_SESSION_ID=" + id,
		envKey + "=" + opts.APIKey,
	}
	if m.cfg.MockLLMURL != "" {
		cmd.Env = append(cmd.Env, "EXHIBIT_MOCK_LLM_URL="+m.cfg.MockLLMURL)
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
		ID:         id,
		OwnerID:    opts.OwnerID,
		grant:      grant,
		nonce:      nonce,
		mgr:        m,
		cmd:        cmd,
		stdin:      stdin,
		subs:       map[chan []byte]struct{}{},
		pending:    map[string]chan json.RawMessage{},
		done:       make(chan struct{}),
		lastActive: time.Now(),
	}
	// Modify mode opens with the artifact's current source already in
	// context, so the agent does not spend a tool call reading what the
	// server just had in hand. get_artifact stays available for the re-read
	// after a save or a concurrent human edit.
	if opts.ArtifactID != "" {
		s.pendingData = []DataBlock{artifactSourceBlock(opts.ArtifactID, opts.ArtifactTitle, opts.ArtifactBody)}
	}
	go s.readLoop(stdout)
	go s.drainStderr(stderr)
	go func() {
		_ = cmd.Wait()
		s.finish()
	}()

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	slog.InfoContext(ctx, "agent session started",
		slog.String("session_id", id),
		slog.String("provider", opts.Provider),
		slog.String("model", opts.Model),
		slog.String("artifact_id", opts.ArtifactID),
	)
	return s, nil
}

// Get returns a live session or nil.
func (m *Manager) Get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Close terminates a session's subprocess and forgets it.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s != nil {
		s.kill()
	}
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

	// grant is the session's API credential and the single source of truth
	// for which artifact it may touch. In create mode it starts unbound and
	// the API's create handler binds it — the session never derives its
	// artifact from tool output, which the model's arguments shape.
	grant *agentscope.Grant
	// nonce fences untrusted text in this session's prompts.
	nonce string

	mgr   *Manager
	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex // serializes stdin writes

	mu          sync.Mutex // guards everything below
	pendingData []DataBlock
	subs        map[chan []byte]struct{}
	backlog     [][]byte
	pending     map[string]chan json.RawMessage
	streaming   bool
	closed      bool
	lastActive  time.Time

	done chan struct{}
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

// Prompt sends a user prompt (optionally with images and untrusted data
// blocks). If the agent is mid-stream the message is queued as a steering
// message.
//
// message is the user's own words and travels as-is. Every DataBlock — the
// artifact source a modify session opens with, an element picked in the
// preview — is fenced onto the end of the same user-role message, so no
// untrusted text ever reaches the model as an instruction.
func (s *Session) Prompt(ctx context.Context, message string, images []ImageContent, data []DataBlock) error {
	s.mu.Lock()
	steer := s.streaming
	// The session's opening block rides the first prompt. It is held, not
	// cleared, until the prompt actually lands — a rejected send must not
	// silently drop the artifact source from the conversation.
	blocks := append(append([]DataBlock{}, s.pendingData...), data...)
	s.lastActive = time.Now()
	s.mu.Unlock()

	cmd := map[string]any{"type": "prompt", "message": composePrompt(s.nonce, message, blocks)}
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
	s.mu.Lock()
	s.pendingData = nil
	s.mu.Unlock()
	return nil
}

// Abort asks pi to stop the current run.
func (s *Session) Abort(ctx context.Context) error {
	_, err := s.roundTrip(ctx, map[string]any{"type": "abort"})
	return err
}

// roundTrip sends one RPC command and waits for its correlated response.
func (s *Session) roundTrip(ctx context.Context, cmd map[string]any) (json.RawMessage, error) {
	id := uuid.New().String()
	cmd["id"] = id
	ch := make(chan json.RawMessage, 1)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("session closed")
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
		return nil, err
	}
	s.writeMu.Lock()
	_, err = s.stdin.Write(append(line, '\n'))
	s.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write to pi: %w", err)
	}

	timeout := time.NewTimer(2 * time.Minute)
	defer timeout.Stop()
	select {
	case resp := <-ch:
		return resp, nil
	case <-s.done:
		return nil, fmt.Errorf("agent process exited")
	case <-timeout.C:
		return nil, fmt.Errorf("timed out waiting for pi response")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readLoop consumes pi's stdout: correlates responses, tracks streaming
// state, detects artifact saves, and broadcasts every event to subscribers.
func (s *Session) readLoop(stdout io.Reader) {
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
		Result   struct {
			Details map[string]any `json:"details"`
		} `json:"result"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		slog.Debug("unparseable pi output", slog.String("session_id", s.ID), slog.String("line", truncate(string(line), 200)))
		return
	}

	if probe.Type == "response" && probe.ID != "" {
		s.mu.Lock()
		ch := s.pending[probe.ID]
		s.mu.Unlock()
		if ch != nil {
			// copy: line's backing array is reused by the reader
			ch <- json.RawMessage(bytes.Clone(line))
		}
		return
	}

	switch probe.Type {
	case "agent_start":
		s.mu.Lock()
		s.streaming = true
		s.lastActive = time.Now()
		s.mu.Unlock()
	case "agent_settled":
		s.mu.Lock()
		s.streaming = false
		s.lastActive = time.Now()
		s.mu.Unlock()
		if artifactID := s.ArtifactID(); artifactID != "" {
			go s.persistTranscript(artifactID)
		}
	case "tool_execution_end":
		if !probe.IsError {
			switch probe.Result.Details["exhibit"] {
			case "artifact_saved":
				s.noteArtifactSaved(probe.Result.Details)
			case "state_changed":
				s.noteStateChanged(probe.Result.Details)
			case "widget_saved":
				s.noteWidgetSaved(probe.Result.Details)
			}
		}
	}

	s.broadcast(bytes.Clone(line))
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

// persistTranscript stores the session's full message list with the artifact
// (colophon-style provenance, av-q3wo). Runs after each settled turn so the
// transcript tracks the conversation as it grows.
func (s *Session) persistTranscript(artifactID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := s.roundTrip(ctx, map[string]any{"type": "get_messages"})
	if err != nil {
		slog.Warn("transcript fetch failed", slog.String("session_id", s.ID), slog.String("err", err.Error()))
		return
	}
	var r struct {
		Data struct {
			Messages json.RawMessage `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &r); err != nil || len(r.Data.Messages) == 0 {
		return
	}
	// The session's owner, not the artifact's: a transcript can only attach to
	// an artifact this session's owner actually holds, so an artifact id the
	// model invented (or lifted from another library) fails with ErrNotFound
	// instead of writing across the tenant boundary.
	if err := s.mgr.st.SaveTranscript(ctx, s.OwnerID, artifactID, s.ID, string(r.Data.Messages)); err != nil {
		slog.Warn("transcript save failed", slog.String("session_id", s.ID), slog.String("err", err.Error()))
	}
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
