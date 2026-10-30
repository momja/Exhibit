package store

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalConnectionUpgradeKeepsSavedResultsAndOwnerAccess(t *testing.T) {
	registerOriginNormalizationMigration()
	path := newMigratedTo(t, 49)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	legacy := &SQLiteStore{db: db}
	ctx := t.Context()
	require.NoError(t, legacy.PutArtifact(ctx, &Artifact{ID: "shared-tool", OwnerID: 1, Title: "Connected tool", SourceBlobID: "body", Tier: Tier1}))
	require.NoError(t, legacy.CreateConnection(ctx, &ConnectionRecord{ID: "personal", OwnerID: 1, Name: "Account", Provider: "bearer", Kind: "native", RecipeJSON: `{"origins":["https://api.example.test"]}`, EncryptedSecret: "sealed"}, -1))
	_, err = db.ExecContext(ctx, `INSERT INTO artifact_connection_grants(artifact_id,connection_id,viewer_id,share_with_recipients) VALUES ('shared-tool','personal',1,1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO artifact_connection_data_policies(artifact_id,connection_id,viewer_id,share_with_recipients) VALUES ('shared-tool','personal',1,1),('shared-tool','personal',2,0)`)
	require.NoError(t, err)
	require.NoError(t, legacy.SetState(ctx, 1, "shared-tool", 1, "result", "owner result"))
	require.NoError(t, legacy.SetState(ctx, 1, "shared-tool", 2, "result", "recipient's previously saved copy"))
	require.NoError(t, db.Close())
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	owner, err := s.GetGrantedConnection(ctx, 1, "shared-tool", "personal")
	require.NoError(t, err)
	assert.Equal(t, "personal", owner.ID)
	_, err = s.GetGrantedConnection(ctx, 2, "shared-tool", "personal")
	assert.ErrorIs(t, err, ErrNotFound)
	grants, err := s.ListConnectionGrants(ctx, 2, "shared-tool")
	require.NoError(t, err)
	assert.Empty(t, grants)
	var shared int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT SUM(share_with_recipients) FROM artifact_connection_grants`).Scan(&shared))
	assert.Zero(t, shared)
	for viewer, want := range map[ViewerID]string{1: "owner result", 2: "recipient's previously saved copy"} {
		state, err := s.GetState(ctx, 1, "shared-tool", viewer)
		require.NoError(t, err)
		assert.Equal(t, want, state["result"])
		policies, err := s.ListConnectionDataPolicies(ctx, int64(viewer), "shared-tool")
		require.NoError(t, err)
		require.Len(t, policies, 1)
	}
}

func TestPersonalConnectionUpgradeRetainsSharedStateOwnerProvenance(t *testing.T) {
	registerOriginNormalizationMigration()
	path := newMigratedTo(t, 49)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	legacy := &SQLiteStore{db: db}
	ctx := t.Context()
	require.NoError(t, legacy.PutArtifact(ctx, &Artifact{ID: "shared-data", OwnerID: 1, Title: "Shared results", SourceBlobID: "body", Tier: Tier1}))
	_, err = db.ExecContext(ctx, `UPDATE artifacts SET share_state_mode='shared' WHERE id='shared-data'`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO artifact_connection_data_policies(artifact_id,connection_id,viewer_id) VALUES ('shared-data','former-recipient-account',2)`)
	require.NoError(t, err)
	require.NoError(t, legacy.SetState(ctx, 1, "shared-data", 1, "result", "recipient's connected result"))
	require.NoError(t, db.Close())
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	// Prove the migration itself retained custody, before new mutation paths
	// can repair a pre-existing shared-state row.
	_, err = s.db.ExecContext(ctx, `UPDATE artifacts SET share_state_mode='own' WHERE id='shared-data'`)
	require.NoError(t, err)
	policies, err := s.ListConnectionDataPolicies(ctx, 1, "shared-data")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, int64(1), policies[0].ViewerID)
	state, err := s.GetState(ctx, 1, "shared-data", 1)
	require.NoError(t, err)
	assert.Equal(t, "recipient's connected result", state["result"])
}
