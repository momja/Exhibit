package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reuse the metering test transport so these tests exercise Session's normal
// cap/abort paths without depending on a particular execution backend.
func newInstanceFanoutSession(t *testing.T, m *Manager, id string, ownerID int64, platformPaid bool) *Session {
	t.Helper()
	s, _ := newMeteringSession(t, platformPaid)
	s.ID, s.OwnerID, s.mgr, s.streaming = id, ownerID, m, true
	if m.sessions == nil {
		m.sessions = make(map[string]*Session)
	}
	m.sessions[id] = s
	return s
}

type instanceFanoutSpendStore struct {
	store.Store
	probes atomic.Int32
	err    error
}

func (s *instanceFanoutSpendStore) InstanceAgentSpend(ctx context.Context, since time.Time) (store.UsageTotals, error) {
	s.probes.Add(1)
	if s.err != nil {
		return store.UsageTotals{}, s.err
	}
	return s.Store.InstanceAgentSpend(ctx, since)
}

func instanceFanoutNotices(s *Session) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.backlog...)
}

func requireInstanceFanoutNotice(t *testing.T, s *Session, which, message string) {
	t.Helper()
	require.Eventually(t, func() bool { return len(instanceFanoutNotices(s)) > 0 }, 5*time.Second, time.Millisecond)
	notices := instanceFanoutNotices(s)
	require.Len(t, notices, 1, "one refusal must notify each active session once")
	var notice map[string]string
	require.NoError(t, json.Unmarshal(notices[0], &notice))
	assert.Equal(t, map[string]string{
		"type": "exhibit_spend_cap", "cap": which, "message": message,
	}, notice, "fanout preserves the existing refusal message without exposing model identity")
}

type instanceFanoutAbortRecorder struct {
	io.WriteCloser
	aborts atomic.Int32
}

func (w *instanceFanoutAbortRecorder) Write(p []byte) (int, error) {
	var command struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(p, &command) == nil && command.Type == "abort" {
		w.aborts.Add(1)
	}
	return w.WriteCloser.Write(p)
}

func TestInstanceCapBYOKGuardrailReceiptStopsPlatformPeers(t *testing.T) {
	byok, db := newMeteringSession(t, false)
	m := byok.mgr
	m.cfg.Caps = SpendCaps{InstanceMicrosPerMonth: microsPtr(1_000_000)}
	spend := &instanceFanoutSpendStore{Store: db}
	m.st = spend
	byok.streaming = true
	m.sessions = map[string]*Session{byok.ID: byok}
	first := newInstanceFanoutSession(t, m, "first-platform-owner", 11, true)
	second := newInstanceFanoutSession(t, m, "second-platform-owner", 22, true)
	transports := make(map[*Session]*instanceFanoutAbortRecorder)
	for _, s := range []*Session{byok, first, second} {
		transport := &instanceFanoutAbortRecorder{WriteCloser: s.stdin}
		s.stdin = transport
		transports[s] = transport
	}

	seedSpend(t, db, first.OwnerID, first.ID, store.PaidByPlatform, store.UsageSourceModel, 600_000)
	seedSpend(t, db, second.OwnerID, second.ID, store.PaidByPlatform, store.UsageSourceTool, 300_000)
	seedSpend(t, db, byok.OwnerID, byok.ID, store.PaidByUser, store.UsageSourceModel, 50_000_000)
	require.Nil(t, m.probeInstanceCap(context.Background()), "aggregate platform spend is below the cap")

	// Only the BYOK session reports new usage. Screening is platform spend
	// regardless of who funds the session's own model requests.
	screen := Usage{Input: 100, Output: 1}
	screen.Cost.Total = 0.1
	byok.RecordGuardrailUsage("guard-provider", "guard-model", screen)
	refusal := &CapRefusal{Which: "instance", LimitMicros: 1_000_000, UsedMicros: 1_000_000,
		ResetAt: nextMonth(time.Now())}
	for _, s := range []*Session{first, second} {
		requireInstanceFanoutNotice(t, s, "instance", refusal.Message())
		require.Eventually(t, func() bool { return transports[s].aborts.Load() == 1 }, 5*time.Second, time.Millisecond,
			"each active platform peer must receive the existing abort RPC")
	}
	assert.Empty(t, instanceFanoutNotices(byok), "the BYOK reporter must not be stopped")
	assert.Zero(t, transports[byok].aborts.Load())
	byok.mu.Lock()
	assert.True(t, byok.streaming)
	byok.mu.Unlock()
	assert.NoError(t, byok.capBeforePrompt(), "BYOK prompts stay exempt from platform limits")
	assert.Equal(t, int32(2), spend.probes.Load(), "one baseline probe and one receipt-triggered probe")
}

