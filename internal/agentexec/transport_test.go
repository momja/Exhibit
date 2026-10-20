package agentexec

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkerToken = "worker-secret-only-0123456789abcdef"

// A real subprocess with deterministic Pi-shaped RPC, intentionally answering
// prompt BEFORE agent_start to exercise the HTTP idle/command race.
func TestPiProcessHelper(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == "--" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	args := os.Args[marker+1:]
	for _, arg := range args {
		if arg == "--version" {
			fmt.Println(PiVersion)
			os.Exit(0)
		}
	}
	session := ""
	for i, arg := range args {
		if arg == "--session" && i+1 < len(args) {
			session = args[i+1]
		}
	}
	if session == "" {
		os.Exit(2)
	}
	_ = os.WriteFile(filepath.Join(os.Getenv("HOME"), "env.txt"), []byte(strings.Join(os.Environ(), "\n")), 0600)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64<<10), MaxCommandBytes)
	for scanner.Scan() {
		var cmd struct{ ID, Type, Message string }
		if json.Unmarshal(scanner.Bytes(), &cmd) != nil {
			continue
		}
		response := func() {
			data, _ := json.Marshal(map[string]any{"type": "response", "id": cmd.ID, "success": true, "data": map[string]any{"tokens": map[string]int{"input": 1, "output": 1}, "cost": 0}})
			fmt.Println(string(data))
		}
		switch cmd.Type {
		case "get_state", "get_session_stats", "compact":
			response()
		case "prompt", "steer":
			if cmd.Message == "blocked" {
				fmt.Printf("{\"type\":\"response\",\"id\":%q,\"success\":true,\"data\":{\"disposition\":\"handled\"}}\n", cmd.ID)
				continue
			}
			response()
			time.Sleep(20 * time.Millisecond)
			fmt.Println(`{"type":"agent_start"}`)
			file, _ := os.OpenFile(session, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			entry, _ := json.Marshal(map[string]any{"type": "message", "message": map[string]any{"role": "user", "content": []map[string]string{{"type": "text", "text": cmd.Message}}}})
			_, _ = file.Write(append(entry, '\n'))
			_ = file.Close()
			if cmd.Message == "hold" {
				continue
			}
			if cmd.Message == "ask" {
				fmt.Println(`{"type":"extension_ui_request","id":"question-1","method":"input","title":"exhibit:questions:v1","placeholder":"{}"}`)
				continue
			}
			fmt.Println(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input":1,"output":1,"cost":{"total":0}}}}`)
			fmt.Println(`{"type":"agent_settled"}`)
		case "extension_ui_response":
			fmt.Println(`{"type":"agent_settled"}`)
		case "abort":
			response()
			fmt.Println(`{"type":"agent_settled"}`)
		}
	}
	os.Exit(0)
}

func fakePi(t *testing.T) string {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	script := filepath.Join(t.TempDir(), "pi")
	// Go test paths contain no shell quotes on supported CI hosts.
	require.NotContains(t, binary, "'")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexec '"+binary+"' -test.run '^TestPiProcessHelper$' -- \"$@\"\n"), 0700))
	return script
}

func testConfig() StartConfig {
	return StartConfig{Identity: Identity{ExecutionID: "attempt-1", SessionID: "conversation-1"}, Provider: "anthropic", APIKey: "assigned-provider-key", ToolToken: "artifact-scoped-grant", APIBaseURL: "http://core.test", SystemPrompt: "current prompt", ExpiresAt: time.Now().Add(time.Hour)}
}

func testService(t *testing.T) (*Service, *httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	s, err := NewService(ServiceConfig{ExecutionID: "attempt-1", SessionID: "conversation-1", Token: testWorkerToken, Runner: Local{PiBin: fakePi(t), WorkRoot: root}, ReadyTimeout: 2 * time.Second})
	require.NoError(t, err)
	server := httptest.NewServer(s)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
		server.Close()
	})
	return s, server, root
}

func collect(t *testing.T, p Process) <-chan json.RawMessage {
	t.Helper()
	lines := make(chan json.RawMessage, 1000)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(p.Stdout())
		scanner.Buffer(make([]byte, 64<<10), MaxEventBytes)
		for scanner.Scan() {
			lines <- bytes.Clone(scanner.Bytes())
		}
	}()
	return lines
}

func waitType(t *testing.T, lines <-chan json.RawMessage, kind string) json.RawMessage {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-lines:
			require.True(t, ok, "stream closed before %s", kind)
			var event struct{ Type string }
			require.NoError(t, json.Unmarshal(line, &event))
			if event.Type == kind {
				return line
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", kind)
			return nil
		}
	}
}

