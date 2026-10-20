package agentcontrol_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	s     *store.SQLiteStore
	c     *agentcontrol.Control
	reg   *agentscope.Registry
	clock atomic.Int64
	path  string
}

var ctx = context.Background()

const file1 = "{\"type\":\"session\",\"id\":\"pi-id\"}\n{\"type\":\"message\",\"message\":{\"role\":\"user\",\"content\":\"hi\"}}\n"

func setup(t *testing.T, limit int) *fixture {
	t.Helper()
	f := &fixture{path: filepath.Join(t.TempDir(), "app.db")}
	f.clock.Store(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC).UnixMilli())
	var err error
	f.s, err = store.OpenSQLite(f.path)
	require.NoError(t, err)
	t.Cleanup(func() { f.s.Close() })
	f.newControl(limit)
	for _, a := range []struct {
		owner int64
		id    string
	}{{1, "artifact"}, {1, "other"}, {2, "foreign"}} {
		require.NoError(t, f.s.PutArtifact(ctx, &store.Artifact{ID: a.id, OwnerID: a.owner, Title: a.id, SourceBlobID: a.id + "-body", Tier: store.Tier1}))
	}
	return f
}
func (f *fixture) newControl(limit int) {
	f.reg = agentscope.NewRegistry()
	f.c = agentcontrol.New(f.s, f.reg, agentcontrol.Config{FleetLimit: limit, LeaseTTL: time.Minute, SupervisorTTL: time.Hour, Now: func() time.Time { return time.UnixMilli(f.clock.Load()) }})
}
func request(owner int64, conversation string) agentcontrol.Reservation {
	return agentcontrol.Reservation{OwnerID: owner, ConversationID: conversation, ExecutionID: "exec-" + conversation, WorkerID: "worker-" + conversation, ArtifactID: "artifact"}
}
func (f *fixture) reserve(t *testing.T, conversation string) agentcontrol.Assignment {
	t.Helper()
	a, err := f.c.Reserve(ctx, request(1, conversation))
	require.NoError(t, err)
	return a
}
func (f *fixture) checkpoint(t *testing.T, a agentcontrol.Assignment, revision int64, kind, file string) agentcontrol.Snapshot {
	t.Helper()
	p, err := f.c.WriteCheckpoint(ctx, a.Identity, agentcontrol.Checkpoint{Revision: revision, Kind: kind, Title: "Original title", SessionFile: file})
	require.NoError(t, err)
	return p
}
func (f *fixture) resume(conversation, execution string) agentcontrol.Reservation {
	r := request(1, conversation)
	r.Resume = true
	r.ExecutionID = execution
	r.WorkerID = "replacement-" + execution
	return r
}