func TestInstanceCapRefusalStopsPlatformRunsAcrossOwners(t *testing.T) {
	reporting, db := newMeteringSession(t, true)
	m := reporting.mgr
	m.cfg.Caps = SpendCaps{InstanceMicrosPerMonth: microsPtr(1_000_000)}
	spend := &instanceFanoutSpendStore{Store: db}
	m.st = spend
	reporting.ID, reporting.OwnerID, reporting.streaming = "reporting", 11, true
	m.sessions = map[string]*Session{reporting.ID: reporting}
	other := newInstanceFanoutSession(t, m, "other-owner", 22, true)
	unspent := newInstanceFanoutSession(t, m, "unspent-owner", 33, true)
	byok := newInstanceFanoutSession(t, m, "byok", 44, false)
	outside, _ := newMeteringSession(t, true)
	outside.streaming = true // A different core Manager is outside this fanout.

	seedSpend(t, db, reporting.OwnerID, reporting.ID, store.PaidByPlatform, store.UsageSourceModel, 400_000)
	seedSpend(t, db, other.OwnerID, other.ID, store.PaidByPlatform, store.UsageSourceTool, 200_000)
	seedSpend(t, db, byok.OwnerID, byok.ID, store.PaidByUser, store.UsageSourceModel, 50_000_000)
	seedSpend(t, db, byok.OwnerID, byok.ID, store.PaidByPlatform, store.UsageSourceGuardrail, 300_000)

	// No individual session reaches the ceiling. The next usage report crosses
	// it only after summing platform spend across owners, including screening.
	reporting.recordUsage(store.AgentUsage{OwnerID: reporting.OwnerID, SessionID: reporting.ID,
		PaidBy: store.PaidByPlatform, Source: store.UsageSourceModel, CostMicros: 100_000})

	refusal := &CapRefusal{Which: "instance", LimitMicros: 1_000_000, UsedMicros: 1_000_000,
		ResetAt: nextMonth(time.Now())}
	for _, s := range []*Session{reporting, other, unspent} {
		requireInstanceFanoutNotice(t, s, "instance", refusal.Message())
	}
	assert.Empty(t, instanceFanoutNotices(byok), "BYOK keeps running on its owner's key")
	assert.Empty(t, instanceFanoutNotices(outside), "only this manager's sessions are stopped")
	assert.Equal(t, int32(1), spend.probes.Load(), "fanout must not re-probe or recurse")
	assert.NoError(t, byok.capBeforePrompt())
	var nextPromptRefusal *CapRefusal
	require.ErrorAs(t, other.capBeforePrompt(), &nextPromptRefusal)
	assert.Equal(t, "instance", nextPromptRefusal.Which, "future prompts keep the existing budget gate")
}

