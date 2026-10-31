package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentGrantResolvesToItsOwnerWithoutAUserOrArtifact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plaintext := "exgrant_a-random-secret"
	sum := sha256.Sum256([]byte(plaintext))
	digest := hex.EncodeToString(sum[:])
	g, err := s.CreateAgentGrant(ctx, 1, "Claude Code", "", digest)
	require.NoError(t, err)
	assert.Nil(t, g.LastUsedAt)

	got, err := s.ResolveAgentGrant(ctx, digest)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, g.ID, got.ID)
	assert.Equal(t, int64(1), got.OwnerID)
	assert.Equal(t, "Claude Code", got.Label)
	assert.Empty(t, got.Client)
	assert.Equal(t, g.CreatedAt, got.CreatedAt)
	require.NotNil(t, got.LastUsedAt)

	var stored string
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT token_hash FROM agent_grants WHERE id = ?", g.ID).Scan(&stored))
	assert.Equal(t, digest, stored)
	encoded, err := json.Marshal(g)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), digest)
	assert.NotContains(t, string(encoded), plaintext)

	for _, unknown := range []string{"", plaintext, "unknown-digest"} {
		missing, err := s.ResolveAgentGrant(ctx, unknown)
		require.NoError(t, err)
		assert.Nil(t, missing)
	}
	_, err = s.CreateAgentGrant(ctx, 2, "duplicate", "", digest)
	require.Error(t, err, "a digest can identify only one credential")
}

func TestAgentGrantRevokeAndAccountDeletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	admin, err := s.UpsertUser(ctx, "admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "member", "member@example.test")
	require.NoError(t, err)
	first, err := s.CreateAgentGrant(ctx, member.ID, "first", "Claude", "digest-first")
	require.NoError(t, err)
	_, err = s.CreateAgentGrant(ctx, member.ID, "second", "", "digest-second")
	require.NoError(t, err)
	_, err = s.CreateAgentGrant(ctx, admin.ID, "admin", "", "digest-admin")
	require.NoError(t, err)
	// Ensure ordering is asserted across distinct times rather than UUID ties.
	_, err = s.db.ExecContext(ctx, "UPDATE agent_grants SET created_at = ? WHERE id = ?", time.Now().Add(-time.Hour).UTC().Format(agentGrantTimeLayout), first.ID)
	require.NoError(t, err)
	list, err := s.ListAgentGrants(ctx, member.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "first", list[0].Label)
	assert.Equal(t, "Claude", list[0].Client)
	require.NoError(t, s.DeleteAgentGrant(ctx, member.ID, first.ID))
	assert.ErrorIs(t, s.DeleteAgentGrant(ctx, member.ID, first.ID), ErrNotFound)
	got, err := s.ResolveAgentGrant(ctx, "digest-first")
	require.NoError(t, err)
	assert.Nil(t, got)
	_, err = s.DeleteAccount(ctx, member.ID)
	require.NoError(t, err)
	got, err = s.ResolveAgentGrant(ctx, "digest-second")
	require.NoError(t, err)
	assert.Nil(t, got)
	got, err = s.ResolveAgentGrant(ctx, "digest-admin")
	require.NoError(t, err)
	assert.NotNil(t, got, "account deletion preserves other owners' grants")
}

func TestAgentGrantSurvivesDeletingTheOwnersLastArtifact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{
		ID: "last-artifact", OwnerID: 1, Title: "Last artifact", SourceBlobID: "body", Tier: Tier1,
	}))
	grant, err := s.CreateAgentGrant(ctx, 1, "Library agent", "", "digest")
	require.NoError(t, err)
	_, err = s.DeleteArtifact(ctx, 1, "last-artifact")
	require.NoError(t, err)
	got, err := s.ResolveAgentGrant(ctx, "digest")
	require.NoError(t, err)
	require.NotNil(t, got, "the credential belongs to the library, not any artifact")
	assert.Equal(t, grant.ID, got.ID)
}