func sendRPC(t *testing.T, p Process, id, kind, message string) {
	t.Helper()
	line, err := json.Marshal(map[string]string{"id": id, "type": kind, "message": message})
	require.NoError(t, err)
	_, err = p.Write(append(line, '\n'))
	require.NoError(t, err)
}

func TestRemoteProcessRoundTripIdleAndRestore(t *testing.T) {
	service, server, root := testService(t)
	var idle atomic.Int32
	var active atomic.Int32
	runner := Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken, Active: func(context.Context) error { active.Add(1); return nil }, Idle: func(_ context.Context, s Snapshot) error { idle.Add(1); return nil }}}
	p, err := runner.Start(context.Background(), testConfig())
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Cleanup() })
	lines := collect(t, p)
	require.Eventually(t, func() bool { return idle.Load() >= 1 }, 3*time.Second, time.Millisecond)
	require.NoError(t, p.WriteSelection(context.Background(), []string{"untrusted element"}))
	sendRPC(t, p, "prompt-1", "prompt", "first")
	waitType(t, lines, "agent_settled")
	require.Eventually(t, func() bool { return idle.Load() >= 2 }, 3*time.Second, time.Millisecond)
	remote := p.(*RemoteProcess)
	remote.mu.Lock()
	assert.False(t, remote.pumping, "no live HTTP event stream after settle")
	remote.mu.Unlock()
	firstSnapshot, err := p.Snapshot(context.Background())
	require.NoError(t, err)
	require.Contains(t, firstSnapshot, "first")
	service.mu.Lock()
	local := service.process.(*LocalProcess)
	service.mu.Unlock()
	selection, err := os.ReadFile(filepath.Join(local.WorkDir, SelectionFile))
	require.NoError(t, err)
	assert.Contains(t, string(selection), "untrusted element")
	sendRPC(t, p, "prompt-2", "prompt", "second")
	waitType(t, lines, "agent_settled")
	_, err = p.Snapshot(context.Background())
	require.NoError(t, err)
	require.NoError(t, p.Close())
	require.NoError(t, p.Wait())
	final, err := p.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Contains(t, final, "first")
	assert.Contains(t, final, "second")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "close removes all scratch")
	assert.GreaterOrEqual(t, active.Load(), int32(3))
	// A different daemon gets only the central checkpoint, with fresh identity.
	_, replacement, _ := testService(t)
	restored := testConfig()
	restored.SessionSnapshot = final
	p2, err := (Remote{Endpoint: Endpoint{URL: replacement.URL, Token: testWorkerToken}}).Start(context.Background(), restored)
	require.NoError(t, err)
	collect(t, p2)
	t.Cleanup(func() { _ = p2.Cleanup() })
	file, err := p2.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, final, file)
}

func TestWorkerCommandDeduplicationAndAbort(t *testing.T) {
	_, server, _ := testService(t)
	p, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	lines := collect(t, p)
	t.Cleanup(func() { _ = p.Cleanup() })
	sendRPC(t, p, "stable-1", "prompt", "hold")
	waitType(t, lines, "agent_start")
	sendRPC(t, p, "stable-1", "prompt", "hold")
	snapshot, err := p.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(snapshot, "hold"), "retrying a stable identity never starts a second prompt")
	sendRPC(t, p, "abort-1", "abort", "")
	waitType(t, lines, "agent_settled")
	command := Command{ID: "stable-1", Deadline: time.Now().Add(time.Minute), RPC: json.RawMessage(`{"id":"stable-1","type":"prompt","message":"different"}`)}
	err = p.(*RemoteProcess).request(context.Background(), "POST", "/commands", command, new(any), 1<<20)
	var failure *HTTPError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "command_conflict", failure.Code)
}

func TestWorkerAuthenticatesVersionsAssignmentAndLimits(t *testing.T) {
	_, server, _ := testService(t)
	for _, test := range []struct {
		token, version, path string
		status               int
	}{
		{"", ProtocolVersion, "/v1/ready", 401},
		{testWorkerToken, "2", "/v1/ready", 426},
		{testWorkerToken, ProtocolVersion, "/v1/executions/another/events?after=0", 404},
	} {
		req, err := http.NewRequest("GET", server.URL+test.path, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+test.token)
		req.Header.Set(VersionHeader, test.version)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, test.status, resp.StatusCode)
	}
	wrong := testConfig()
	wrong.SessionID = "somebody-else"
	_, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), wrong)
	require.ErrorContains(t, err, "assignment mismatch")
}