func TestAdmissionIsAtomicAndRetriesDoNotProvisionAnotherExecution(t *testing.T) {
	f := setup(t, 12)
	var wg sync.WaitGroup
	var admitted atomic.Int32
	assignments := make(chan agentcontrol.Assignment, 30)
	failures := make(chan error, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, err := f.c.Reserve(ctx, request(1, fmt.Sprint(i)))
			if err == nil {
				admitted.Add(1)
				assignments <- a
			} else {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(assignments)
	close(failures)
	require.Equal(t, int32(10), admitted.Load())
	for err := range failures {
		require.ErrorIs(t, err, agentcontrol.ErrOwnerLimit)
	}
	var first agentcontrol.Assignment
	for a := range assignments {
		first = a
		require.NoError(t, f.c.Activate(ctx, a.Identity))
	}
	again, err := f.c.Reserve(ctx, request(1, first.ConversationID))
	require.NoError(t, err)
	assert.Equal(t, first.ExecutionID, again.ExecutionID)
	assert.Equal(t, int64(1), again.Generation)
	conflicting := request(1, first.ConversationID)
	conflicting.WorkerID = "imposter"
	_, err = f.c.Reserve(ctx, conflicting)
	require.ErrorIs(t, err, agentcontrol.ErrConflict)
	for i := 0; i < 2; i++ {
		r := request(2, fmt.Sprintf("second-%d", i))
		r.ArtifactID = "foreign"
		_, err = f.c.Reserve(ctx, r)
		require.NoError(t, err)
	}
	r := request(3, "over-fleet")
	r.ArtifactID = ""
	_, err = f.c.Reserve(ctx, r)
	require.ErrorIs(t, err, agentcontrol.ErrFleetLimit)
	require.NoError(t, f.c.Release(ctx, first.Identity, agentcontrol.Failed))
	require.NoError(t, f.c.Release(ctx, first.Identity, agentcontrol.Failed))
	_, err = f.c.Reserve(ctx, r)
	require.NoError(t, err, "failed startup returns its capacity")
	_, err = f.c.Reserve(ctx, request(1, first.ConversationID))
	require.ErrorIs(t, err, agentcontrol.ErrStale, "retry never resurrects failed execution")
}

func TestIndependentSQLiteConnectionsSerializeAdmission(t *testing.T) {
	f := setup(t, 3)
	other, err := store.OpenSQLite(f.path)
	require.NoError(t, err)
	defer other.Close()
	c2 := agentcontrol.New(other, agentscope.NewRegistry(), agentcontrol.Config{FleetLimit: 3, Now: func() time.Time { return time.UnixMilli(f.clock.Load()) }})
	var wg sync.WaitGroup
	var count atomic.Int32
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := f.c
			if i%2 == 0 {
				c = c2
			}
			_, err := c.Reserve(ctx, request(1, fmt.Sprint(i)))
			if err == nil {
				count.Add(1)
			} else {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	assert.Equal(t, int32(3), count.Load())
	for err := range failures {
		assert.ErrorIs(t, err, agentcontrol.ErrFleetLimit)
	}
}

func TestResumeReservesBeforeSnapshotReadAndRetainsRevisionTitleAndSpend(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "conversation")
	f.checkpoint(t, a, 1, agentcontrol.CheckpointSettled, file1)
	require.NoError(t, f.c.BeginClose(ctx, a.Identity))
	_, err := f.c.Reserve(ctx, f.resume(a.ConversationID, "early"))
	require.ErrorIs(t, err, agentcontrol.ErrAssigned)
	final := file1 + "{\"type\":\"message\",\"turn\":2}\n"
	f.checkpoint(t, a, 2, agentcontrol.CheckpointSettled, final)
	require.NoError(t, f.c.Release(ctx, a.Identity, agentcontrol.Closed))
	var wg sync.WaitGroup
	results := make(chan agentcontrol.Assignment, 20)
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, err := f.c.Reserve(ctx, f.resume("conversation", fmt.Sprint(i)))
			if err == nil {
				results <- a
			} else {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	require.Len(t, results, 1)
	resumed := <-results
	assert.Equal(t, int64(2), resumed.Generation)
	assert.Equal(t, int64(2), resumed.Revision)
	for err := range failures {
		require.ErrorIs(t, err, agentcontrol.ErrAssigned)
	}
	snapshot, err := f.c.ReadSnapshot(ctx, resumed.Identity)
	require.NoError(t, err)
	assert.Equal(t, final, snapshot.SessionFile)
	assert.Equal(t, "Original title", snapshot.Title)
	_, err = f.c.ReadSnapshot(ctx, a.Identity)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	f.checkpoint(t, resumed, 3, agentcontrol.CheckpointTool, final+"{\"type\":\"tool\"}\n")
}

func TestCheckpointOrderingRetriesValidationAndCoreVersionProvenance(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "snapshot")
	p := agentcontrol.Checkpoint{Revision: 2, Kind: agentcontrol.CheckpointSettled, Title: "Original title", SessionFile: file1}
	saved, err := f.c.WriteCheckpoint(ctx, a.Identity, p)
	require.NoError(t, err)
	assert.Equal(t, 1, saved.VersionSeq)
	blob := "new-body"
	_, err = f.s.CommitVersion(ctx, 1, "artifact", store.VersionChange{BodyBlobID: &blob, Provenance: store.Provenance{Origin: store.VersionAgent, SessionID: a.ConversationID}})
	require.NoError(t, err)
	same, err := f.c.WriteCheckpoint(ctx, a.Identity, p)
	require.NoError(t, err)
	assert.Equal(t, saved, same, "retry retains original provenance")
	old := p
	old.Revision = 1
	_, err = f.c.WriteCheckpoint(ctx, a.Identity, old)
	require.ErrorIs(t, err, agentcontrol.ErrConflict)
	changed := p
	changed.SessionFile += "{\"type\":\"different\"}\n"
	_, err = f.c.WriteCheckpoint(ctx, a.Identity, changed)
	require.ErrorIs(t, err, agentcontrol.ErrConflict)
	changed.Revision = 3
	changed.Title = "rewritten title"
	_, err = f.c.WriteCheckpoint(ctx, a.Identity, changed)
	require.ErrorIs(t, err, agentcontrol.ErrConflict)
	for _, file := range []string{"", "{\"incomplete\":1}", "not json\n", "[]\n", "{bad}\n"} {
		bad := p
		bad.Revision = 3
		bad.SessionFile = file
		_, err = f.c.WriteCheckpoint(ctx, a.Identity, bad)
		require.ErrorIs(t, err, agentcontrol.ErrInvalid)
	}
	require.NoError(t, f.c.RecordArtifactVersion(ctx, a.Identity, 2))
	require.ErrorIs(t, f.c.RecordArtifactVersion(ctx, a.Identity, 1), agentcontrol.ErrConflict)
	browserBlob := "browser-body"
	_, err = f.s.CommitVersion(ctx, 1, "artifact", store.VersionChange{BodyBlobID: &browserBlob})
	require.NoError(t, err)
	require.ErrorIs(t, f.c.RecordArtifactVersion(ctx, a.Identity, 3), agentcontrol.ErrConflict)
	p.Revision = 3
	p.SessionFile += "{\"type\":\"tool\"}\n"
	p.Kind = agentcontrol.CheckpointTool
	saved, err = f.c.WriteCheckpoint(ctx, a.Identity, p)
	require.NoError(t, err)
	assert.Equal(t, 2, saved.VersionSeq)
	foreign := a.Identity
	foreign.OwnerID = 2
	_, err = f.c.WriteCheckpoint(ctx, foreign, p)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	kept, err := f.s.GetTranscript(ctx, 1, "artifact", "snapshot")
	require.NoError(t, err)
	assert.Equal(t, p.SessionFile, kept.SessionFile)
	summaries, err := f.s.ListTranscripts(ctx, 1, "artifact")
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Empty(t, summaries[0].SessionFile)
}

func TestToolFencesAndCreateBinding(t *testing.T) {
	f := setup(t, 32)
	r := request(1, "create")
	r.ArtifactID = ""
	a, err := f.c.Reserve(ctx, r)
	require.NoError(t, err)
	g, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	require.NotNil(t, f.reg.Resolve(g.Token()))
	assert.Equal(t, a.Generation, g.Execution().Generation)
	require.ErrorIs(t, g.BindArtifact("foreign"), agentcontrol.ErrNotFound)
	release, err := g.Guard(ctx)
	require.NoError(t, err)
	require.NoError(t, g.BindArtifactContext(ctx, "artifact"))
	release()
	assert.Equal(t, "artifact", g.Scope().ArtifactID)
	durable, err := f.s.GetAgentExecution(ctx, a.Identity, time.UnixMilli(f.clock.Load()))
	require.NoError(t, err)
	assert.Equal(t, "artifact", durable.ArtifactID)
	require.NoError(t, f.c.Bind(ctx, a.Identity, "artifact"))
	require.ErrorIs(t, f.c.Bind(ctx, a.Identity, "other"), agentcontrol.ErrConflict)
	require.Error(t, g.BindArtifact("other"))
	require.NotNil(t, f.reg.Resolve(g.Token()))
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			grant := f.reg.ResolveContext(ctx, g.Token())
			if assert.NotNil(t, grant) {
				release, err := grant.Guard(ctx)
				if assert.NoError(t, err) {
					release()
				}
			}
		}()
	}
	wg.Wait()
	foreign := a.Identity
	foreign.OwnerID = 2
	_, err = f.c.IssueTool(ctx, foreign)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	foreign = a.Identity
	foreign.Generation++
	_, err = f.c.Guard(ctx, foreign)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	foreign = a.Identity
	foreign.WorkerID = "foreign-worker"
	_, err = f.c.Guard(ctx, foreign)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	require.NoError(t, f.c.Release(ctx, a.Identity, agentcontrol.Closed))
	assert.Nil(t, f.reg.Resolve(g.Token()))
	_, err = g.Guard(ctx)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	assert.True(t, agentscope.IsCredential(g.Token()))
	assert.True(t, agentscope.IsCredential("exagent_retired"))
}

