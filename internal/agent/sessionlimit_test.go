package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLimitedManager runs Create against a fake pi: a script that stays alive
// and answers nothing, which is all the admission tests need — sessions in
// the registry with a live subprocess behind them.
func newLimitedManager(t *testing.T) *Manager {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fakepi.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755))

	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	m, err := New(Config{
		PiBin:       script,
		WorkRoot:    t.TempDir(),
		APIBaseURL:  "http://app.test",
		Credentials: agentscope.NewRegistry(),
	}, db)
	require.NoError(t, err)
	t.Cleanup(func() {
		m.mu.Lock()
		all := make([]*Session, 0, len(m.sessions))
		for _, s := range m.sessions {
			all = append(all, s)
		}
		m.mu.Unlock()
		for _, s := range all {
			s.kill()
		}
	})
	return m
}

func createSession(t *testing.T, m *Manager, ownerID int64) *Session {
	t.Helper()
	s, err := m.Create(context.Background(), CreateOpts{OwnerID: ownerID, Provider: "anthropic", APIKey: "k"})
	require.NoError(t, err)
	return s
}

// An owner holds at most MaxOwnerSessions open sessions — what makes the
// spend cap's overshoot bound finite (at most one run per open session). At
// the limit the oldest *idle* one is evicted rather than the new one refused:
// the chat page closes its session on pagehide, but a reload storm must not
// lock its owner out for the idle timeout.
func TestAnOwnerHoldsAtMostTenOpenSessions(t *testing.T) {
	m := newLimitedManager(t)

	var ids [MaxOwnerSessions]string
	for i := range ids {
		ids[i] = createSession(t, m, 7).ID
	}

	extra := createSession(t, m, 7)
	assert.NotNil(t, m.Get(7, extra.ID), "the eleventh session is admitted")
	assert.Nil(t, m.Get(7, ids[0]), "by evicting the oldest idle session")

	kept := 0
	for _, id := range ids[1:] {
		if m.Get(7, id) != nil {
			kept++
		}
	}
	assert.Equal(t, MaxOwnerSessions-1, kept, "exactly one session was evicted")

	other := createSession(t, m, 8)
	assert.NotNil(t, m.Get(8, other.ID), "another owner's sessions are untouched")
}

// The refusal is only for the case eviction cannot help with: every session
// is mid-run, and none of them is the manager's to kill.
func TestCreateRefusesWhenEverySessionIsBusy(t *testing.T) {
	m := newLimitedManager(t)

	for i := 0; i < MaxOwnerSessions; i++ {
		s := createSession(t, m, 7)
		s.mu.Lock()
		s.streaming = true
		s.mu.Unlock()
	}

	_, err := m.Create(context.Background(), CreateOpts{OwnerID: 7, Provider: "anthropic", APIKey: "k"})
	assert.ErrorIs(t, err, ErrSessionLimit)
	assert.Contains(t, err.Error(), "all of yours are busy", "the message says what to do about it")
}

func TestConcurrentReservationsCountStartupCapacity(t *testing.T) {
	m := newLimitedManager(t)
	start := make(chan struct{})
	releases := make(chan func(), 100)
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			release, err := m.reserveProcess(context.Background(), 7)
			if err == nil {
				releases <- release
			} else {
				assert.ErrorIs(t, err, ErrSessionLimit)
			}
		}()
	}
	close(start)
	workers.Wait()
	close(releases)
	assert.Equal(t, MaxOwnerSessions, len(releases))
	m.mu.Lock()
	assert.Equal(t, MaxOwnerSessions, m.processCount)
	m.mu.Unlock()
	for release := range releases {
		release()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Zero(t, m.processCount)
	assert.Empty(t, m.processOwners)
}

func TestBusyOwnerRefusalsNeverSpawn(t *testing.T) {
	m := newLimitedManager(t)
	for i := 0; i < MaxOwnerSessions; i++ {
		s := createSession(t, m, 7)
		s.mu.Lock()
		s.streaming = true
		s.mu.Unlock()
	}
	marker := filepath.Join(t.TempDir(), "spawned")
	script := filepath.Join(t.TempDir(), "refused-pi")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700))
	m.cfg.PiBin = script
	var workers sync.WaitGroup
	for i := 0; i < 50; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := m.Create(context.Background(), CreateOpts{OwnerID: 7, Provider: "anthropic"})
			assert.ErrorIs(t, err, ErrSessionLimit)
		}()
	}
	workers.Wait()
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "refused requests must not execute the process")
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Equal(t, MaxOwnerSessions, m.processCount)
}

func TestInstanceLimitIncludesPendingStartsAndReturnsExitedSlots(t *testing.T) {
	m := newLimitedManager(t)
	var releases []func()
	for i := 0; i < MaxInstanceSessions-1; i++ {
		release, err := m.reserveProcess(context.Background(), int64(i+100))
		require.NoError(t, err)
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	s := createSession(t, m, 7)
	s.mu.Lock()
	s.streaming = true
	s.mu.Unlock()
	_, err := m.Create(context.Background(), CreateOpts{OwnerID: 8, Provider: "anthropic"})
	require.ErrorIs(t, err, ErrInstanceSessionLimit)
	m.Close(7, s.ID)
	select {
	case <-s.processExited:
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit")
	}
	replacement := createSession(t, m, 8)
	require.NotNil(t, replacement)
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Equal(t, MaxInstanceSessions, m.processCount)
}

func TestFailedStartupReturnsReservation(t *testing.T) {
	m := newLimitedManager(t)
	m.cfg.PiBin = filepath.Join(t.TempDir(), "does-not-exist")
	for i := 0; i < MaxOwnerSessions+1; i++ {
		_, err := m.Create(context.Background(), CreateOpts{OwnerID: 7, Provider: "anthropic"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrSessionLimit)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Zero(t, m.processCount)
	assert.Empty(t, m.processOwners)
}

func TestConcurrentInstanceReservationsNeverExceedCeiling(t *testing.T) {
	m := newLimitedManager(t)
	start := make(chan struct{})
	releases := make(chan func(), 100)
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func(owner int64) {
			defer workers.Done()
			<-start
			release, err := m.reserveProcess(context.Background(), owner)
			if err == nil {
				releases <- release
			} else {
				assert.ErrorIs(t, err, ErrInstanceSessionLimit)
			}
		}(int64(i + 1))
	}
	close(start)
	workers.Wait()
	close(releases)
	assert.Equal(t, MaxInstanceSessions, len(releases))
	for release := range releases {
		release()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Zero(t, m.processCount)
	assert.Empty(t, m.processOwners)
}
