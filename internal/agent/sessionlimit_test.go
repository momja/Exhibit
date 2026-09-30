package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