func TestTakeoverWaitsForInFlightWriteAndExpiredBearerCannotReturn(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "takeover")
	f.checkpoint(t, a, 1, agentcontrol.CheckpointSettled, file1)
	g, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	release, err := g.Guard(ctx)
	require.NoError(t, err)
	f.clock.Add(int64((2 * time.Minute) / time.Millisecond))
	takeover := make(chan agentcontrol.Assignment, 1)
	failure := make(chan error, 1)
	go func() {
		next, err := f.c.Reserve(ctx, f.resume("takeover", "new-execution"))
		if err != nil {
			failure <- err
		} else {
			takeover <- next
		}
	}()
	select {
	case err := <-failure:
		t.Fatal(err)
	case <-takeover:
		t.Fatal("takeover crossed in-flight fence")
	case <-time.After(30 * time.Millisecond):
	}
	// A write committed under the guard remains before the replacement's read.
	require.NoError(t, f.s.UpdateArtifact(ctx, 1, "artifact", map[string]any{"title": "completed under old fence"}))
	release()
	release()
	select {
	case err := <-failure:
		t.Fatal(err)
	case next := <-takeover:
		assert.Equal(t, int64(2), next.Generation)
	case <-time.After(2 * time.Second):
		t.Fatal("takeover did not resume")
	}
	assert.Nil(t, f.reg.Resolve(g.Token()))
	_, err = g.Guard(ctx)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	require.ErrorIs(t, f.c.Renew(ctx, a.Identity), agentcontrol.ErrStale)
	require.ErrorIs(t, f.c.Bind(ctx, a.Identity, "other"), agentcontrol.ErrStale)
}

