// Research-only Go supervisor for the Pi 1.4.0 extension. No production packages depend on it.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type request struct {
	ID      string         `json:"id"`
	Op      string         `json:"op"`
	Model   string         `json:"model,omitempty"`
	APIKey  string         `json:"apiKey,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

type reply struct {
	ID     string         `json:"id"`
	Result map[string]any `json:"result"`
	Error  string         `json:"error,omitempty"`
}

type worker struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	queue   chan []byte
	done    chan struct{}
	mu      sync.Mutex
	pending map[string]chan reply
	once    sync.Once
	seq     atomic.Uint64
	dir     string
}

// Restart is explicit: failed/in-flight calls are never replayed (they may already be billed).
// The server would keep one independent supervisor, not use agent.Manager/reserveProcess.
func startWorker(ctx context.Context, binary, extension, testURL string) (*worker, error) {
	dir, err := os.MkdirTemp("", "exhibit-inference-")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "--mode", "rpc", "--offline", "--no-session", "--no-tools",
		"--no-extensions", "--no-mcp", "--no-skills", "--no-prompt-templates", "--no-themes",
		"--no-context-files", "--no-approve", "-e", extension)
	cmd.Dir = dir
	// No inherited provider keys, auth store, user extensions, or caller secrets in argv/env.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir,
		"PI_CODING_AGENT_DIR=" + filepath.Join(dir, "agent"), "PI_OFFLINE=1"}
	if testURL != "" {
		cmd.Env = append(cmd.Env, "EXHIBIT_INFERENCE_TEST_URL="+testURL)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	// Deliberately don't copy child stderr or unrecognized stdout to logs.
	cmd.Stderr = io.Discard
	w := &worker{cmd: cmd, stdin: stdin, queue: make(chan []byte, 32), done: make(chan struct{}), pending: make(map[string]chan reply), dir: dir}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	go func() {
		for {
			select {
			case b := <-w.queue:
				if _, err := stdin.Write(b); err != nil {
					w.fail()
					return
				}
			case <-w.done:
				return
			}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var event struct {
				Type, Method, Message, Command string
				Success                        bool
				Data                           struct{ Disposition string }
			}
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				w.fail()
				break
			}
			// A missing/broken extension must never silently become an ordinary agent session.
			if event.Type == "response" && event.Command == "prompt" && (!event.Success || event.Data.Disposition != "handled") {
				w.fail()
				break
			}
			if event.Type != "extension_ui_request" || event.Method != "notify" || !strings.HasPrefix(event.Message, "exhibit_inference:") {
				continue
			}
			var r reply
			if json.Unmarshal([]byte(strings.TrimPrefix(event.Message, "exhibit_inference:")), &r) != nil {
				w.fail()
				break
			}
			w.mu.Lock()
			ch := w.pending[r.ID]
			delete(w.pending, r.ID)
			w.mu.Unlock()
			if ch != nil {
				ch <- r
			}
		}
		w.fail()
		_ = cmd.Wait() // reap exactly once, after draining stdout
		_ = os.RemoveAll(dir)
	}()
	if _, err := w.call(ctx, request{Op: "ping"}); err != nil {
		w.close()
		return nil, fmt.Errorf("worker readiness: %w", err)
	}
	return w, nil
}

func (w *worker) fail() {
	w.once.Do(func() {
		close(w.done)
		_ = w.cmd.Process.Kill()
		_ = w.stdin.Close()
	})
}

func (w *worker) close() { w.fail() }

func (w *worker) send(ctx context.Context, req request) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if len(body) > 65_536 {
		return errors.New("request too large")
	}
	line, _ := json.Marshal(map[string]any{"type": "prompt", "id": req.ID, "message": string(body)})
	line = append(line, '\n')
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.done:
		return errors.New("worker exited")
	case w.queue <- line:
		return nil
	}
}

func (w *worker) call(ctx context.Context, req request) (map[string]any, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("a wall-clock deadline is required")
	}
	req.ID = fmt.Sprint(w.seq.Add(1))
	ch := make(chan reply, 1)
	w.mu.Lock()
	w.pending[req.ID] = ch
	w.mu.Unlock()
	defer func() { w.mu.Lock(); delete(w.pending, req.ID); w.mu.Unlock() }()
	if err := w.send(ctx, req); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		if r.Error != "" {
			return r.Result, errors.New(r.Error)
		}
		return r.Result, nil // includes stopReason and any usage even on provider error
	case <-w.done:
		return nil, errors.New("worker exited; outcome unknown")
	case <-ctx.Done():
		// Per-request cancellation, never Pi's session-wide `abort` command.
		grace, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		_ = w.send(grace, request{ID: req.ID, Op: "cancel"})
		select {
		case r := <-ch:
			return r.Result, ctx.Err() // retain any reported usage
		case <-w.done:
			return nil, ctx.Err()
		case <-grace.Done():
			w.fail()
			return nil, ctx.Err() // wedged JS/ignored abort: kill generation
		}
	}
}

func rssKiB(pid int) int64 {
	b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			var value int64
			_, _ = fmt.Sscanf(line, "VmRSS: %d kB", &value)
			return value
		}
	}
	return 0
}
