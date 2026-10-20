package agentexec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/momja/Exhibit/internal/agentassets"
)

// Local starts Pi on this host. Remote configurations never construct it.
type Local struct {
	PiBin    string
	WorkRoot string
}

type LocalProcess struct {
	Command        *exec.Cmd
	WorkDir        string
	SessionPath    string
	stdin          io.WriteCloser
	stdout, stderr *os.File
	exited         chan struct{}
	err            error
	cleanupMu      sync.Mutex
	cleaned        bool
	leaseMu        sync.Mutex
	expiresAt      time.Time
	leaseActive    bool
}

func (r Local) Start(ctx context.Context, cfg StartConfig) (Process, error) {
	envKey, ok := ProviderEnvironment[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(cfg.SessionSnapshot) > MaxSessionBytes {
		return nil, errors.New("session snapshot exceeds limit")
	}
	if err := os.MkdirAll(r.WorkRoot, 0700); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(r.WorkRoot)
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp(root, "execution-")
	if err != nil {
		return nil, err
	}
	started := false
	defer func() {
		if !started {
			_ = os.RemoveAll(workDir)
		}
	}()
	if err := agentassets.Materialize(workDir); err != nil {
		return nil, err
	}
	tmpDir := filepath.Join(workDir, "tmp")
	if err := os.Mkdir(tmpDir, 0700); err != nil {
		return nil, err
	}
	sessionPath := filepath.Join(workDir, SessionFile)
	if cfg.SessionSnapshot != "" {
		if err := os.WriteFile(sessionPath, []byte(cfg.SessionSnapshot), 0600); err != nil {
			return nil, err
		}
	}
	args := []string{"--mode", "rpc", "--session", sessionPath,
		"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-themes", "--no-context-files", "--no-builtin-tools",
		"-e", filepath.Join(workDir, "exhibit.ts"), "--system-prompt", cfg.SystemPrompt}
	if cfg.Model != "" {
		args = append(args, "--provider", cfg.Provider, "--model", cfg.Model)
	}
	if cfg.Guardrail != nil {
		args = append(args, "-e", filepath.Join(workDir, "guard.ts"))
	}
	cmd := exec.Command(r.PiBin, args...)
	cmd.Dir = "/" // Recorded cwd must survive moving the conversation to another host.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "LANG=" + os.Getenv("LANG"),
		"HOME=" + workDir, "TMPDIR=" + tmpDir,
		"EXHIBIT_API_URL=" + cfg.APIBaseURL, "EXHIBIT_TOKEN=" + cfg.ToolToken,
		"EXHIBIT_ARTIFACT_ID=" + cfg.TargetArtifactID, "EXHIBIT_SESSION_ID=" + cfg.SessionID,
		"EXHIBIT_SELECTION_FILE=" + filepath.Join(workDir, SelectionFile), envKey + "=" + cfg.APIKey,
	}
	if cfg.MockLLMURL != "" {
		cmd.Env = append(cmd.Env, "EXHIBIT_MOCK_LLM_URL="+cfg.MockLLMURL)
	}
	if cfg.WidgetOnly {
		cmd.Env = append(cmd.Env, "EXHIBIT_WIDGET_ONLY=1")
	}
	if cfg.MaxRequestBodyBytes > 0 {
		cmd.Env = append(cmd.Env, "EXHIBIT_MAX_BODY_BYTES="+strconv.FormatInt(cfg.MaxRequestBodyBytes, 10))
	}
	if g := cfg.Guardrail; g != nil {
		cmd.Env = append(cmd.Env, "EXHIBIT_GUARD_PROVIDER="+g.Provider, "EXHIBIT_GUARD_MODEL="+g.Model, "EXHIBIT_GUARD_API_KEY="+g.APIKey)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	defer func() {
		if !started {
			_ = stdin.Close()
		}
	}()
	// Own these pipes so Wait cannot close them before final output is drained.
	stdout, outWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer outWriter.Close()
	defer func() {
		if !started {
			_ = stdout.Close()
		}
	}()
	stderr, errWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer errWriter.Close()
	defer func() {
		if !started {
			_ = stderr.Close()
		}
	}()
	cmd.Stdout, cmd.Stderr = outWriter, errWriter
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pi: %w", err)
	}
	p := &LocalProcess{Command: cmd, WorkDir: workDir, SessionPath: sessionPath, stdin: stdin, stdout: stdout, stderr: stderr, exited: make(chan struct{}), expiresAt: cfg.ExpiresAt}
	started = true
	go func() { p.err = cmd.Wait(); close(p.exited) }()
	go p.enforceBounds()
	return p, nil
}

