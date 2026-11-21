package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recipient's consent is theirs alone, needs a readable artifact, and is
// listed beside the owner's own approvals; the ledger reports inference apart
// from agent spend while the owner total still counts both (Exh-hwjg).
func TestInferenceConsentsAndLedger(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	owner, err := s.UpsertUser(ctx, "local:owner", "")
	require.NoError(t, err)
	friend, err := s.UpsertUser(ctx, "local:friend", "")
	require.NoError(t, err)
	stranger, err := s.UpsertUser(ctx, "local:stranger", "")
	require.NoError(t, err)
	for _, a := range []*Artifact{
		{ID: "mine", OwnerID: owner.ID, Title: "Triage", SourceBlobID: "b1", Tier: Tier1},
		{ID: "theirs", OwnerID: owner.ID, Title: "Shared board", SourceBlobID: "b2", Tier: Tier1},
	} {
		require.NoError(t, s.PutArtifact(ctx, a))
	}
	require.NoError(t, s.UpdateArtifact(ctx, owner.ID, "mine", map[string]any{"inference_approved": true}))
	a, err := s.GetArtifact(ctx, owner.ID, "mine")
	require.NoError(t, err)
	assert.True(t, a.InferenceApproved)

	// No grant: a stranger cannot leave a consent row.
	assert.ErrorIs(t, s.SetInferenceConsent(ctx, ViewerID(stranger.ID), "theirs", true), ErrNotFound)
	_, err = s.CreateGrant(ctx, owner.ID, "theirs", friend.ID)
	require.NoError(t, err)
	require.NoError(t, s.SetInferenceConsent(ctx, ViewerID(friend.ID), "theirs", true))
	require.NoError(t, s.SetInferenceConsent(ctx, ViewerID(friend.ID), "theirs", true), "idempotent")
	ok, err := s.HasInferenceConsent(ctx, ViewerID(friend.ID), "theirs")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.HasInferenceConsent(ctx, ViewerID(owner.ID), "theirs")
	require.NoError(t, err)
	assert.False(t, ok, "consent is per viewer")

	perms, err := s.ListInferencePermissions(ctx, ViewerID(owner.ID))
	require.NoError(t, err)
	assert.Equal(t, []InferencePermission{{ArtifactID: "mine", Title: "Triage", Owned: true}}, perms)
	perms, err = s.ListInferencePermissions(ctx, ViewerID(friend.ID))
	require.NoError(t, err)
	assert.Equal(t, []InferencePermission{{ArtifactID: "theirs", Title: "Shared board"}}, perms)

	// Revoking the grant hides the consent without needing to delete it.
	require.NoError(t, s.DeleteGrant(ctx, owner.ID, "theirs", friend.ID))
	perms, err = s.ListInferencePermissions(ctx, ViewerID(friend.ID))
	require.NoError(t, err)
	assert.Empty(t, perms)

	since := time.Now().Add(-time.Hour)
	require.NoError(t, s.RecordAgentUsage(ctx, AgentUsage{OwnerID: owner.ID, SessionID: "agent", Provider: "p", PaidBy: PaidByPlatform, Source: UsageSourceModel, CostMicros: 100}))
	for _, kind := range []string{"complete", "complete", "decide"} {
		require.NoError(t, s.RecordAgentUsage(ctx, AgentUsage{OwnerID: owner.ID, SessionID: "inf", Provider: "p", PaidBy: PaidByPlatform,
			Source: UsageSourceInference, ArtifactID: "mine", Kind: kind, InputTokens: 10, CostMicros: 7}))
	}
	// A recipient's call is the recipient's row, not the owner's.
	require.NoError(t, s.RecordAgentUsage(ctx, AgentUsage{OwnerID: friend.ID, SessionID: "inf", Provider: "p", PaidBy: PaidByPlatform,
		Source: UsageSourceInference, ArtifactID: "theirs", Kind: "decide", CostMicros: 5, CostSource: CostSourceNone}))

	total, err := s.OwnerAgentSpend(ctx, owner.ID, since)
	require.NoError(t, err)
	assert.Equal(t, int64(121), total.CostMicros, "one shared total")
	inf, err := s.OwnerInferenceSpend(ctx, owner.ID, since)
	require.NoError(t, err)
	assert.Equal(t, int64(21), inf.CostMicros)

	usage, err := s.InferenceUsageByArtifact(ctx, owner.ID, since)
	require.NoError(t, err)
	require.Len(t, usage, 2)
	assert.Equal(t, "complete", usage[0].Kind)
	assert.Equal(t, int64(2), usage[0].Calls)
	assert.Equal(t, "Triage", usage[0].Title)
	assert.Equal(t, "decide", usage[1].Kind)

	// The friend lost the grant, so the title is withheld but the spend stays.
	usage, err = s.InferenceUsageByArtifact(ctx, friend.ID, since)
	require.NoError(t, err)
	require.Len(t, usage, 1)
	assert.Equal(t, "", usage[0].Title)
	assert.Equal(t, int64(5), usage[0].CostMicros)
}
