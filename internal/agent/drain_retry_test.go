package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A real Pi-shaped subprocess, just enough to prepare a private final file and
// answer readiness. Recovery tests must never ask it to run/replay a prompt.
func TestDrainPiHelper(t *testing.T) {
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
	for i, arg := range args {
		if arg == "--session" && i+1 < len(args) {
			_ = os.WriteFile(args[i+1], []byte("{\"private\":true}\n"), 0600)
		}
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var command struct{ ID, Type string }
		if json.Unmarshal(scanner.Bytes(), &command) != nil {
			continue
		}
		if command.Type != "get_state" {
			os.Exit(3)
		}
		fmt.Printf("{\"type\":\"response\",\"id\":%q,\"success\":true,\"data\":{}}\n", command.ID)
	}
	os.Exit(0)
}

type retryTransport func(*http.Request) (*http.Response, error)

func (f retryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func remoteDrainManager(t *testing.T, transport http.RoundTripper) (*Manager, *agentexec.Service) {
	t.Helper()
	m := newLimitedManager(t)
	binary, err := os.Executable()
	require.NoError(t, err)
	require.NotContains(t, binary, "'")
	script := filepath.Join(t.TempDir(), "fake-pi")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexec '"+binary+"' -test.run '^TestDrainPiHelper$' -- \"$@\"\n"), 0700))
	const token = "retry-worker-authentication-0123456789abcdef"
	service, err := agentexec.NewService(agentexec.ServiceConfig{LocalAssignments: true, Token: token, Runner: agentexec.Local{PiBin: script, WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(service)
	t.Cleanup(func() { _ = service.Shutdown(context.Background()); server.Close() })
	m.cfg.Runner = agentexec.Remote{Endpoint: agentexec.Endpoint{URL: server.URL, Token: token, AllowUnassigned: true, Transport: transport}}
	return m, service
}

func waitRetired(t *testing.T, s *Session) {
	t.Helper()
	select {
	case <-s.retired:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not finish its attempt")
	}
}

func TestRetryDrainRecoversTerminalSnapshotAfterCloseNetworkFailure(t *testing.T) {
	var offline atomic.Bool
	var closes, commands, checkpoints, releases atomic.Int32
	transport := retryTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/commands") {
			commands.Add(1)
		}
		if strings.HasSuffix(r.URL.Path, "/close") {
			closes.Add(1)
			if offline.Load() {
				return nil, errors.New("worker unavailable")
			}
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	m, _ := remoteDrainManager(t, transport)
	m.cfg.ReserveExecution = func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(_ context.Context, c ExecutionCheckpoint) error {
			checkpoints.Add(1)
			assert.Equal(t, "{\"private\":true}\n", c.SessionFile)
			return nil
		}, Release: func(context.Context) error { releases.Add(1); return nil }}, nil
	}
	s := createSession(t, m, 7)
	offline.Store(true)
	m.Close(7, s.ID)
	waitRetired(t, s)
	assert.Zero(t, checkpoints.Load())
	assert.Zero(t, releases.Load())
	_, err := m.RetainedSnapshot(context.Background(), 7, s.ID)
	require.Error(t, err, "failed transport cannot claim it has a final snapshot")
	before := closes.Load()
	offline.Store(false)
	require.NoError(t, m.RetryDrain(context.Background(), 7, s.ID))
	assert.Greater(t, closes.Load(), before, "explicit recovery must retry terminal stop/read")
	assert.Equal(t, int32(1), checkpoints.Load())
	assert.Equal(t, int32(1), releases.Load())
	assert.Zero(t, commands.Load(), "terminal recovery never replays Pi input")
	require.NoError(t, m.Shutdown(context.Background()))
}

type cleanupRetryRunner struct {
	local    agentexec.Local
	attempts *atomic.Int32
}
type cleanupRetryProcess struct {
	agentexec.Process
	attempts *atomic.Int32
}

func (r cleanupRetryRunner) Start(ctx context.Context, c agentexec.StartConfig) (agentexec.Process, error) {
	p, err := r.local.Start(ctx, c)
	if err != nil {
		return nil, err
	}
	return &cleanupRetryProcess{Process: p, attempts: r.attempts}, nil
}
func (p *cleanupRetryProcess) Cleanup() error {
	if p.attempts.Add(1) == 1 {
		return errors.New("transient cleanup failure")
	}
	return p.Process.Cleanup()
}