func TestUncertainCommandIsNotReplayed(t *testing.T) {
	service, _, _ := testService(t)
	var commands atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/commands") {
			commands.Add(1)
			recorder := httptest.NewRecorder()
			service.ServeHTTP(recorder, r)
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				connection.Close()
			}
			return
		}
		service.ServeHTTP(w, r)
	}))
	defer server.Close()
	p, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	collect(t, p)
	t.Cleanup(func() { _ = p.Cleanup() })
	_, err = p.Write([]byte(`{"id":"uncertain-1","type":"prompt","message":"first"}` + "\n"))
	var uncertain *UncertainError
	require.ErrorAs(t, err, &uncertain)
	assert.Equal(t, int32(1), commands.Load())
	require.Error(t, p.Wait())
}

func TestLocalEnvironmentBoundsAndCrashCleanup(t *testing.T) {
	t.Setenv("EXHIBIT_OPERATOR_TOKEN", "must-not-reach-pi")
	t.Setenv("OPENAI_API_KEY", "another-users-key")
	root := t.TempDir()
	p, err := (Local{PiBin: fakePi(t), WorkRoot: root}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	collect(t, p)
	t.Cleanup(func() { _ = p.Cleanup() })
	local := p.(*LocalProcess)
	var env []byte
	require.Eventually(t, func() bool {
		env, err = os.ReadFile(filepath.Join(local.WorkDir, "env.txt"))
		return err == nil && len(env) > 0
	}, 3*time.Second, time.Millisecond)
	assert.Contains(t, string(env), "ANTHROPIC_API_KEY=assigned-provider-key")
	assert.Contains(t, string(env), "EXHIBIT_TOKEN=artifact-scoped-grant")
	assert.Contains(t, string(env), "TMPDIR="+filepath.Join(local.WorkDir, "tmp"))
	assert.NotContains(t, string(env), "must-not-reach-pi")
	assert.NotContains(t, string(env), "another-users-key")
	assert.NotContains(t, string(env), testWorkerToken)
	assert.Contains(t, local.Command.Args, "--no-context-files")
	assert.Contains(t, local.Command.Args, "--no-builtin-tools")
	file, err := os.Create(local.SessionPath)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(MaxSessionBytes+1))
	file.Close()
	done := make(chan struct{})
	go func() { _ = p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("oversized scratch did not stop Pi")
	}
	require.NoError(t, p.Cleanup())
	stale := filepath.Join(root, "execution-abandoned")
	require.NoError(t, os.Mkdir(stale, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(stale, "credentials"), []byte("secret"), 0600))
	lock, err := LockScratchRoot(root)
	require.NoError(t, err)
	defer lock.Close()
	_, err = os.Stat(stale)
	assert.True(t, os.IsNotExist(err))
	_, err = LockScratchRoot(root)
	assert.Error(t, err, "another daemon must not delete live scratch")
}

func TestOversizedEventsAndPartialSnapshotsStayBounded(t *testing.T) {
	r := bufio.NewReaderSize(io.MultiReader(io.LimitReader(zeroes{}, MaxEventBytes+1), strings.NewReader("\n{\"type\":\"next\"}\n")), 64<<10)
	_, large, err := ReadEventLine(r)
	require.NoError(t, err)
	assert.True(t, large)
	line, large, err := ReadEventLine(r)
	require.NoError(t, err)
	assert.False(t, large)
	assert.Contains(t, string(line), "next")
	file, err := ReadSessionContents(strings.NewReader("{}\npartial"), 100)
	require.NoError(t, err)
	assert.Equal(t, "{}\n", file)
	_, err = ReadSessionContents(strings.NewReader(strings.Repeat("x", 101)), 100)
	assert.Error(t, err)
}

type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// Instrument actual TCP lifecycle, rather than merely checking a pumping flag.
func TestIdleWorkerHasNoOpenHTTPConnectionsOrPolling(t *testing.T) {
	service, _, _ := testService(t)
	var open atomic.Int32
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); service.ServeHTTP(w, r) }))
	var mu sync.Mutex
	connections := map[net.Conn]bool{}
	server.Config.ConnState = func(c net.Conn, state http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		if state == http.StateNew {
			connections[c] = true
			open.Add(1)
		}
		if state == http.StateClosed && connections[c] {
			delete(connections, c)
			open.Add(-1)
		}
	}
	server.Start()
	defer server.Close()
	p, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	lines := collect(t, p)
	t.Cleanup(func() { _ = p.Cleanup() })
	sendRPC(t, p, "prompt-idle", "prompt", "first")
	waitType(t, lines, "agent_settled")
	require.Eventually(t, func() bool {
		remote := p.(*RemoteProcess)
		remote.mu.Lock()
		defer remote.mu.Unlock()
		return !remote.pumping && open.Load() == 0
	}, 3*time.Second, time.Millisecond)
	count := requests.Load()
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, count, requests.Load(), "idle must not trigger health checks or event reconnect")
}

