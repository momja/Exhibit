package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFailedFinalCheckpointOrReleaseRetainsSnapshotUntilExplicitRetry(t *testing.T) {
	for _, failure := range []string{"checkpoint", "release"} {
		t.Run(failure, func(t *testing.T) {
			m := newLimitedManager(t)
			var fail atomic.Bool
			fail.Store(true)
			var releaseCalls atomic.Int32
			m.cfg.ReserveExecution = func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error) {
				return &ExecutionReservation{Checkpoint: func(context.Context, ExecutionCheckpoint) error {
					if failure == "checkpoint" && fail.Load() {
						return errors.New("checkpoint unavailable")
					}
					return nil
				}, Release: func(context.Context) error {
					releaseCalls.Add(1)
					if failure == "release" && fail.Load() {
						return errors.New("release unavailable")
					}
					return nil
				}}, nil
			}
			s := createSession(t, m, 7)
			const file = "{\"final\":true}\n"
			require.NoError(t, os.WriteFile(s.sessionFile, []byte(file), 0600))
			m.Close(7, s.ID)
			<-s.retired
			retained, err := m.RetainedSnapshot(context.Background(), 7, s.ID)
			require.NoError(t, err)
			assert.Equal(t, file, retained)
			_, err = os.Stat(s.workDir)
			require.NoError(t, err, "unacknowledged bytes must survive failure")
			if failure == "checkpoint" {
				assert.Zero(t, releaseCalls.Load())
			}
			_, err = m.RetainedSnapshot(context.Background(), 8, s.ID)
			require.Error(t, err)
			fail.Store(false)
			require.NoError(t, m.RetryDrain(context.Background(), 7, s.ID))
			_, err = os.Stat(s.workDir)
			assert.True(t, os.IsNotExist(err), "explicit successful retry permits reclamation")
		})
	}
}

func TestParentResumeSnapshotAndCloseOrdering(t *testing.T) {
	m := newLimitedManager(t)
	var reserved atomic.Bool
	m.st = &executionStore{Store: m.st, reserved: &reserved} // Any direct resume read fails.
	var mu sync.Mutex
	var order []string
	m.cfg.ReserveExecution = func(_ context.Context, id agentexec.Identity, _ *agentscope.Grant) (*ExecutionReservation, error) {
		assert.True(t, id.Resume)
		return &ExecutionReservation{Resume: &ResumeSnapshot{SessionFile: "{}\n", Title: "Kept title", Kind: "tool", Revision: 17, PreviousStatus: "interrupted"}, UsageBaseline: &store.UsageTotals{}, BeginClose: func(context.Context) error { mu.Lock(); order = append(order, "begin"); mu.Unlock(); return nil }, Checkpoint: func(_ context.Context, c ExecutionCheckpoint) error {
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, []string{"begin"}, order)
			assert.Equal(t, int64(17), c.BaseRevision)
			assert.Equal(t, "Kept title", c.Title)
			order = append(order, "checkpoint")
			return nil
		}, Release: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, "release")
			return nil
		}}, nil
	}
	s, err := m.Create(context.Background(), resumeOptions("parent-conversation"))
	require.NoError(t, err)
	assert.Equal(t, InterruptedResumeNotice, s.InterruptionNotice())
	assert.Contains(t, s.resumeNotice, InterruptedResumeNotice)
	require.NoError(t, m.Shutdown(context.Background()))
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"begin", "checkpoint", "release"}, order)
}

func TestOwnerErasureDiscardsRetainedAndLiveWithoutPersistence(t *testing.T) {
	m := newLimitedManager(t)
	var attempts atomic.Int32
	m.cfg.ReserveExecution = func(_ context.Context, id agentexec.Identity, _ *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(context.Context, ExecutionCheckpoint) error {
			if id.OwnerID == 7 {
				attempts.Add(1)
				return errors.New("owner persistence unavailable")
			}
			return nil
		}}, nil
	}
	retained := createSession(t, m, 7)
	require.NoError(t, os.WriteFile(retained.sessionFile, []byte("{\"private\":true}\n"), 0600))
	m.Close(7, retained.ID)
	<-retained.retired
	live := createSession(t, m, 7)
	other := createSession(t, m, 8)
	before := attempts.Load()
	// The integration calls this only after Store.DeleteAccount succeeds.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, m.EraseOwner(ctx, 7))
	assert.Equal(t, before, attempts.Load(), "erasure must not persist deleted history")
	for _, s := range []*Session{retained, live} {
		_, err := os.Stat(s.workDir)
		assert.True(t, os.IsNotExist(err))
		assert.Nil(t, m.cfg.Credentials.Resolve(s.grant.Token()))
	}
	_, err := m.RetainedSnapshot(ctx, 7, retained.ID)
	assert.Error(t, err)
	assert.Same(t, other, m.Get(8, other.ID), "another owner is untouched")
	_, err = m.Create(ctx, CreateOpts{OwnerID: 7, Provider: "anthropic", APIKey: "key"})
	require.Error(t, err)
	require.NoError(t, m.Shutdown(ctx))
}

type synchronizedTestRunner struct {
	local agentexec.Local
	ack   *atomic.Int32
}
type synchronizedTestProcess struct {
	agentexec.Process
	ack *atomic.Int32
}

func (r synchronizedTestRunner) Start(ctx context.Context, c agentexec.StartConfig) (agentexec.Process, error) {
	p, err := r.local.Start(ctx, c)
	if err != nil {
		return nil, err
	}
	return &synchronizedTestProcess{Process: p, ack: r.ack}, nil
}
func (p *synchronizedTestProcess) SynchronizeEvents() func() { return func() { p.ack.Add(1) } }

func TestFailedCheckpointStopsEventAcknowledgment(t *testing.T) {
	m := newLimitedManager(t)
	var acknowledged atomic.Int32
	script := filepath.Join(t.TempDir(), "checkpoint-failure-pi")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"message_end\",\"message\":{\"role\":\"toolResult\"}}'\nexec sleep 60\n"), 0700))
	m.cfg.Runner = synchronizedTestRunner{local: agentexec.Local{PiBin: script, WorkRoot: t.TempDir()}, ack: &acknowledged}
	m.cfg.ReserveExecution = func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(context.Context, ExecutionCheckpoint) error { return errors.New("unacknowledged") }}, nil
	}
	s := createSession(t, m, 7)
	select {
	case <-s.retired:
	case <-time.After(5 * time.Second):
		t.Fatal("failed checkpoint did not stop execution")
	}
	assert.Zero(t, acknowledged.Load(), "reader must not acknowledge failed persistence")
	s.mu.Lock()
	last := string(s.backlog[len(s.backlog)-2])
	s.mu.Unlock()
	assert.Contains(t, last, "extension_error")
	require.NoError(t, m.EraseOwner(context.Background(), 7))
}
