package store

import (
	"context"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reserveAccountAgent(t *testing.T, c *agentcontrol.Control, ownerID int64, conversation string) (agentcontrol.Assignment, *agentscope.Grant) {
	t.Helper()
	assignment, err := c.Reserve(context.Background(), agentcontrol.Reservation{OwnerID: ownerID, ConversationID: conversation, ExecutionID: "execution-" + conversation, WorkerID: "worker-" + conversation})
	require.NoError(t, err)
	grant, err := c.IssueTool(context.Background(), assignment.Identity)
	require.NoError(t, err)
	return assignment, grant
}

func requireAccountAgentStatus(t *testing.T, s *SQLiteStore, executionID, want string) {
	t.Helper()
	var status string
	require.NoError(t, s.db.QueryRow(`SELECT status FROM agent_executions WHERE execution_id=?`, executionID).Scan(&status))
	assert.Equal(t, want, status)
}

func TestDeleteAccountFencesAlreadyAuthorizedUnboundCreate(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()
	registry := agentscope.NewRegistry()
	control := agentcontrol.New(fx.s, registry, agentcontrol.Config{})

	// This first tool create commits before erasure. Both it and the preexisting
	// library must disappear, including the version and origin rows it created.
	before, beforeGrant := reserveAccountAgent(t, control, fx.member.ID, "committed-before-erasure")
	require.NoError(t, fx.s.PutAgentArtifact(ctx, before.Identity, &Artifact{ID: "agent-before-erasure", SourceBlobID: "agent-before-body", Tier: Tier1, NetworkAllowlist: []string{"https://example.test"}}, time.Now()))
	require.NoError(t, beforeGrant.BindArtifactContext(ctx, "agent-before-erasure"))
	require.NoError(t, control.Activate(ctx, before.Identity))

	late, lateGrant := reserveAccountAgent(t, control, fx.member.ID, "already-authorized-create")
	require.Same(t, lateGrant, registry.Resolve(lateGrant.Token()))
	release, err := lateGrant.Guard(ctx)
	require.NoError(t, err)
	defer release()
	require.Empty(t, lateGrant.Scope().ArtifactID)
	closing, _ := reserveAccountAgent(t, control, fx.member.ID, "closing-at-erasure")
	require.NoError(t, control.BeginClose(ctx, closing.Identity))
	failed, _ := reserveAccountAgent(t, control, fx.member.ID, "historical-failure")
	require.NoError(t, control.Release(ctx, failed.Identity, agentcontrol.Failed))

	// Another owner's existing data and outstanding create authority survive.
	require.NoError(t, fx.s.PutArtifact(ctx, &Artifact{ID: "other-owner-artifact", OwnerID: fx.admin.ID, SourceBlobID: "other-body", Tier: Tier1}))
	other, otherGrant := reserveAccountAgent(t, control, fx.admin.ID, "other-owner-create")

	// Assert the SQL ordering itself: erasure cannot delete an artifact until
	// every live attempt for its owner has been interrupted in that transaction.
	_, err = fx.s.db.Exec(`CREATE TRIGGER require_owner_execution_fence BEFORE DELETE ON artifacts
 WHEN EXISTS(SELECT 1 FROM agent_executions e JOIN agent_conversations c ON c.id=e.conversation_id
             WHERE c.owner_id=OLD.owner_id AND e.status IN ('reserved','active','closing'))
 BEGIN SELECT RAISE(ABORT,'owner still has live execution authority'); END`)
	require.NoError(t, err)
	queued, err := fx.s.DeleteAccount(ctx, fx.member.ID)
	require.NoError(t, err)
	assert.Contains(t, queued, "agent-before-body")

	// The already-authorized request resumes after DeleteAccount commits,
	// before any Manager erase/release. Its held guard cannot resurrect rows.
	err = fx.s.PutAgentArtifact(ctx, late.Identity, &Artifact{ID: "agent-after-erasure", SourceBlobID: "late-body", Tier: Tier1}, time.Now())
	require.ErrorIs(t, err, agentcontrol.ErrStale)
	assert.Nil(t, registry.Resolve(lateGrant.Token()))
	for _, assignment := range []agentcontrol.Assignment{before, late, closing} {
		requireAccountAgentStatus(t, fx.s, assignment.ExecutionID, agentcontrol.Interrupted)
	}
	requireAccountAgentStatus(t, fx.s, failed.ExecutionID, agentcontrol.Failed)
	assert.Zero(t, countRows(t, fx.s, `SELECT COUNT(*) FROM artifacts WHERE owner_id=?1`, fx.member.ID))
	assert.Zero(t, countRows(t, fx.s, `SELECT COUNT(*) FROM blob_references WHERE owner_id=?1`, fx.member.ID))
	for _, table := range []string{"artifact_versions", "artifact_network_origins"} {
		var count int
		require.NoError(t, fx.s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE artifact_id IN ('agent-before-erasure','agent-after-erasure')`).Scan(&count))
		assert.Zero(t, count)
	}
	deleted, err := fx.s.GetUser(ctx, fx.member.ID)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, deleted)

	survived, err := fx.s.GetArtifact(ctx, fx.admin.ID, "other-owner-artifact")
	require.NoError(t, err)
	require.NotNil(t, survived)
	requireAccountAgentStatus(t, fx.s, other.ExecutionID, agentcontrol.Reserved)
	require.Same(t, otherGrant, registry.Resolve(otherGrant.Token()))
	require.NoError(t, fx.s.PutAgentArtifact(ctx, other.Identity, &Artifact{ID: "other-owner-created-after", SourceBlobID: "other-after-body", Tier: Tier1}, time.Now()))
}

func TestDeleteAccountFailureRollsBackExecutionFence(t *testing.T) {
	for _, failure := range []string{"last enabled admin", "later SQL failure"} {
		t.Run(failure, func(t *testing.T) {
			fx := seedEverything(t, newTestStore(t))
			ctx := context.Background()
			registry := agentscope.NewRegistry()
			control := agentcontrol.New(fx.s, registry, agentcontrol.Config{})
			ownerID := fx.member.ID
			if failure == "last enabled admin" {
				ownerID = fx.admin.ID
			}
			require.NoError(t, fx.s.PutArtifact(ctx, &Artifact{ID: "rollback-existing", OwnerID: ownerID, SourceBlobID: "rollback-existing-body", Tier: Tier1}))
			assignment, grant := reserveAccountAgent(t, control, ownerID, "rollback-create")
			release, err := grant.Guard(ctx)
			require.NoError(t, err)
			defer release()
			if failure == "later SQL failure" {
				// Tags are deleted after users and artifacts. Failing here proves their
				// cascades and the earlier authority UPDATE roll back as one transaction.
				_, err = fx.s.db.Exec(`CREATE TRIGGER fail_account_erasure BEFORE DELETE ON tags
    WHEN OLD.id='member-tag' BEGIN SELECT RAISE(ABORT,'injected erasure failure'); END`)
				require.NoError(t, err)
			}
			_, err = fx.s.DeleteAccount(ctx, ownerID)
			if failure == "last enabled admin" {
				require.ErrorIs(t, err, ErrLastAdmin)
			} else {
				require.ErrorContains(t, err, "injected erasure failure")
			}
			requireAccountAgentStatus(t, fx.s, assignment.ExecutionID, agentcontrol.Reserved)
			owner, err := fx.s.GetUser(ctx, ownerID)
			require.NoError(t, err)
			require.NotNil(t, owner)
			existing, err := fx.s.GetArtifact(ctx, ownerID, "rollback-existing")
			require.NoError(t, err)
			require.NotNil(t, existing)
			memberData, err := fx.s.GetArtifact(ctx, fx.member.ID, deletedArtifact)
			require.NoError(t, err)
			require.NotNil(t, memberData)
			require.Same(t, grant, registry.Resolve(grant.Token()))
			// Authority still works after failed deletion, even for the very create
			// request that was authorized before the failed erasure attempt.
			require.NoError(t, fx.s.PutAgentArtifact(ctx, assignment.Identity, &Artifact{ID: "create-after-refused-erasure", SourceBlobID: "surviving-create-body", Tier: Tier1}, time.Now()))
			require.NoError(t, grant.BindArtifactContext(ctx, "create-after-refused-erasure"))
		})
	}
}
