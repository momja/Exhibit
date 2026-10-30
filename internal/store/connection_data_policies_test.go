package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putPolicyConnection(t *testing.T, s *SQLiteStore, ownerID int64, id, providerOrigin string) {
	t.Helper()
	require.NoError(t, s.CreateConnection(context.Background(), &ConnectionRecord{ID: id, OwnerID: ownerID, Name: "Private API", Provider: "bearer", Kind: "native", RecipeJSON: `{"origins":["` + providerOrigin + `"]}`, EncryptedSecret: "sealed"}, -1))
}

func TestConnectionDataPoliciesSurviveUngrantAndRevoke(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "retained", OwnerID: alice, Title: "Private data", Tier: Tier1, SourceBlobID: "body"}))
	putPolicyConnection(t, s, alice, "private", "https://bank.example.test")
	g := ConnectionGrantRecord{ArtifactID: "retained", ConnectionID: "private", ViewerID: alice, PermittedEgress: []string{"https://allowed.example.test", "https://bank.example.test"}}
	require.NoError(t, s.PutConnectionGrant(ctx, g))
	policies, err := s.ListConnectionDataPolicies(ctx, alice, "retained")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, []string{"https://allowed.example.test", "https://bank.example.test"}, policies[0].Origins)
	assert.False(t, policies[0].ShareWithRecipients)
	require.NoError(t, s.DeleteConnectionGrant(ctx, alice, "retained", "private"))
	retained, err := s.ListConnectionDataPolicies(ctx, alice, "retained")
	require.NoError(t, err)
	assert.Equal(t, policies, retained)
	require.NoError(t, s.DeleteConnection(ctx, alice, "private"))
	retained, err = s.ListConnectionDataPolicies(ctx, alice, "retained")
	require.NoError(t, err)
	assert.Equal(t, policies, retained)
	_, shareErr := s.CreateAnonymousLink(ctx, alice, "retained")
	require.NoError(t, shareErr)
	require.NoError(t, s.UpdateArtifact(ctx, alice, "retained", map[string]any{"share_state_mode": ShareStateShared}))
	other, err := s.ListConnectionDataPolicies(ctx, bob, "retained")
	require.NoError(t, err)
	assert.Len(t, other, 1, "shared saved results retain their provenance without sharing the account")
}

func TestConnectionDataPoliciesAndGrantsCommitTogether(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "atomic-policy", OwnerID: alice, Title: "Private data", Tier: Tier1, SourceBlobID: "body"}))
	putPolicyConnection(t, s, alice, "private", "https://bank.example.test")
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER refuse_data_policy BEFORE INSERT ON artifact_connection_data_policies BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	require.NoError(t, err)
	assert.Error(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "atomic-policy", ConnectionID: "private", ViewerID: alice}))
	grants, err := s.ListConnectionGrants(ctx, alice, "atomic-policy")
	require.NoError(t, err)
	assert.Empty(t, grants, "a grant cannot become usable if its durable data policy failed")
}

func TestClearConnectionDataClearsStateAndHistoryButRetainsActivePolicies(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const id = "clear-private-data"
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: alice, Title: "Private data", Tier: Tier1, SourceBlobID: "body"}))
	for _, name := range []string{"active", "removed"} {
		putPolicyConnection(t, s, alice, name, "https://"+name+".example.test")
		require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: id, ConnectionID: name, ViewerID: alice}))
	}
	require.NoError(t, s.DeleteConnection(ctx, alice, "removed"))
	require.NoError(t, s.SetStateAsViewer(ctx, ViewerID(alice), id, "balance", "private financial data"))
	_, err := s.CommitVersion(ctx, alice, id, VersionChange{BodyBlobID: strPtr("new-body")})
	require.NoError(t, err)
	require.NoError(t, s.ClearConnectionData(ctx, alice, id))
	state, err := s.GetStateAsViewer(ctx, ViewerID(alice), id)
	require.NoError(t, err)
	assert.Empty(t, state)
	policies, err := s.ListConnectionDataPolicies(ctx, alice, id)
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "active", policies[0].ConnectionID)
	_, err = s.RestoreVersion(ctx, alice, id, 1, Provenance{}, "")
	require.NoError(t, err)
	state, err = s.GetStateAsViewer(ctx, ViewerID(alice), id)
	require.NoError(t, err)
	assert.Empty(t, state, "restoring old code cannot recover cleared financial state")
	require.NoError(t, s.DeleteConnectionGrant(ctx, alice, id, "active"))
	require.NoError(t, s.ClearConnectionData(ctx, alice, id))
	policies, err = s.ListConnectionDataPolicies(ctx, alice, id)
	require.NoError(t, err)
	assert.Empty(t, policies)
	_, shareErr := s.CreateAnonymousLink(ctx, alice, id)
	require.NoError(t, shareErr)
}