func (p *LocalProcess) Write(b []byte) (int, error) {
	if len(b) > MaxCommandBytes {
		return 0, errors.New("Pi command exceeds limit")
	}
	if pipe, ok := p.stdin.(*os.File); ok {
		_ = pipe.SetWriteDeadline(time.Now().Add(30 * time.Second))
	}
	n, err := p.stdin.Write(b)
	if err != nil {
		_ = p.Close()
	}
	return n, err
}
func (p *LocalProcess) Close() error      { _ = p.stdin.Close(); return p.Kill() }
func (p *LocalProcess) Stdout() io.Reader { return p.stdout }
func (p *LocalProcess) Stderr() io.Reader { return p.stderr }
func (p *LocalProcess) Wait() error       { <-p.exited; return p.err }
func (p *LocalProcess) Kill() error {
	select {
	case <-p.exited:
		return nil
	default:
	}
	err := syscall.Kill(-p.Command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func (p *LocalProcess) Snapshot(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return ReadSessionFile(p.SessionPath)
}
func (p *LocalProcess) WriteSelection(ctx context.Context, selection []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return WriteSelection(p.WorkDir, selection)
}
func (p *LocalProcess) Cleanup() error {
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()
	if p.cleaned {
		return nil
	}
	if err := p.Close(); err != nil {
		return err
	}
	_ = p.Wait()
	_ = p.stdout.Close()
	_ = p.stderr.Close()
	if err := os.RemoveAll(p.WorkDir); err != nil {
		return err
	}
	p.cleaned = true
	return nil
}

func (p *LocalProcess) ErasureCleanup() Reclaimer {
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()
	// The caller has already stopped/reaped the local process. Keep only its
	// scratch path if filesystem reclamation needs a later retry.
	p.Command.Env = nil
	_ = p.stdout.Close()
	_ = p.stderr.Close()
	return scratchReclaimer(p.WorkDir)
}

type scratchReclaimer string

func (r scratchReclaimer) Cleanup() error { return os.RemoveAll(string(r)) }

// Scratch is transient and bounded even when Pi or an extension misbehaves.
// The worker package never removes active scratch to enforce a quota: it stops
// the process first so core can retrieve the last complete snapshot.
func (p *LocalProcess) SetLease(ctx context.Context, deadline time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !deadline.After(time.Now()) || deadline.After(time.Now().Add(24*time.Hour)) {
		return errors.New("worker lease must expire within 24 hours")
	}
	p.leaseMu.Lock()
	p.expiresAt = deadline
	p.leaseMu.Unlock()
	return nil
}

func (p *LocalProcess) SetActive(active bool) {
	p.leaseMu.Lock()
	p.leaseActive = active
	p.leaseMu.Unlock()
}

func (p *LocalProcess) enforceBounds() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.exited:
			return
		case now := <-ticker.C:
			p.leaseMu.Lock()
			expiry, active := p.expiresAt, p.leaseActive
			p.leaseMu.Unlock()
			if (active && !expiry.IsZero() && !now.Before(expiry)) || scratchTooLarge(p.WorkDir) {
				_ = p.Close()
				return
			}
		}
	}
}

func scratchTooLarge(root string) bool {
	var size int64
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		count++
		if count > 1024 {
			return errors.New("scratch file limit")
		}
		info, err := entry.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		size += info.Size()
		if size > 96<<20 || (entry.Name() == SessionFile && info.Size() > MaxSessionBytes) {
			return errors.New("scratch byte limit")
		}
		return nil
	})
	return err != nil
}

func WriteSelection(root string, selection []string) error {
	path := filepath.Join(root, SelectionFile)
	if len(selection) == 0 {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.Marshal(selection)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func ReadSessionFile(path string) (string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	return ReadSessionContents(file, MaxSessionBytes)
}

// Read an overflow byte before trimming partial lines: even an unfinished
// oversized entry must be refused without allocating its entire contents.
func ReadSessionContents(r io.Reader, limit int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > limit {
		return "", fmt.Errorf("session file exceeds the %d bytes kept", limit)
	}
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return "", nil
	}
	return string(data[:end+1]), nil
}

// LockScratchRoot takes exclusive ownership before removing abandoned execution
// directories from a prior daemon crash. Never point this at core's data dir.
func LockScratchRoot(root string) (io.Closer, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(root, ".worker.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("worker scratch root is already in use")
	}
	entries, err := os.ReadDir(root)
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "execution-") {
				err = os.RemoveAll(filepath.Join(root, entry.Name()))
				if err != nil {
					break
				}
			}
		}
	}
	if err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}
