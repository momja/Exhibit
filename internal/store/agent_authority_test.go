package store

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentAuthorityUpgradeKeepsLegacyConversationsAndUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, goose.DownToContext(ctx, s.db, "migrations", 33))
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "artifact", OwnerID: 1, Title: "t", SourceBlobID: "b", Tier: Tier1}))
	require.NoError(t, s.SaveTranscript(ctx, 1, Transcript{ArtifactID: "artifact", SessionID: "conversation", SessionFile: "{\"type\":\"session\"}\n", Title: "retained title"}))
	_, err = s.db.Exec(`INSERT INTO agent_transcripts(artifact_id,session_id,messages) VALUES('artifact','legacy','[]')`)
	require.NoError(t, err)
	require.NoError(t, s.RecordAgentUsage(ctx, AgentUsage{OwnerID: 1, SessionID: "conversation", Provider: "p", PaidBy: PaidByPlatform, Source: UsageSourceModel, InputTokens: 9, CostMicros: 50}))
	require.NoError(t, s.Close())
	s, err = OpenSQLite(path)
	require.NoError(t, err)
	defer s.Close()
	c := agentcontrol.New(s, agentscope.NewRegistry(), agentcontrol.Config{})
	a, err := c.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ArtifactID: "artifact", ConversationID: "conversation", ExecutionID: "new", WorkerID: "w", Resume: true})
	require.NoError(t, err)
	p, err := c.ReadSnapshot(ctx, a.Identity)
	require.NoError(t, err)
	assert.Equal(t, "retained title", p.Title)
	_, err = c.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ArtifactID: "artifact", ConversationID: "legacy", ExecutionID: "legacy-new", WorkerID: "w", Resume: true})
	require.ErrorIs(t, err, agentcontrol.ErrNotResumable)
	spend, err := s.SessionAgentSpend(ctx, 1, "conversation")
	require.NoError(t, err)
	assert.Equal(t, int64(50), spend.CostMicros)
}

func TestAgentSupervisorPersistsOnlyHashAndLateUsageReceiptsAreAtomic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := agentcontrol.New(s, agentscope.NewRegistry(), agentcontrol.Config{})
	a, err := c.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ConversationID: "conversation", ExecutionID: "execution", WorkerID: "worker"})
	require.NoError(t, err)
	token, err := c.IssueSupervisor(ctx, a.Identity, "platform")
	require.NoError(t, err)
	var persisted []byte
	require.NoError(t, s.db.QueryRow(`SELECT token_hash FROM agent_supervisor_credentials`).Scan(&persisted))
	want := sha256.Sum256([]byte(token))
	assert.Equal(t, want[:], persisted)
	assert.NotContains(t, string(persisted), token)
	supervisor, err := c.AuthenticateSupervisor(ctx, token)
	require.NoError(t, err)
	// Force a failure after the receipt INSERT but before the ledger INSERT.
	// Retrying must still record the usage exactly once, rather than lose it.
	_, err = s.db.Exec(`CREATE TRIGGER fail_agent_usage BEFORE INSERT ON agent_usage BEGIN SELECT RAISE(ABORT,'test unavailable'); END`)
	require.NoError(t, err)
	u := agentcontrol.Usage{EventID: "event", Source: "model", InputTokens: 3, CostMicros: 7}
	_, err = c.ReportUsage(ctx, supervisor, u)
	require.Error(t, err)
	var count int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM agent_usage_receipts`).Scan(&count))
	assert.Zero(t, count)
	_, err = s.db.Exec(`DROP TRIGGER fail_agent_usage`)
	require.NoError(t, err)
	inserted, err := c.ReportUsage(ctx, supervisor, u)
	require.NoError(t, err)
	assert.True(t, inserted)
	_, err = c.ReportUsage(ctx, supervisor, u)
	require.NoError(t, err)
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM agent_usage`).Scan(&count))
	assert.Equal(t, 1, count)
	require.NoError(t, c.Release(ctx, a.Identity, agentcontrol.Failed))
	// Expired leases revoke write authority without destroying attribution.
	_, err = s.GetAgentExecution(ctx, a.Identity, time.Now().Add(time.Hour))
	require.ErrorIs(t, err, agentcontrol.ErrStale)
}

func TestAgentSnapshotReadBoundsImportedFileBeforeDriverAllocation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "artifact", OwnerID: 1, SourceBlobID: "body", Tier: Tier1}))
	c := agentcontrol.New(s, agentscope.NewRegistry(), agentcontrol.Config{})
	a, err := c.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ArtifactID: "artifact", ConversationID: "conversation", ExecutionID: "execution", WorkerID: "worker"})
	require.NoError(t, err)
	require.NoError(t, s.SaveTranscript(ctx, 1, Transcript{ArtifactID: "artifact", SessionID: "conversation", SessionFile: "{}\n"}))
	_, err = s.db.Exec(`UPDATE agent_transcripts SET session_file=CAST(zeroblob(?) AS TEXT) WHERE session_id='conversation'`, agentcontrol.MaxSnapshotBytes+1)
	require.NoError(t, err)
	_, err = c.ReadSnapshot(ctx, a.Identity)
	require.ErrorIs(t, err, agentcontrol.ErrTooLarge)
}

func TestAgentCreateAndBindingCommitTogetherAndIgnoreUntrustedOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := agentcontrol.New(s, agentscope.NewRegistry(), agentcontrol.Config{})
	a, err := c.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ConversationID: "create", ExecutionID: "attempt", WorkerID: "w"})
	require.NoError(t, err)
	g, err := c.IssueTool(ctx, a.Identity)
	require.NoError(t, err)
	release, err := g.Guard(ctx)
	require.NoError(t, err)
	defer release()
	artifact := &Artifact{ID: "created", OwnerID: 99, SourceBlobID: "body", Tier: Tier1, NetworkAllowlist: []string{"https://example.test"}}
	// A failed binding must roll back the artifact, initial version and origins.
	_, err = s.db.Exec(`CREATE TRIGGER fail_agent_bind BEFORE UPDATE OF artifact_id ON agent_conversations BEGIN SELECT RAISE(ABORT,'test unavailable'); END`)
	require.NoError(t, err)
	require.Error(t, s.PutAgentArtifact(ctx, a.Identity, artifact, time.Now()))
	got, err := s.GetArtifact(ctx, 1, "created")
	require.NoError(t, err)
	assert.Nil(t, got)
	_, err = s.db.Exec(`DROP TRIGGER fail_agent_bind`)
	require.NoError(t, err)
	require.NoError(t, s.PutAgentArtifact(ctx, a.Identity, artifact, time.Now()))
	// Publish the binding to this in-memory grant only after the atomic commit.
	require.NoError(t, g.BindArtifactContext(ctx, "created"))
	got, err = s.GetArtifact(ctx, 1, "created")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(1), got.OwnerID)
	assert.Equal(t, []string{"https://example.test"}, got.NetworkAllowlist)
	version, err := s.GetVersion(ctx, 1, "created", 1)
	require.NoError(t, err)
	assert.Equal(t, "create", version.SessionID)
	assert.Equal(t, VersionAgent, version.Origin)
	second := *artifact
	second.ID = "second-create"
	require.ErrorIs(t, s.PutAgentArtifact(ctx, a.Identity, &second, time.Now()), agentcontrol.ErrConflict)
	got, err = s.GetArtifact(ctx, 1, "second-create")
	require.NoError(t, err)
	assert.Nil(t, got)
}
