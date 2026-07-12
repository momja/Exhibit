// Package agent manages Pi sidecar processes (Exh-m4ym, av-q3wo). Each chat
// session spawns one `pi --mode rpc` subprocess — Mario Zechner's agent
// harness speaking strict JSONL over stdin/stdout — loaded with only the
// exhibit tools extension, so everything the model saves flows through the
// exhibit HTTP API (the single write path). The user's decrypted provider key
// is handed to the subprocess through its environment and never appears in
// argv, page JS, or the datastore.
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

	"github.com/artifact-viewer/artifact-viewer/internal/store"
	"github.com/google/uuid"
)

//go:embed ext/exhibit.ts
var extFS embed.FS

// Config for the Manager.
type Config struct {
	PiBin        string // pi executable, e.g. "pi"
	WorkRoot     string // scratch root; per-session cwd + the materialized extension
	APIBaseURL   string // exhibit app origin the extension calls back into
	AuthToken    string // exhibit API token for the extension
	MockLLMURL   string // when set, sessions may use the "exhibit-mock" provider
	IdleTimeout  time.Duration
	SystemPrompt string // optional override; empty uses the default
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
	OwnerID       int64
	Provider      string
	Model         string
	APIKey        string // decrypted, handed to the subprocess env only
	ArtifactID    string // non-empty: session edits an existing artifact
	ArtifactTitle string
}

const defaultSystemPrompt = `You are the artifact builder inside Exhibit, a personal library of small self-contained web tools.

An artifact is a SINGLE-FILE, self-contained HTML document: all CSS and JavaScript inline in the one file, no external network dependencies (a per-artifact allowlist blocks unapproved origins at render time, so prefer zero external references). localStorage and sessionStorage work and persist across the user's devices.

Your tools:
- create_artifact(title, body): save a brand-new artifact. Returns its id and render URL.
- update_artifact(id, body[, title]): overwrite an existing artifact's source.
- get_artifact(id): read an artifact's current source and metadata.

Workflow: compose the complete HTML document, then save it with create_artifact (new) or update_artifact (existing). Always save the FULL document — never a fragment or a diff. After saving, tell the user in one or two sentences what you built or changed; do not repeat the source code in chat.

If the user message includes a snippet (an attached screenshot plus an element descriptor with selector/outerHTML), that is the exact element the user means — locate it in the source by the descriptor and apply the change there.`

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

	sysPrompt := m.cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = defaultSystemPrompt
	}
	if opts.ArtifactID != "" {
		sysPrompt += fmt.Sprintf("\n\nThis session is editing the existing artifact id %q titled %q. Read it with get_artifact before changing it, and save with update_artifact (never create_artifact).", opts.ArtifactID, opts.ArtifactTitle)
	}

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
		"EXHIBIT_TOKEN=" + m.cfg.AuthToken,
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

	s := &Session{
		ID:         id,
		OwnerID:    opts.OwnerID,
		ArtifactID: opts.ArtifactID,
		mgr:        m,
		cmd:        cmd,
		stdin:      stdin,
		subs:       map[chan []byte]struct{}{},
		pending:    map[string]chan json.RawMessage{},
		done:       make(chan struct{}),
		lastActive: time.Now(),
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

	mgr   *Manager
	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex // serializes stdin writes

	mu         sync.Mutex // guards everything below
	ArtifactID string     // artifact bound to this session (set on first save)
	subs       map[chan []byte]struct{}
	backlog    [][]byte
	pending    map[string]chan json.RawMessage
	streaming  bool
	closed     bool
	lastActive time.Time

	done chan struct{}
}

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

// Prompt sends a user prompt (optionally with images). If the agent is
// mid-stream the message is queued as a steering message.
func (s *Session) Prompt(ctx context.Context, message string, images []ImageContent) error {
	cmd := map[string]any{"type": "prompt", "message": message}
	if len(images) > 0 {
		cmd["images"] = images
	}
	s.mu.Lock()
	if s.streaming {
		cmd["streamingBehavior"] = "steer"
	}
	s.lastActive = time.Now()
	s.mu.Unlock()

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
		artifactID := s.ArtifactID
		s.mu.Unlock()
		if artifactID != "" {
			go s.persistTranscript(artifactID)
		}
	case "tool_execution_end":
		if !probe.IsError && probe.Result.Details["exhibit"] == "artifact_saved" {
			s.noteArtifactSaved(probe.Result.Details)
		}
	}

	s.broadcast(bytes.Clone(line))
}

// noteArtifactSaved binds the session to the saved artifact and emits a
// synthetic event the chat UI uses to show the live preview.
func (s *Session) noteArtifactSaved(details map[string]any) {
	artifactID, _ := details["artifactId"].(string)
	if artifactID == "" {
		return
	}
	s.mu.Lock()
	s.ArtifactID = artifactID
	s.mu.Unlock()
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
	if err := s.mgr.st.SaveTranscript(ctx, artifactID, s.ID, string(r.Data.Messages)); err != nil {
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
	close(s.done)
	ev, _ := json.Marshal(map[string]string{"type": "exhibit_session_closed"})
	s.broadcast(ev)
}

func (s *Session) kill() {
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