func TestStaticLocalServiceAcceptsGeneratedIdentitiesAndReleasesCapacity(t *testing.T) {
	service, err := NewService(ServiceConfig{LocalAssignments: true, MaxAssignments: 2, Token: testWorkerToken, Runner: Local{PiBin: fakePi(t), WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(service)
	defer server.Close()
	defer func() { _ = service.Shutdown(context.Background()) }()
	runner := Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken, AllowUnassigned: true}}
	create := func(id string) (Process, error) {
		cfg := testConfig()
		cfg.ExecutionID = id
		cfg.SessionID = "session-" + id
		return runner.Start(context.Background(), cfg)
	}
	first, err := create("generated-first")
	require.NoError(t, err)
	collect(t, first)
	second, err := create("generated-second")
	require.NoError(t, err)
	collect(t, second)
	_, err = create("over-capacity")
	require.ErrorContains(t, err, "assignment_limit")
	require.NoError(t, first.Cleanup())
	third, err := create("generated-third")
	require.NoError(t, err)
	collect(t, third)
	require.NoError(t, second.Cleanup())
	require.NoError(t, third.Cleanup())
	_, err = create("generated-first")
	require.ErrorContains(t, err, "execution_retired")
	_, err = (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.ErrorContains(t, err, "assignment mismatch", "unassigned acceptance must be explicitly configured in core too")
}

func TestColdRestartRestoresOnlyForNewExplicitPrompt(t *testing.T) {
	service, firstServer, _ := testService(t)
	var restoreCalls atomic.Int32
	var checkpoint string
	var checkpointMu sync.Mutex
	currentURL := firstServer.URL
	var endpointMu sync.Mutex
	provisioner := ProvisionFunc(func(context.Context, Identity) (Endpoint, error) {
		endpointMu.Lock()
		defer endpointMu.Unlock()
		return Endpoint{URL: currentURL, Token: testWorkerToken,
			Idle: func(_ context.Context, s Snapshot) error {
				checkpointMu.Lock()
				checkpoint = s.SessionFile
				checkpointMu.Unlock()
				return nil
			},
			LoadSnapshot: func(context.Context, Identity) (RestoreState, error) {
				restoreCalls.Add(1)
				checkpointMu.Lock()
				defer checkpointMu.Unlock()
				return RestoreState{SessionFile: checkpoint, ArtifactID: "artifact"}, nil
			}}, nil
	})
	p, err := (Remote{Provisioner: provisioner}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	lines := collect(t, p)
	t.Cleanup(func() { _ = p.Cleanup() })
	sendRPC(t, p, "first-before-restart", "prompt", "first")
	waitType(t, lines, "agent_settled")
	require.Eventually(t, func() bool {
		r := p.(*RemoteProcess)
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.quiescent && !r.pumping
	}, 3*time.Second, time.Millisecond)
	// Replace the daemon without sending any event or command to the idle core.
	require.NoError(t, service.Shutdown(context.Background()))
	_, replacement, _ := testService(t)
	endpointMu.Lock()
	currentURL = replacement.URL
	endpointMu.Unlock()
	assert.Zero(t, restoreCalls.Load(), "replacement must not trigger background replay")
	sendRPC(t, p, "new-explicit-prompt", "prompt", "second")
	waitType(t, lines, "agent_settled")
	file, err := p.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(file, "first"))
	assert.Equal(t, 1, strings.Count(file, "second"))
	assert.Equal(t, int32(1), restoreCalls.Load())
}

func TestWorkerReadinessAndOutstandingCommandDeadlines(t *testing.T) {
	silent := filepath.Join(t.TempDir(), "silent-pi")
	require.NoError(t, os.WriteFile(silent, []byte("#!/bin/sh\nexec sleep 60\n"), 0700))
	service, err := NewService(ServiceConfig{ExecutionID: "attempt-1", SessionID: "conversation-1", Token: testWorkerToken, ReadyTimeout: 30 * time.Millisecond, Runner: Local{PiBin: silent, WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(service)
	defer server.Close()
	defer func() { _ = service.Shutdown(context.Background()) }()
	_, err = (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.ErrorContains(t, err, "readiness_timeout")
	working, workingServer, _ := testService(t)
	p, err := (Remote{Endpoint: Endpoint{URL: workingServer.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	collect(t, p)
	defer p.Cleanup()
	command := Command{ID: "expired", Deadline: time.Now().Add(-time.Second), RPC: json.RawMessage(`{"id":"expired","type":"prompt","message":"must not run"}`)}
	err = p.(*RemoteProcess).request(context.Background(), "POST", "/commands", command, new(any), 1<<20)
	require.ErrorContains(t, err, "invalid_deadline")
	working.mu.Lock()
	assert.NotContains(t, working.commands, "expired")
	working.mu.Unlock()
}
