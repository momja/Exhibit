package agent

import (
	"context"
	"errors"
	"os"
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

type executionStore struct {
	store.Store
	reserved    *atomic.Bool
	directSaves atomic.Int32
}

func (s *executionStore) GetTranscript(ctx context.Context, owner int64, artifact, id string) (*store.Transcript, error) {
	if !s.reserved.Load() {
		return nil, errors.New("snapshot read before durable execution reservation")
	}
	return s.Store.GetTranscript(ctx, owner, artifact, id)
}
func (s *executionStore) SessionAgentSpend(ctx context.Context, owner int64, id string) (store.UsageTotals, error) {
	if !s.reserved.Load() {
		return store.UsageTotals{}, errors.New("usage baseline read before durable execution reservation")
	}
	return s.Store.SessionAgentSpend(ctx, owner, id)
}
func (s *executionStore) SaveTranscript(context.Context, int64, store.Transcript) error {
	s.directSaves.Add(1)
	return errors.New("assigned runs must checkpoint through durable authority")
}

func TestExecutionReservationPrecedesResumeReads(t *testing.T) {
	m := newLimitedManager(t)
	ctx := context.Background()
	require.NoError(t, m.st.PutArtifact(ctx, &store.Artifact{ID: "artifact", OwnerID: 7, SourceBlobID: "source", Title: "Artifact", Tier: 1}))
	require.NoError(t, m.st.SaveTranscript(ctx, 7, store.Transcript{ArtifactID: "artifact", SessionID: "conversation", Title: "Kept title", SessionFile: "{}\n"}))
	var reserved atomic.Bool
	wrapped := &executionStore{Store: m.st, reserved: &reserved}
	m.st = wrapped
	var released atomic.Bool
	var mu sync.Mutex
	var checkpoints []ExecutionCheckpoint
	m.cfg.ReserveExecution = func(_ context.Context, id agentexec.Identity, _ *agentscope.Grant) (*ExecutionReservation, error) {
		assert.True(t, id.Resume)
		assert.Equal(t, "conversation", id.SessionID)
		reserved.Store(true)
		return &ExecutionReservation{Checkpoint: func(_ context.Context, checkpoint ExecutionCheckpoint) error {
			mu.Lock()
			defer mu.Unlock()
			checkpoints = append(checkpoints, checkpoint)
			return nil
		}, Release: func(context.Context) error { released.Store(true); return nil }}, nil
	}
	s, err := m.Create(ctx, resumeOptions("conversation"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(s.sessionFile, []byte("{}\n{\"next\":true}\n"), 0600))
	s.handleLine([]byte(`{"type":"message_end","message":{"role":"toolResult","toolCallId":"tool","content":[]}}`))
	s.persistTranscript("artifact")
	require.NoError(t, m.Shutdown(ctx))
	assert.True(t, released.Load())
	assert.Zero(t, wrapped.directSaves.Load())
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, checkpoints, 3)
	assert.Equal(t, "tool", checkpoints[0].Kind)
	assert.Equal(t, "settled", checkpoints[1].Kind)
	for i, checkpoint := range checkpoints {
		assert.Equal(t, uint64(i+1), checkpoint.Sequence)
		assert.Equal(t, "Kept title", checkpoint.Title)
		assert.Contains(t, checkpoint.SessionFile, "next")
	}
}

func TestReplacementGrantOwnsCreateBindingAndPromptProvenance(t *testing.T) {
	m := newLimitedManager(t)
	var original, replacement *agentscope.Grant
	m.cfg.ReserveExecution = func(_ context.Context, id agentexec.Identity, grant *agentscope.Grant) (*ExecutionReservation, error) {
		original = grant
		var err error
		replacement, err = m.cfg.Credentials.Issue(id.OwnerID, id.ArtifactID, id.SessionID)
		return &ExecutionReservation{Grant: replacement, Checkpoint: func(context.Context, ExecutionCheckpoint) error { return nil }}, err
	}
	s := createSession(t, m, 7)
	assert.Same(t, replacement, s.grant)
	assert.Nil(t, m.cfg.Credentials.Resolve(original.Token()), "unfenced original token is immediately revoked")
	replacement.BindArtifact("created-by-fenced-grant")
	assert.Equal(t, "created-by-fenced-grant", s.ArtifactID())
	s.stdin = statsResponder{s: s, respond: func(id string) { s.handleLine([]byte(statsLine(id, 0, 0, 0))) }}
	require.NoError(t, s.Prompt(context.Background(), "change the colors", nil, nil))
	assert.Equal(t, "change the colors", replacement.Prompt())
	s.noteArtifactSaved(map[string]any{"artifactId": "untrusted-result"})
	s.mu.Lock()
	last := string(s.backlog[len(s.backlog)-1])
	s.mu.Unlock()
	assert.Contains(t, last, "created-by-fenced-grant")
	assert.NotContains(t, last, "untrusted-result")
	// Restore the transport so shutdown stops the real fake subprocess.
	s.stdin = s.process
	require.NoError(t, m.Shutdown(context.Background()))
}

func TestFailedFinalCheckpointDoesNotReleaseExecution(t *testing.T) {
	m := newLimitedManager(t)
	var released atomic.Bool
	m.cfg.ReserveExecution = func(context.Context, agentexec.Identity, *agentscope.Grant) (*ExecutionReservation, error) {
		return &ExecutionReservation{Checkpoint: func(context.Context, ExecutionCheckpoint) error { return errors.New("central checkpoint unavailable") }, Release: func(context.Context) error { released.Store(true); return nil }}, nil
	}
	s := createSession(t, m, 7)
	s.mu.Lock()
	s.streaming = true
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.ErrorContains(t, m.Shutdown(ctx), "checkpoint unavailable")
	assert.False(t, released.Load(), "central assignment must remain fenced for recovery")
}
