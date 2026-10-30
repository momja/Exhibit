package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putGrantArtifact(t *testing.T, s *SQLiteStore, ownerID int64, id string) {
	t.Helper()
	require.NoError(t, s.PutArtifact(context.Background(), &Artifact{
		ID: id, OwnerID: ownerID, Title: id, SourceBlobID: id + "-body", Tier: Tier1,
	}))
}

func TestAgentGrantResolvesToItsOwnerAndArtifact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putGrantArtifact(t, s, 1, "art-1")

	g, err := s.CreateAgentGrant(ctx, 1, "art-1", "Claude Code", "digest-1")
	require.NoError(t, err)

	got, err := s.ResolveAgentGrant(ctx, "digest-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, g.ID, got.ID)
	assert.Equal(t, int64(1), got.OwnerID)
	assert.Equal(t, "art-1", got.ArtifactID)
	assert.Equal(t, "Claude Code", got.Label)
	require.NotNil(t, got.LastUsedAt, "a resolve is a use")

	missing, err := s.ResolveAgentGrant(ctx, "digest-unknown")
	require.NoError(t, err)
	assert.Nil(t, missing)
	empty, err := s.ResolveAgentGrant(ctx, "")
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestAgentGrantIsOwnerScoped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putGrantArtifact(t, s, 1, "mine")
	putGrantArtifact(t, s, 2, "theirs")

	_, err := s.CreateAgentGrant(ctx, 1, "theirs", "x", "digest-x")
	assert.ErrorIs(t, err, ErrNotFound, "another owner's artifact is a missing one")

	theirs, err := s.CreateAgentGrant(ctx, 2, "theirs", "theirs", "digest-theirs")
	require.NoError(t, err)

	list, err := s.ListAgentGrants(ctx, 1, "theirs")
	require.NoError(t, err)
	assert.Empty(t, list)
	assert.ErrorIs(t, s.DeleteAgentGrant(ctx, 1, "theirs", theirs.ID), ErrNotFound)

	// A grant id is addressed under its artifact too.
	mine, err := s.CreateAgentGrant(ctx, 1, "mine", "mine", "digest-mine")
	require.NoError(t, err)
	assert.ErrorIs(t, s.DeleteAgentGrant(ctx, 2, "theirs", mine.ID), ErrNotFound)

	list, err = s.ListAgentGrants(ctx, 2, "theirs")
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestAgentGrantRevokeAndCascade(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putGrantArtifact(t, s, 1, "art-1")

	first, err := s.CreateAgentGrant(ctx, 1, "art-1", "first", "digest-first")
	require.NoError(t, err)
	_, err = s.CreateAgentGrant(ctx, 1, "art-1", "second", "digest-second")
	require.NoError(t, err)

	list, err := s.ListAgentGrants(ctx, 1, "art-1")
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "first", list[0].Label, "oldest first")

	require.NoError(t, s.DeleteAgentGrant(ctx, 1, "art-1", first.ID))
	assert.ErrorIs(t, s.DeleteAgentGrant(ctx, 1, "art-1", first.ID), ErrNotFound)
	got, err := s.ResolveAgentGrant(ctx, "digest-first")
	require.NoError(t, err)
	assert.Nil(t, got, "a revoked grant resolves to nothing")

	_, err = s.DeleteArtifact(ctx, 1, "art-1")
	require.NoError(t, err)
	got, err = s.ResolveAgentGrant(ctx, "digest-second")
	require.NoError(t, err)
	assert.Nil(t, got, "a grant goes with its artifact")
}

func TestAgentGrantRefusedWhileItsOwnerIsDisabled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	admin, err := s.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)
	_ = admin
	putGrantArtifact(t, s, member.ID, "art-m")
	_, err = s.CreateAgentGrant(ctx, member.ID, "art-m", "agent", "digest-m")
	require.NoError(t, err)

	require.NoError(t, s.SetUserDisabled(ctx, member.ID, true))
	got, err := s.ResolveAgentGrant(ctx, "digest-m")
	require.NoError(t, err)
	assert.Nil(t, got)

	require.NoError(t, s.SetUserDisabled(ctx, member.ID, false))
	got, err = s.ResolveAgentGrant(ctx, "digest-m")
	require.NoError(t, err)
	assert.NotNil(t, got, "re-enabling the account restores the grant it kept")
}

func TestAgentGrantUseIsRecordedCoarsely(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putGrantArtifact(t, s, 1, "art-1")
	g, err := s.CreateAgentGrant(ctx, 1, "art-1", "agent", "digest-1")
	require.NoError(t, err)

	old := time.Now().UTC().Add(-time.Hour)
	_, err = s.db.ExecContext(ctx, "UPDATE agent_grants SET last_used_at = ? WHERE id = ?",
		old.Format(agentGrantTimeLayout), g.ID)
	require.NoError(t, err)

	got, err := s.ResolveAgentGrant(ctx, "digest-1")
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.WithinDuration(t, time.Now(), *got.LastUsedAt, 5*time.Second)

	var stored string
	require.NoError(t, s.db.QueryRowContext(ctx,
		"SELECT last_used_at FROM agent_grants WHERE id = ?", g.ID).Scan(&stored))
	first := stored

	// A second use within the resolution writes nothing.
	_, err = s.ResolveAgentGrant(ctx, "digest-1")
	require.NoError(t, err)
	require.NoError(t, s.db.QueryRowContext(ctx,
		"SELECT last_used_at FROM agent_grants WHERE id = ?", g.ID).Scan(&stored))
	assert.Equal(t, first, stored)
}