func TestReleaseWaitsThroughFinalMutation(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "release")
	g, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	release, err := g.Guard(ctx)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- f.c.Release(ctx, a.Identity, agentcontrol.Closed) }()
	select {
	case <-done:
		t.Fatal("release crossed in-flight write")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	require.NoError(t, <-done)
	assert.Nil(t, f.reg.Resolve(g.Token()))
}

func TestRestartReconciliationKeepsSnapshotAndHistoricalUsageAuthority(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "restart")
	f.checkpoint(t, a, 1, agentcontrol.CheckpointTool, file1)
	secret, err := f.c.IssueSupervisor(ctx, a.Identity, "platform")
	require.NoError(t, err)
	require.NoError(t, f.c.Activate(ctx, a.Identity))
	require.NoError(t, f.c.Renew(ctx, a.Identity))
	require.NoError(t, f.s.Close())
	f.s, err = store.OpenSQLite(f.path)
	require.NoError(t, err)
	f.newControl(32)
	interrupted, err := f.c.InterruptAll(ctx)
	require.NoError(t, err)
	require.Len(t, interrupted, 1)
	assert.Equal(t, agentcontrol.Interrupted, interrupted[0].Status)
	none, err := f.c.InterruptAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, none)
	_, err = f.c.Guard(ctx, a.Identity)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	next, err := f.c.Reserve(ctx, f.resume("restart", "replacement"))
	require.NoError(t, err)
	assert.Equal(t, int64(2), next.Generation)
	snap, err := f.c.ReadSnapshot(ctx, next.Identity)
	require.NoError(t, err)
	assert.Equal(t, agentcontrol.Interrupted, snap.PreviousStatus)
	assert.Equal(t, agentcontrol.CheckpointTool, snap.Kind, "a completed tool checkpoint is not a settled turn")
	supervisor, err := f.c.AuthenticateSupervisor(ctx, secret)
	require.NoError(t, err)
	recorded, err := f.c.ReportUsage(ctx, supervisor, agentcontrol.Usage{EventID: "late", Source: "model", InputTokens: 7})
	require.NoError(t, err)
	assert.True(t, recorded)
	f.clock.Add(int64((2 * time.Minute) / time.Millisecond))
	expired, err := f.c.ReconcileExpired(ctx)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, next.ExecutionID, expired[0].ExecutionID)
	require.ErrorIs(t, f.c.Renew(ctx, next.Identity), agentcontrol.ErrStale)
	f.clock.Add(int64((time.Hour) / time.Millisecond))
	_, err = f.c.AuthenticateSupervisor(ctx, secret)
	require.ErrorIs(t, err, agentcontrol.ErrCredential)
}

