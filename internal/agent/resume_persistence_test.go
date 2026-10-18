package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A page can close (or lose an idle session) just after a turn settles, before
// its background save finishes. Its identity must stay unavailable for resume
// until that final history is durable, even though it no longer uses a slot.
func TestResumeWaitsForFinalPersistence(t *testing.T) {
	for _, ending := range []string{"close", "eviction", "idle timeout", "natural exit"} {
		t.Run(ending, func(t *testing.T) {
			m := newLimitedManager(t)
			s := resumableConversation(t, m, "conversation")
			final := "{\"turn\":1}\n{\"turn\":2}\n"
			require.NoError(t, os.WriteFile(s.sessionFile, []byte(final), 0o600))
			release := make(chan struct{})
			s.persisting.Add(1)
			go func() { defer s.persisting.Done(); <-release; s.persistTranscript("artifact") }()
			released := false
			defer func() {
				if !released {
					close(release)
					s.persisting.Wait()
				}
			}()

			switch ending {
			case "close":
				m.Close(7, s.ID)
			case "eviction":
				for i := 0; i < MaxOwnerSessions; i++ {
					createSession(t, m, 7)
				}
			case "idle timeout":
				m.reapIdle(time.Now().Add(time.Second))
			case "natural exit":
				// Exit the disposable subprocess without calling a manager close path.
				require.NoError(t, s.cmd.Process.Kill())
			}
			select {
			case <-s.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("process did not exit")
			}
			assert.Nil(t, m.Get(7, s.ID), "a draining session is unavailable to callers")
			_, err := m.Create(context.Background(), resumeOptions(s.ID))
			require.ErrorIs(t, err, ErrSessionLive, "do not load the preceding saved turn")

			close(release)
			released = true
			s.persisting.Wait()
			waitForConversationRelease(t, m, s.ID)
			resumed, err := m.Create(context.Background(), resumeOptions(s.ID))
			require.NoError(t, err)
			b, err := os.ReadFile(resumed.sessionFile)
			require.NoError(t, err)
			assert.Equal(t, final, string(b), "the final settled turn survives continuation")
			resumed.persistTranscript("artifact")
			kept, err := m.st.GetTranscript(context.Background(), 7, "artifact", s.ID)
			require.NoError(t, err)
			assert.Equal(t, final, kept.SessionFile)
		})
	}
}

// A handler's earlier read may race the old process's final save. Create takes
// an identity rather than that snapshot and reloads after reserving it.
func TestResumeLoadsHistoryAfterReservingConversation(t *testing.T) {
	m := newLimitedManager(t)
	s := resumableConversation(t, m, "conversation")
	earlier, err := m.st.GetTranscript(context.Background(), 7, "artifact", s.ID)
	require.NoError(t, err)
	final := earlier.SessionFile + "{\"turn\":2}\n"
	require.NoError(t, os.WriteFile(s.sessionFile, []byte(final), 0o600))
	s.persisting.Add(1)
	go func() { defer s.persisting.Done(); s.persistTranscript("artifact") }()
	m.Close(7, s.ID)
	waitForConversationRelease(t, m, s.ID)

	resumed, err := m.Create(context.Background(), resumeOptions(earlier.SessionID))
	require.NoError(t, err)
	b, err := os.ReadFile(resumed.sessionFile)
	require.NoError(t, err)
	assert.Equal(t, final, string(b), "a caller's earlier read must never seed the process")
}

type pausedResumeStore struct {
	store.Store
	entered chan struct{}
	release chan struct{}
}

func (s *pausedResumeStore) GetTranscript(ctx context.Context, owner int64, artifact, session string) (*store.Transcript, error) {
	close(s.entered)
	<-s.release
	return s.Store.GetTranscript(ctx, owner, artifact, session)
}

// Duplicate requests must be excluded before either can spawn a process or
// take a snapshot. A slow database read must not hold the registry mutex.
func TestConcurrentResumeReservesBeforeLoadingHistory(t *testing.T) {
	m := newLimitedManager(t)
	s := resumableConversation(t, m, "conversation")
	m.Close(7, s.ID)
	waitForConversationRelease(t, m, s.ID)
	paused := &pausedResumeStore{Store: m.st, entered: make(chan struct{}), release: make(chan struct{})}
	m.st = paused
	done := make(chan error, 1)
	go func() { _, err := m.Create(context.Background(), resumeOptions(s.ID)); done <- err }()
	<-paused.entered
	// Always release the first request, including when an assertion fails.
	defer func() { close(paused.release); require.NoError(t, <-done) }()
	_, err := m.Create(context.Background(), resumeOptions(s.ID))
	assert.ErrorIs(t, err, ErrSessionLive)
	assert.Nil(t, m.Get(7, "missing"), "other registry operations remain available")
}

// A failed Pi startup can exit before Create has returned. Repeated attempts
// must release the identity and scratch directory, including for a resume.
func TestImmediateProcessExitReleasesConversation(t *testing.T) {
	m := newLimitedManager(t)
	ctx := context.Background()
	require.NoError(t, m.st.PutArtifact(ctx, &store.Artifact{
		ID: "artifact", OwnerID: 7, Title: "Counter", SourceBlobID: "body", Tier: 1,
	}))
	require.NoError(t, m.st.SaveTranscript(ctx, 7, store.Transcript{
		ArtifactID: "artifact", SessionID: "conversation", SessionFile: "{\"turn\":1}\n",
	}))
	script := filepath.Join(t.TempDir(), "exiting-pi")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o700))
	m.cfg.PiBin = script
	for _, resumeID := range []string{"", "conversation"} {
		t.Run("resume="+resumeID, func(t *testing.T) {
			for range 32 {
				s, err := m.Create(ctx, resumeOptions(resumeID))
				require.NoError(t, err)
				select {
				case <-s.Done():
				case <-time.After(3 * time.Second):
					t.Fatal("failed startup did not finish")
				}
				waitForConversationRelease(t, m, s.ID)
				require.Eventually(t, func() bool {
					_, err := os.Stat(s.workDir)
					return os.IsNotExist(err)
				}, 3*time.Second, time.Millisecond)
				assert.Nil(t, m.Get(7, s.ID))
			}
		})
	}
}

func resumableConversation(t *testing.T, m *Manager, id string) *Session {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, m.st.PutArtifact(ctx, &store.Artifact{
		ID: "artifact", OwnerID: 7, Title: "Counter", SourceBlobID: "body", Tier: 1,
	}))
	require.NoError(t, m.st.SaveTranscript(ctx, 7, store.Transcript{
		ArtifactID: "artifact", SessionID: id, Title: "Counter", SessionFile: "{\"turn\":1}\n",
	}))
	s, err := m.Create(ctx, resumeOptions(id))
	require.NoError(t, err)
	return s
}

func resumeOptions(id string) CreateOpts {
	return CreateOpts{OwnerID: 7, ArtifactID: "artifact", Provider: "anthropic", APIKey: "k", ResumeSessionID: id}
}

func waitForConversationRelease(t *testing.T, m *Manager, id string) {
	t.Helper()
	require.Eventually(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.sessions[id] == nil
	}, 3*time.Second, time.Millisecond)
}