func TestClearConnectionDataIsAtomicOnPolicyFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const id = "atomic-clear"
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: alice, Title: "Private data", Tier: Tier1, SourceBlobID: "body"}))
	putPolicyConnection(t, s, alice, "private", "https://bank.example.test")
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: id, ConnectionID: "private", ViewerID: alice}))
	require.NoError(t, s.DeleteConnection(ctx, alice, "private"))
	require.NoError(t, s.SetStateAsViewer(ctx, ViewerID(alice), id, "balance", "financial data"))
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER refuse_policy_clear BEFORE DELETE ON artifact_connection_data_policies BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	require.NoError(t, err)
	assert.Error(t, s.ClearConnectionData(ctx, alice, id))
	state, err := s.GetStateAsViewer(ctx, ViewerID(alice), id)
	require.NoError(t, err)
	assert.Equal(t, "financial data", state["balance"], "state and restrictions must be cleared together")
	policies, err := s.ListConnectionDataPolicies(ctx, alice, id)
	require.NoError(t, err)
	assert.Len(t, policies, 1)
}

func TestClearConnectionDataRespectsViewerStateAndOwnerPolicies(t *testing.T) {
	f := newSharedFixture(t, ShareStateOwn)
	ctx := context.Background()
	putPolicyConnection(t, f.s, alice, "owner-policy", "https://owner.example.test")
	putPolicyConnection(t, f.s, bob, "viewer-policy", "https://viewer.example.test")
	require.NoError(t, f.s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: f.artifactID, ConnectionID: "owner-policy", ViewerID: alice}))
	require.NoError(t, f.s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: f.artifactID, ConnectionID: "viewer-policy", ViewerID: bob}))
	require.NoError(t, f.s.DeleteConnection(ctx, bob, "viewer-policy"))
	require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "private", "recipient financial data"))
	assert.ErrorIs(t, f.s.ClearConnectionData(ctx, 99999, f.artifactID), ErrNotFound)
	require.NoError(t, f.s.ClearConnectionData(ctx, bob, f.artifactID))
	state, err := f.s.GetStateAsViewer(ctx, ViewerID(bob), f.artifactID)
	require.NoError(t, err)
	assert.Empty(t, state)
	ownerState, err := f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, "value", ownerState["seed"])
	policies, err := f.s.ListConnectionDataPolicies(ctx, bob, f.artifactID)
	require.NoError(t, err)
	assert.Empty(t, policies, "own state does not inherit the owner's provenance")
	require.NoError(t, f.s.UpdateArtifact(ctx, alice, f.artifactID, map[string]any{"share_state_mode": ShareStateShared}))
	assert.ErrorIs(t, f.s.ClearConnectionData(ctx, bob, f.artifactID), ErrNotFound)
	ownerState, err = f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
	require.NoError(t, err)
	assert.Equal(t, "value", ownerState["seed"], "recipient reset cannot erase everybody's board")
}

func TestConnectionDataPolicyMigrationBackfillsExistingGrants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "old-connected", OwnerID: alice, Title: "Existing app", Tier: Tier1, SourceBlobID: "body"}))
	putPolicyConnection(t, s, alice, "old-connection", "https://api.example.test")
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "old-connected", ConnectionID: "old-connection", ViewerID: alice, PermittedEgress: []string{"https://allowed.example.test"}}))
	// A former recipient may still hold a saved copy even if their share is
	// currently revoked. Regranting access must retain that copy's policy.
	require.NoError(t, s.SetState(ctx, OwnerID(alice), "old-connected", ViewerID(bob), "balance", "existing financial data"))
	// Rewind newer migrations as well, so this remains a version-45 fixture
	// when additional connector migrations are introduced.
	require.NoError(t, goose.DownToContext(ctx, s.db, "migrations", 45))
	// Model the historical delegation before migration 046 existed.
	_, err = s.db.ExecContext(ctx, `UPDATE artifact_connection_grants SET share_with_recipients=1`)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	upgraded, err := OpenSQLite(path)
	require.NoError(t, err)
	defer upgraded.Close()
	policies, err := upgraded.ListConnectionDataPolicies(ctx, bob, "old-connected")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	for _, p := range policies {
		assert.Equal(t, bob, p.ViewerID)
		assert.False(t, p.ShareWithRecipients)
		assert.Equal(t, []string{"https://allowed.example.test", "https://api.example.test"}, p.Origins)
	}
	require.NoError(t, upgraded.DeleteConnection(ctx, alice, "old-connection"))
	policies, err = upgraded.ListConnectionDataPolicies(ctx, bob, "old-connected")
	require.NoError(t, err)
	assert.Len(t, policies, 1)
}