func TestRetryDrainContinuesAfterAcknowledgedAuthorityRelease(t *testing.T) {
	m := newLimitedManager(t)
	var cleanups, checkpoints, releases atomic.Int32
	var closed atomic.Bool
	m.cfg.Runner = cleanupRetryRunner{local: agentexec.Local{PiBin: m.cfg.PiBin, WorkRoot: m.cfg.WorkRoot}, attempts: &cleanups}
	m.cfg.ReserveExecution = func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(context.Context, ExecutionCheckpoint) error {
			checkpoints.Add(1)
			if closed.Load() {
				return errors.New("authority already closed")
			}
			return nil
		}, Release: func(context.Context) error { releases.Add(1); closed.Store(true); return nil }}, nil
	}
	s := createSession(t, m, 7)
	m.Close(7, s.ID)
	waitRetired(t, s)
	require.NoError(t, m.RetryDrain(context.Background(), 7, s.ID))
	assert.Equal(t, int32(1), checkpoints.Load())
	assert.Equal(t, int32(1), releases.Load())
	assert.Equal(t, int32(2), cleanups.Load())
	require.NoError(t, m.Shutdown(context.Background()))
}

func TestRetryDrainReusesCheckpointIdentityAfterLostAcknowledgment(t *testing.T) {
	m := newLimitedManager(t)
	var checkpoints []ExecutionCheckpoint
	m.cfg.ReserveExecution = func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(_ context.Context, c ExecutionCheckpoint) error {
			checkpoints = append(checkpoints, c)
			if len(checkpoints) == 1 {
				return errors.New("lost checkpoint acknowledgment")
			}
			return nil
		}}, nil
	}
	s := createSession(t, m, 7)
	require.NoError(t, os.WriteFile(s.sessionFile, []byte("{\"final\":true}\n"), 0600))
	m.Close(7, s.ID)
	waitRetired(t, s)
	require.NoError(t, m.RetryDrain(context.Background(), 7, s.ID))
	require.Len(t, checkpoints, 2)
	assert.Equal(t, checkpoints[0], checkpoints[1], "same bytes, metadata and sequence on retry")
}

func TestOwnerErasureRetriesMinimalRemoteCleanupWithoutPersistence(t *testing.T) {
	var unavailable atomic.Bool
	transport := retryTransport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/release") && unavailable.Load() {
			return nil, errors.New("reclamation unavailable")
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	m, _ := remoteDrainManager(t, transport)
	var saves, authorityReleases atomic.Int32
	m.cfg.ReserveExecution = func(_ context.Context, id agentexec.Identity, _ *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(context.Context, ExecutionCheckpoint) error {
			if id.OwnerID == 7 {
				saves.Add(1)
				return errors.New("checkpoint unavailable")
			}
			return nil
		}, Release: func(context.Context) error { authorityReleases.Add(1); return nil }}, nil
	}
	retained := createSession(t, m, 7)
	m.Close(7, retained.ID)
	waitRetired(t, retained)
	live := createSession(t, m, 7)
	other := createSession(t, m, 8)
	before := saves.Load()
	unavailable.Store(true)
	require.Error(t, m.EraseOwner(context.Background(), 7))
	assert.Equal(t, before, saves.Load(), "deleted history is never checkpointed")
	assert.Zero(t, authorityReleases.Load(), "erasure uses reclaim, not authority finalization")
	m.mu.Lock()
	assert.Len(t, m.erasureTargets[7], 2, "failed cleanup capabilities remain retryable")
	assert.Empty(t, m.recoveries, "private Session payloads must not remain in recovery")
	assert.NotContains(t, m.sessions, retained.ID)
	assert.NotContains(t, m.sessions, live.ID)
	m.mu.Unlock()
	for _, s := range []*Session{retained, live} {
		assert.Nil(t, m.cfg.Credentials.Resolve(s.grant.Token()))
		assert.Nil(t, s.backlog)
		assert.Nil(t, s.terminalCheckpoint)
	}
	assert.Same(t, other, m.Get(8, other.ID))
	require.Error(t, m.EraseOwner(context.Background(), 7), "persistent failure remains an error on the next sweep")
	unavailable.Store(false)
	require.NoError(t, m.EraseOwner(context.Background(), 7))
	require.NoError(t, m.EraseOwner(context.Background(), 7), "completed erasure is idempotent")
	m.mu.Lock()
	assert.Empty(t, m.erasureTargets[7])
	m.mu.Unlock()
	assert.Equal(t, before, saves.Load())
	assert.Same(t, other, m.Get(8, other.ID))
	require.NoError(t, m.Shutdown(context.Background()))
}