func TestAgentGrantRefusedWhileItsOwnerIsDisabled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)
	_, err = s.CreateAgentGrant(ctx, member.ID, "agent", "", "digest-m")
	require.NoError(t, err)
	require.NoError(t, s.SetUserDisabled(ctx, member.ID, true))
	got, err := s.ResolveAgentGrant(ctx, "digest-m")
	require.NoError(t, err)
	assert.Nil(t, got)
	list, err := s.ListAgentGrants(ctx, member.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Nil(t, list[0].LastUsedAt, "a rejected token is not a use")
	require.NoError(t, s.SetUserDisabled(ctx, member.ID, false))
	got, err = s.ResolveAgentGrant(ctx, "digest-m")
	require.NoError(t, err)
	assert.NotNil(t, got, "re-enabling restores the retained grant")
}

func TestAgentGrantUseIsRecordedCoarsely(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	g, err := s.CreateAgentGrant(ctx, 1, "agent", "", "digest-1")
	require.NoError(t, err)
	old := time.Now().UTC().Add(-time.Hour)
	_, err = s.db.ExecContext(ctx, "UPDATE agent_grants SET last_used_at = ? WHERE id = ?", old.Format(agentGrantTimeLayout), g.ID)
	require.NoError(t, err)
	got, err := s.ResolveAgentGrant(ctx, "digest-1")
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.WithinDuration(t, time.Now(), *got.LastUsedAt, 5*time.Second)
	var stored string
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT last_used_at FROM agent_grants WHERE id = ?", g.ID).Scan(&stored))
	first := stored
	_, err = s.ResolveAgentGrant(ctx, "digest-1")
	require.NoError(t, err)
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT last_used_at FROM agent_grants WHERE id = ?", g.ID).Scan(&stored))
	assert.Equal(t, first, stored)
	// The SQL predicate also protects against a second stale reader touching it.
	require.NoError(t, s.touchAgentGrant(ctx, g.ID, time.Now().Add(time.Second)))
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT last_used_at FROM agent_grants WHERE id = ?", g.ID).Scan(&stored))
	assert.Equal(t, first, stored)
}

func TestAgentGrantForUserRequiresAnEnabledAccountAtCreation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.UpsertUser(ctx, "admin", "admin@example.test")
	require.NoError(t, err)
	member, err := s.UpsertUser(ctx, "member", "member@example.test")
	require.NoError(t, err)

	assertRefused := func(ownerID int64, digest string) {
		t.Helper()
		grant, err := s.CreateAgentGrantForUser(ctx, ownerID, "OAuth agent", "OAuth agent", digest)
		require.ErrorIs(t, err, ErrNotFound)
		assert.Nil(t, grant)
		resolved, err := s.ResolveAgentGrant(ctx, digest)
		require.NoError(t, err)
		assert.Nil(t, resolved)
	}
	assertRefused(999, "missing-user")
	assertRefused(0, "no-owner")

	issued, err := s.CreateAgentGrantForUser(ctx, member.ID, "OAuth agent", "OAuth agent", "enabled-user")
	require.NoError(t, err)
	assert.Equal(t, member.ID, issued.OwnerID)
	assert.Equal(t, "OAuth agent", issued.Label)
	assert.Equal(t, "OAuth agent", issued.Client)
	resolved, err := s.ResolveAgentGrant(ctx, "enabled-user")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, issued.ID, resolved.ID)

	require.NoError(t, s.SetUserDisabled(ctx, member.ID, true))
	assertRefused(member.ID, "disabled-user")
	require.NoError(t, s.SetUserDisabled(ctx, member.ID, false))
	resolved, err = s.ResolveAgentGrant(ctx, "disabled-user")
	require.NoError(t, err)
	assert.Nil(t, resolved, "re-enabling cannot activate a grant refused during disable")

	_, err = s.DeleteAccount(ctx, member.ID)
	require.NoError(t, err)
	assertRefused(member.ID, "deleted-user")
	grants, err := s.ListAgentGrants(ctx, member.ID)
	require.NoError(t, err)
	assert.Empty(t, grants, "an outstanding authorization cannot recreate a deleted account's grant")
	resolved, err = s.ResolveAgentGrant(ctx, "enabled-user")
	require.NoError(t, err)
	assert.Nil(t, resolved, "deletion also removes grants created before it")
}