func TestConnectionDataPoliciesOnForeignArtifactsAreErasedWithAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	admin, err := s.UpsertUser(ctx, "policy-admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "policy-member", "member@example.test")
	require.NoError(t, err)
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "foreign-policy", OwnerID: admin.ID, Title: "Foreign app", Tier: Tier1, SourceBlobID: "body"}))
	putPolicyConnection(t, s, member.ID, "member-private", "https://bank.example.test")
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "foreign-policy", ConnectionID: "member-private", ViewerID: member.ID}))
	require.NoError(t, s.DeleteConnection(ctx, member.ID, "member-private"))
	policies, err := s.ListConnectionDataPolicies(ctx, member.ID, "foreign-policy")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	_, err = s.DeleteAccount(ctx, member.ID)
	require.NoError(t, err)
	policies, err = s.ListConnectionDataPolicies(ctx, member.ID, "foreign-policy")
	require.NoError(t, err)
	assert.Empty(t, policies, "foreign artifact cascade cannot erase this account's retained data policy")
	_, err = s.GetArtifact(ctx, admin.ID, "foreign-policy")
	require.NoError(t, err)
}

func TestSharedSavedDataPolicyCopiesOnlyNarrowAndRequireRecipient(t *testing.T) {
	f := newSharedFixture(t, ShareStateShared)
	ctx := context.Background()
	putPolicyConnection(t, f.s, alice, "shared-policy", "https://api.example.test")
	g := ConnectionGrantRecord{ArtifactID: f.artifactID, ViewerID: alice, ConnectionID: "shared-policy", PermittedEgress: []string{"https://old.example.test"}}
	require.NoError(t, f.s.PutConnectionGrant(ctx, g))
	require.NoError(t, f.s.RetainSharedConnectionDataPolicies(ctx, bob, f.artifactID))
	g.PermittedEgress = []string{"https://new.example.test"}
	require.NoError(t, f.s.PutConnectionGrant(ctx, g))
	require.NoError(t, f.s.RetainSharedConnectionDataPolicies(ctx, bob, f.artifactID))
	policies, err := f.s.ListConnectionDataPolicies(ctx, bob, f.artifactID)
	require.NoError(t, err)
	require.Len(t, policies, 2)
	for _, p := range policies {
		if p.ViewerID == bob {
			assert.Equal(t, []string{"https://api.example.test"}, p.Origins, "the owner cannot widen consent for data already held by the recipient")
			assert.False(t, p.ShareWithRecipients)
		}
	}
	require.NoError(t, f.s.RetainSharedConnectionDataPolicies(ctx, 99999, f.artifactID))
	var retained int
	require.NoError(t, f.s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_connection_data_policies WHERE viewer_id=99999`).Scan(&retained))
	assert.Zero(t, retained)
}

func TestSharedStateRetainsRecipientProvenanceAfterModeAndAccessChanges(t *testing.T) {
	for _, initialMode := range []string{ShareStateOwn, ShareStateShared} {
		t.Run(initialMode, func(t *testing.T) {
			f := newSharedFixture(t, initialMode)
			ctx := t.Context()
			reader, err := f.s.UpsertUser(ctx, "third-reader", "reader@example.test")
			require.NoError(t, err)
			carol := reader.ID
			putPolicyConnection(t, f.s, bob, "recipient-account", "https://bank.example.test")
			require.NoError(t, f.s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: f.artifactID, ConnectionID: "recipient-account", ViewerID: bob}))
			require.NoError(t, f.s.UpdateArtifact(ctx, alice, f.artifactID, map[string]any{"share_state_mode": ShareStateShared}))
			require.NoError(t, f.s.SetStateAsViewer(ctx, ViewerID(bob), f.artifactID, "balance", "recipient's saved result"))
			_, err = f.s.CreateGrant(ctx, alice, f.artifactID, carol)
			require.NoError(t, err)
			state, err := f.s.GetStateAsViewer(ctx, ViewerID(carol), f.artifactID)
			require.NoError(t, err)
			require.Equal(t, "recipient's saved result", state["balance"])
			require.NoError(t, f.s.DeleteConnection(ctx, bob, "recipient-account"))
			require.NoError(t, f.s.DeleteGrant(ctx, alice, f.artifactID, bob))
			require.NoError(t, f.s.UpdateArtifact(ctx, alice, f.artifactID, map[string]any{"share_state_mode": ShareStateOwn}))
			state, err = f.s.GetStateAsViewer(ctx, ViewerID(alice), f.artifactID)
			require.NoError(t, err)
			assert.Equal(t, "recipient's saved result", state["balance"])
			for _, viewer := range []int64{alice, carol} {
				policies, err := f.s.ListConnectionDataPolicies(ctx, viewer, f.artifactID)
				require.NoError(t, err)
				require.Len(t, policies, 1, "the shared-state owner and readers retain custody")
				assert.Equal(t, viewer, policies[0].ViewerID)
				grants, err := f.s.ListConnectionGrants(ctx, viewer, f.artifactID)
				require.NoError(t, err)
				assert.Empty(t, grants, "saved-data custody confers no account access")
			}
		})
	}
}
