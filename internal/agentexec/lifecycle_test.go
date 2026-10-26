package agentexec

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHolds struct {
	acquired, renewed, released atomic.Int32
	fail                        atomic.Bool
}

func (h *testHolds) Acquire(context.Context, string, time.Duration) error {
	if h.fail.Load() {
		return errors.New("unavailable")
	}
	h.acquired.Add(1)
	return nil
}
func (h *testHolds) Renew(context.Context, string, time.Duration) error { h.renewed.Add(1); return nil }
func (h *testHolds) Release(context.Context, string) error              { h.released.Add(1); return nil }

func lifecycleService(t *testing.T, holds Holds, expiry time.Duration) (*Service, Process, <-chan json.RawMessage) {
	t.Helper()
	s, err := NewService(ServiceConfig{ExecutionID: "attempt-1", SessionID: "conversation-1", Token: testWorkerToken, Holds: holds, HoldTTL: 3 * time.Second, Runner: Local{PiBin: fakePi(t), WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(s)
	cfg := testConfig()
	cfg.ExpiresAt = time.Now().Add(expiry)
	p, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), cfg)
	require.NoError(t, err)
	lines := collect(t, p)
	t.Cleanup(func() { _ = p.Cleanup(); _ = s.Shutdown(context.Background()); server.Close() })
	return s, p, lines
}

func waitQuiescent(t *testing.T, p Process) {
	t.Helper()
	r := p.(*RemoteProcess)
	require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.quiescent && !r.pumping }, 3*time.Second, time.Millisecond)
}

func TestIdleLeaseExpiresWithoutPollingAndRenewsBeforeExplicitPrompt(t *testing.T) {
	holds := &testHolds{}
	service, p, lines := lifecycleService(t, holds, 1500*time.Millisecond)
	waitQuiescent(t, p)
	assert.Zero(t, holds.acquired.Load(), "preparation has no daemon-lifetime task hold")
	// No request, poll, heartbeat, or provider work across the old deadline.
	time.Sleep(2100 * time.Millisecond)
	select {
	case <-p.(*RemoteProcess).done:
		t.Fatal("idle old lease killed a legitimate prepared worker")
	default:
	}
	require.NoError(t, p.(LeaseProcess).SetLease(context.Background(), time.Now().Add(5*time.Second)))
	service.mu.Lock()
	old := service.expires
	service.mu.Unlock()
	assert.True(t, old.Before(time.Now()), "idle renewal is staged without HTTP")
	sendRPC(t, p, "after-idle", "prompt", "first")
	waitType(t, lines, "agent_settled")
	waitQuiescent(t, p)
	assert.Equal(t, int32(1), holds.acquired.Load())
	assert.Equal(t, int32(1), holds.released.Load())
	before := holds.renewed.Load()
	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, before, holds.renewed.Load(), "idle does not renew task holds")
}

func TestWaitingHumanLeasePauseCanRenewWithoutReplayingTool(t *testing.T) {
	holds := &testHolds{}
	_, p, lines := lifecycleService(t, holds, 1800*time.Millisecond)
	sendRPC(t, p, "question-prompt", "prompt", "ask")
	waitType(t, lines, "extension_ui_request")
	waitQuiescent(t, p)
	time.Sleep(2100 * time.Millisecond)
	require.NoError(t, p.(LeaseProcess).SetLease(context.Background(), time.Now().Add(4*time.Second)))
	sendRPC(t, p, "question-1", "extension_ui_response", "")
	waitType(t, lines, "agent_settled")
	waitQuiescent(t, p)
	file, err := p.Snapshot(context.Background())
	require.NoError(t, err)
	assert.Contains(t, file, "ask")
	assert.Equal(t, int32(2), holds.acquired.Load())
	assert.Equal(t, int32(2), holds.released.Load())
}

func TestActiveLeaseRenewalAndCoordinatorLossBoundHolds(t *testing.T) {
	holds := &testHolds{}
	_, p, lines := lifecycleService(t, holds, 2*time.Second)
	sendRPC(t, p, "active", "prompt", "hold")
	waitType(t, lines, "agent_start")
	require.NoError(t, p.(LeaseProcess).SetLease(context.Background(), time.Now().Add(4*time.Second)))
	time.Sleep(2200 * time.Millisecond)
	select {
	case <-p.(*RemoteProcess).done:
		t.Fatal("healthy active renewal did not extend local watchdog")
	default:
	}
	assert.Positive(t, holds.renewed.Load())
	select {
	case <-p.(*RemoteProcess).done:
	case <-time.After(4 * time.Second):
		t.Fatal("core loss failed to expire active work")
	}
	require.Eventually(t, func() bool { return holds.released.Load() == 1 }, 2*time.Second, time.Millisecond)
}

func TestHoldReleaseWaitsForDurableEventAcknowledgment(t *testing.T) {
	holds := &testHolds{}
	s, err := NewService(ServiceConfig{ExecutionID: "attempt-1", SessionID: "conversation-1", Token: testWorkerToken, Holds: holds, Runner: Local{PiBin: fakePi(t), WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(s)
	defer server.Close()
	defer s.Shutdown(context.Background())
	p, err := (Remote{Endpoint: Endpoint{URL: server.URL, Token: testWorkerToken}}).Start(context.Background(), testConfig())
	require.NoError(t, err)
	defer p.Cleanup()
	handled := p.(EventSynchronizer).SynchronizeEvents()
	settled := make(chan struct{})
	ack := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(p.Stdout())
		for scanner.Scan() {
			var event struct{ Type string }
			_ = json.Unmarshal(scanner.Bytes(), &event)
			if event.Type == "agent_settled" {
				close(settled)
				<-ack
			}
			handled()
		}
	}()
	sendRPC(t, p, "barrier", "prompt", "first")
	select {
	case <-settled:
	case <-time.After(3 * time.Second):
		t.Fatal("no settlement")
	}
	assert.Zero(t, holds.released.Load(), "stdout bytes alone do not acknowledge durability")
	close(ack)
	require.Eventually(t, func() bool { return holds.released.Load() == 1 }, 3*time.Second, time.Millisecond)
}

func TestConcurrentSelectionChangesDoNotTriggerScratchQuota(t *testing.T) {
	root := t.TempDir()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 2000 {
			_ = WriteSelection(root, []string{"selection"})
			_ = WriteSelection(root, nil)
		}
	}()
	for range 1000 {
		require.False(t, scratchTooLarge(root), "normal rename/removal is not a quota breach")
	}
	<-done
	file, err := os.Create(filepath.Join(root, SessionFile))
	require.NoError(t, err)
	require.NoError(t, file.Truncate(MaxSessionBytes+1))
	file.Close()
	assert.True(t, scratchTooLarge(root), "real byte limits remain enforced")
}