func TestLateUsageIsDeduplicatedInExistingLedgerWithCentralFunding(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "usage")
	f.checkpoint(t, a, 1, agentcontrol.CheckpointSettled, file1)
	secret, err := f.c.IssueSupervisor(ctx, a.Identity, "user")
	require.NoError(t, err)
	s, err := f.c.AuthenticateSupervisor(ctx, secret)
	require.NoError(t, err)
	require.NoError(t, f.c.Release(ctx, a.Identity, agentcontrol.Closed))
	_, err = f.c.Reserve(ctx, f.resume("usage", "replacement"))
	require.NoError(t, err)
	u := agentcontrol.Usage{EventID: "stable-model-8", Provider: "provider", Model: "model", Source: "model", InputTokens: 40, OutputTokens: 10, CacheReadTokens: 3, CacheWriteTokens: 2, CostMicros: 100}
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inserted, err := f.c.ReportUsage(ctx, s, u)
			if assert.NoError(t, err) && inserted {
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), count.Load())
	changed := u
	changed.InputTokens++
	_, err = f.c.ReportUsage(ctx, s, changed)
	require.ErrorIs(t, err, agentcontrol.ErrConflict)
	for _, source := range []string{"compaction", "tool", "guardrail"} {
		event := u
		event.EventID = source
		event.Source = source
		_, err = f.c.ReportUsage(ctx, s, event)
		require.NoError(t, err)
	}
	spend, err := f.s.SessionAgentSpend(ctx, 1, a.ConversationID)
	require.NoError(t, err)
	assert.Equal(t, int64(300), spend.CostMicros)
	assert.Equal(t, int64(120), spend.InputTokens)
	instance, err := f.s.InstanceAgentSpend(ctx, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, int64(100), instance.CostMicros, "guardrail funding stays platform even for BYOK")
	forged := s
	forged.OwnerID = 2
	_, err = f.c.ReportUsage(ctx, forged, u)
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	forged = s
	forged.PaidBy = "platform"
	_, err = f.c.ReportUsage(ctx, forged, u)
	require.ErrorIs(t, err, agentcontrol.ErrCredential)
	_, err = f.c.ReportUsage(ctx, s, agentcontrol.Usage{EventID: "negative", Source: "model", InputTokens: -1})
	require.ErrorIs(t, err, agentcontrol.ErrInvalid)
}

func TestResumeMissingForeignAndLegacyConversations(t *testing.T) {
	f := setup(t, 32)
	for _, r := range []agentcontrol.Reservation{f.resume("missing", "missing"), {OwnerID: 2, ConversationID: "foreign", ExecutionID: "foreign", WorkerID: "w", ArtifactID: "artifact", Resume: true}} {
		_, err := f.c.Reserve(ctx, r)
		require.ErrorIs(t, err, agentcontrol.ErrNotFound)
	}
	// The old Store path still imports resumable conversations without having a
	// durable assignment row from before this migration.
	require.NoError(t, f.s.SaveTranscript(ctx, 1, store.Transcript{ArtifactID: "artifact", SessionID: "imported", Title: "legacy title", SessionFile: file1}))
	a, err := f.c.Reserve(ctx, f.resume("imported", "import"))
	require.NoError(t, err)
	snapshot, err := f.c.ReadSnapshot(ctx, a.Identity)
	require.NoError(t, err)
	assert.Equal(t, "legacy title", snapshot.Title)
	assert.Zero(t, snapshot.Revision)
}

func TestReconcileExpiredDoesNotInterruptLiveLease(t *testing.T) {
	f := setup(t, 32)
	first := f.reserve(t, "first")
	f.clock.Add(30 * 1000)
	second := f.reserve(t, "second")
	f.clock.Add(31 * 1000)
	expired, err := f.c.ReconcileExpired(ctx)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, first.ExecutionID, expired[0].ExecutionID)
	release, err := f.c.Guard(ctx, second.Identity)
	require.NoError(t, err)
	release()
	require.NoError(t, f.c.WithExecution(ctx, second.Identity, func(context.Context) error { return nil }))
	want := errors.New("operation failed")
	require.ErrorIs(t, f.c.WithExecution(ctx, second.Identity, func(context.Context) error { return want }), want)
}