func TestInstanceCapRefusalFromInactiveReporterStillStopsActivePeers(t *testing.T) {
	for _, state := range []string{"idle", "closed", "stopping"} {
		t.Run(state, func(t *testing.T) {
			reporting, db := newMeteringSession(t, true)
			m := reporting.mgr
			m.cfg.Caps = SpendCaps{InstanceMicrosPerMonth: microsPtr(1_000)}
			reporting.ID, reporting.streaming = "reporting", state != "idle"
			reporting.closed, reporting.stopping = state == "closed", state == "stopping"
			m.sessions = map[string]*Session{reporting.ID: reporting}
			active := newInstanceFanoutSession(t, m, "active-other-owner", 22, true)
			idle := newInstanceFanoutSession(t, m, "idle", 33, true)
			idle.streaming = false
			closed := newInstanceFanoutSession(t, m, "closed", 44, true)
			closed.closed = true // Even an unhealthy closed/streaming combination is inactive.
			stopping := newInstanceFanoutSession(t, m, "stopping", 55, true)
			stopping.stopping = true
			byok := newInstanceFanoutSession(t, m, "byok", 66, false)

			reporting.recordUsage(store.AgentUsage{OwnerID: reporting.OwnerID, SessionID: reporting.ID,
				PaidBy: store.PaidByPlatform, Source: store.UsageSourceModel, CostMicros: 1_000})
			refusal := &CapRefusal{Which: "instance", LimitMicros: 1_000, UsedMicros: 1_000,
				ResetAt: nextMonth(time.Now())}
			requireInstanceFanoutNotice(t, active, "instance", refusal.Message())
			for _, s := range []*Session{reporting, idle, closed, stopping, byok} {
				assert.Empty(t, instanceFanoutNotices(s), "%s must not receive a stop", s.ID)
			}
			// Final usage is still durable even though its emitting run is inactive.
			totals, err := db.SessionAgentSpend(context.Background(), reporting.OwnerID, reporting.ID)
			require.NoError(t, err)
			assert.Equal(t, int64(1_000), totals.CostMicros)
		})
	}
}

func TestInstanceCapProbeFailureStopsPlatformRunsAcrossOwners(t *testing.T) {
	reporting, db := newMeteringSession(t, true)
	m := reporting.mgr
	m.cfg.Caps = SpendCaps{InstanceMicrosPerMonth: microsPtr(1_000)}
	spend := &instanceFanoutSpendStore{Store: db, err: errors.New("database unreachable")}
	m.st = spend
	reporting.streaming = true
	m.sessions = map[string]*Session{reporting.ID: reporting}
	other := newInstanceFanoutSession(t, m, "other-owner", 22, true)
	byok := newInstanceFanoutSession(t, m, "byok", 33, false)

	reporting.checkInstanceCap()
	for _, s := range []*Session{reporting, other} {
		requireInstanceFanoutNotice(t, s, "unresolved", (&CapRefusal{Which: "unresolved"}).Message())
	}
	assert.Empty(t, instanceFanoutNotices(byok))
	assert.Equal(t, int32(1), spend.probes.Load(), "a failed probe must not be retried once per session")
}

func TestInstanceCapFanoutReleasesManagerBeforeTakingSessionLocks(t *testing.T) {
	reporting, db := newMeteringSession(t, true)
	m := reporting.mgr
	spend := &instanceFanoutSpendStore{Store: db}
	m.st = spend
	reporting.streaming = true
	m.sessions = map[string]*Session{reporting.ID: reporting}
	blocked := newInstanceFanoutSession(t, m, "locked-other-owner", 22, true)
	refusal := &CapRefusal{Which: "instance", LimitMicros: 1_000, UsedMicros: 1_000,
		ResetAt: nextMonth(time.Now())}

	blocked.mu.Lock()
	done := make(chan struct{})
	go func() {
		m.stopRunsForInstanceCap(refusal, reporting)
		close(done)
	}()
	defer func() {
		blocked.mu.Unlock()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("fanout did not finish after releasing the session lock")
		}
	}()

	// The reporter is notified first, proving fanout has taken its snapshot
	// and reached the stopping phase while the peer's mutex is unavailable.
	requireInstanceFanoutNotice(t, reporting, "instance", refusal.Message())
	unlocked := m.mu.TryLock()
	if unlocked {
		m.mu.Unlock()
	}
	require.True(t, unlocked, "a busy session must not block manager operations during fanout")
	assert.Equal(t, int32(0), spend.probes.Load(), "applying a refusal performs no fresh probe")
}