func TestReissuedCreateGrantsCannotPerformTwoFirstSaves(t *testing.T) {
	f := setup(t, 32)
	r := request(1, "two-grants")
	r.ArtifactID = ""
	a, err := f.c.Reserve(ctx, r)
	require.NoError(t, err)
	first, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	second, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	release, err := first.Guard(ctx)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		unlock, err := second.Guard(ctx)
		if err == nil {
			unlock()
		}
		result <- err
	}()
	select {
	case <-result:
		t.Fatal("second create crossed the assignment create guard")
	case <-time.After(30 * time.Millisecond):
	}
	require.NoError(t, first.BindArtifact("artifact"))
	release()
	require.ErrorIs(t, <-result, agentcontrol.ErrStale)
	assert.Nil(t, f.reg.Resolve(second.Token()))
}

func TestFailedReservationCanBeReplacedWithoutPretendingToResume(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "failed-start")
	require.NoError(t, f.c.Release(ctx, a.Identity, agentcontrol.Failed))
	r := request(1, "failed-start")
	r.ExecutionID = "retry-attempt"
	r.WorkerID = "worker-retry"
	replacement, err := f.c.Reserve(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, int64(2), replacement.Generation)
	f.checkpoint(t, replacement, 1, agentcontrol.CheckpointSettled, file1)
	require.NoError(t, f.c.Release(ctx, replacement.Identity, agentcontrol.Closed))
	r.ExecutionID = "would-reset-history"
	_, err = f.c.Reserve(ctx, r)
	require.ErrorIs(t, err, agentcontrol.ErrAssigned)
}

// A tool request may spend most of its time reading an upload or staging a
// save. That held operation fence must not stall another worker's lifecycle.
func TestConversationGuardDoesNotBlockOtherLifecycle(t *testing.T) {
	f := setup(t, 2)
	a := f.reserve(t, "slow-save")
	release, err := f.c.Guard(ctx, a.Identity)
	require.NoError(t, err)
	defer release()
	operationCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		b, err := f.c.Reserve(operationCtx, request(1, "healthy-worker"))
		if err != nil {
			done <- err
			return
		}
		f.clock.Add(50 * 1000)
		if err := f.c.Renew(operationCtx, b.Identity); err != nil {
			done <- err
			return
		}
		// B's original lease would now have expired. Its independent renewal keeps
		// it authoritative while A still holds its operation fence.
		f.clock.Add(20 * 1000)
		unlock, err := f.c.Guard(operationCtx, b.Identity)
		if err != nil {
			done <- err
			return
		}
		unlock()
		done <- f.c.Release(operationCtx, b.Identity, agentcontrol.Closed)
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated lifecycle waited for A's operation")
	}
}

func TestReserveReclaimsExpiredCapacityWithoutRevokingAnotherConversation(t *testing.T) {
	f := setup(t, 1)
	a := f.reserve(t, "expired-save")
	release, err := f.c.Guard(ctx, a.Identity)
	require.NoError(t, err)
	defer release()
	f.clock.Add(61 * 1000)
	operationCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := f.c.Reserve(operationCtx, request(1, "replacement-capacity")); done <- err }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("unrelated admission waited for A's operation")
	}
	candidates, err := f.s.ListAgentExecutionsToReconcile(ctx, false, time.UnixMilli(f.clock.Load()))
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, a.ExecutionID, candidates[0].ExecutionID)
	assert.Equal(t, agentcontrol.Reserved, candidates[0].Status, "admission must not mutate authority outside its conversation fence")
}

func TestUnboundCreatesInDifferentConversationsDoNotBlock(t *testing.T) {
	f := setup(t, 32)
	grants := make([]*agentscope.Grant, 0, 2)
	for _, conversation := range []string{"create-a", "create-b"} {
		r := request(1, conversation)
		r.ArtifactID = ""
		a, err := f.c.Reserve(ctx, r)
		require.NoError(t, err)
		g, err := f.c.IssueTool(ctx, a.Identity)
		require.NoError(t, err)
		grants = append(grants, g)
	}
	release, err := grants[0].Guard(ctx)
	require.NoError(t, err)
	defer release()
	operationCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		unlock, err := grants[1].Guard(operationCtx)
		if err != nil {
			done <- err
			return
		}
		defer unlock()
		done <- grants[1].BindArtifactContext(operationCtx, "other")
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("B's first save waited for A's create")
	}
	assert.Empty(t, grants[0].Scope().ArtifactID)
	assert.Equal(t, "other", grants[1].Scope().ArtifactID)
}

func TestSameConversationWaitersHonorCancellation(t *testing.T) {
	for _, operation := range []string{"reserve", "release", "reconcile"} {
		t.Run(operation, func(t *testing.T) {
			f := setup(t, 32)
			a := f.reserve(t, "busy")
			release, err := f.c.Guard(ctx, a.Identity)
			require.NoError(t, err)
			defer release()
			operationCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "reserve":
					_, err = f.c.Reserve(operationCtx, request(1, a.ConversationID))
				case "release":
					err = f.c.Release(operationCtx, a.Identity, agentcontrol.Closed)
				case "reconcile":
					_, err = f.c.InterruptAll(operationCtx)
				}
				done <- err
			}()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(time.Second):
				t.Fatal("canceled waiter still blocked behind the guard")
			}
			// Canceling a queued takeover must leave the original attempt valid.
			unlock, err := f.c.Guard(ctx, a.Identity)
			require.NoError(t, err)
			unlock()
		})
	}
}

func TestCreateGuardWaitersHonorCancellationAcrossSameAndReissuedGrant(t *testing.T) {
	f := setup(t, 32)
	r := request(1, "create-cancel")
	r.ArtifactID = ""
	a, err := f.c.Reserve(ctx, r)
	require.NoError(t, err)
	first, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	second, err := f.c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	release, err := first.Guard(ctx)
	require.NoError(t, err)
	defer release()
	for _, g := range []*agentscope.Grant{first, second} {
		operationCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		done := make(chan error, 1)
		go func() {
			unlock, err := g.Guard(operationCtx)
			if err == nil {
				unlock()
			}
			done <- err
		}()
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(time.Second):
			t.Fatal("canceled create waiter still blocked")
		}
		cancel()
	}
	require.NoError(t, first.BindArtifactContext(ctx, "artifact"))
}

func TestReconciliationWaitDoesNotBlockOtherConversationRenewal(t *testing.T) {
	f := setup(t, 32)
	a := f.reserve(t, "a-busy")
	b := f.reserve(t, "b-healthy")
	release, err := f.c.Guard(ctx, a.Identity)
	require.NoError(t, err)
	defer release()
	operationCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	reconciled := make(chan error, 1)
	go func() { _, err := f.c.InterruptAll(operationCtx); reconciled <- err }()
	renewCtx, cancelRenew := context.WithTimeout(ctx, time.Second)
	defer cancelRenew()
	renewed := make(chan error, 1)
	go func() { renewed <- f.c.Renew(renewCtx, b.Identity) }()
	select {
	case err := <-renewed:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("reconciliation of A blocked B's renewal")
	}
	cancel()
	select {
	case err := <-reconciled:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not cancel")
	}
}

func TestReconciliationReturnsCompletedCleanupWhenCanceled(t *testing.T) {
	f := setup(t, 32)
	first := f.reserve(t, "a-reap")
	second := f.reserve(t, "z-busy")
	release, err := f.c.Guard(ctx, second.Identity)
	require.NoError(t, err)
	defer release()
	operationCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		assignments []agentcontrol.Assignment
		err         error
	}
	done := make(chan result, 1)
	go func() { assignments, err := f.c.InterruptAll(operationCtx); done <- result{assignments, err} }()
	require.Eventually(t, func() bool {
		candidates, err := f.s.ListAgentExecutionsToReconcile(ctx, true, time.UnixMilli(f.clock.Load()))
		return err == nil && len(candidates) == 1 && candidates[0].ExecutionID == second.ExecutionID
	}, 2*time.Second, time.Millisecond, "the first conversation should reconcile before waiting for the second")
	cancel()
	select {
	case got := <-done:
		require.ErrorIs(t, got.err, context.Canceled)
		require.Len(t, got.assignments, 1)
		assert.Equal(t, first.ExecutionID, got.assignments[0].ExecutionID)
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not cancel")
	}
	unlock, err := f.c.Guard(ctx, second.Identity)
	require.NoError(t, err)
	unlock()
}
