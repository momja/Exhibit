package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHostedDeletionRequiresFreshSessionAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hosted.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	oldSession := time.Now().Add(-time.Hour).UnixMilli()
	_, err = s.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", oldSession)
	require.NoError(t, err)
	u, err := s.UpsertHostedUser(ctx, "clerk:member", "old@example.test", oldSession)
	require.NoError(t, err)
	again, err := s.UpsertHostedUser(ctx, "clerk:member", "new@example.test", oldSession)
	require.NoError(t, err)
	require.Equal(t, u.ID, again.ID)
	require.Equal(t, "new@example.test", again.Email)

	_, err = s.DeleteHostedAccount(ctx, u.ID, oldSession)
	require.NoError(t, err)
	var cutoff int64
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT deleted_at_ms FROM hosted_account_deletions WHERE external_id=?", u.ExternalID).Scan(&cutoff))
	for _, createdAt := range []int64{oldSession, cutoff} {
		_, err = s.UpsertHostedUser(ctx, u.ExternalID, "new@example.test", createdAt)
		require.ErrorIs(t, err, ErrNotFound)
	}
	require.NoError(t, s.Close())
	s, err = OpenSQLite(path)
	require.NoError(t, err)
	_, err = s.UpsertHostedUser(ctx, u.ExternalID, "new@example.test", oldSession)
	require.ErrorIs(t, err, ErrNotFound)
	fresh, err := s.UpsertHostedUser(ctx, u.ExternalID, "new@example.test", cutoff+1)
	require.NoError(t, err)
	require.NotEqual(t, u.ID, fresh.ID)
	require.False(t, fresh.IsAdmin)
	// An old browser cannot authenticate as the newly created owner either.
	_, err = s.UpsertHostedUser(ctx, u.ExternalID, "stale@example.test", oldSession)
	require.ErrorIs(t, err, ErrNotFound)
	fresh, err = s.GetUser(ctx, fresh.ID)
	require.NoError(t, err)
	require.Equal(t, "new@example.test", fresh.Email)
}

func TestHostedDeleteGuardIgnoresUnusableLocalAdmin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	created := time.Now().Add(-time.Hour).UnixMilli()
	admin, err := s.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", created)
	require.NoError(t, err)
	local, err := s.UpsertUser(ctx, "local:unusable", "unusable")
	require.NoError(t, err)
	require.NoError(t, s.SetUserAdmin(ctx, local.ID, true))
	_, err = s.DeleteHostedAccount(ctx, admin.ID, created)
	require.ErrorIs(t, err, ErrLastAdmin)
	// Refusal must not leave behind a cutoff that logs the administrator out.
	_, err = s.UpsertHostedUser(ctx, admin.ExternalID, admin.Email, created)
	require.NoError(t, err)
	var cutoffs int
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM hosted_account_deletions").Scan(&cutoffs))
	require.Zero(t, cutoffs)
}

func TestHostedUpsertCannotRaceDeletionToResurrectOldSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	created := time.Now().Add(-time.Hour).UnixMilli()
	_, err := s.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", created)
	require.NoError(t, err)
	member, err := s.UpsertHostedUser(ctx, "clerk:member", "member@example.test", created)
	require.NoError(t, err)
	var wg sync.WaitGroup
	wg.Add(2)
	var deleteErr, upsertErr error
	go func() {
		defer wg.Done()
		_, deleteErr = s.DeleteHostedAccount(ctx, member.ID, created)
	}()
	go func() {
		defer wg.Done()
		_, upsertErr = s.UpsertHostedUser(ctx, member.ExternalID, member.Email, created)
	}()
	wg.Wait()
	require.NoError(t, deleteErr)
	if upsertErr != nil {
		require.ErrorIs(t, upsertErr, ErrNotFound)
	}
	_, err = s.GetUserByExternalID(ctx, member.ExternalID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestHostedDeletionUsesProviderTimeWhenClockIsAhead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	providerCreatedAt := time.Now().Add(time.Hour).UnixMilli()
	_, err := s.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", providerCreatedAt)
	require.NoError(t, err)
	member, err := s.UpsertHostedUser(ctx, "clerk:member", "member@example.test", providerCreatedAt)
	require.NoError(t, err)
	// A second provider session, including one never presented to Exhibit,
	// may have a creation time further ahead of this server's wall clock.
	providerCutoff := providerCreatedAt + 1000
	_, err = s.DeleteHostedAccount(ctx, member.ID, providerCutoff)
	require.NoError(t, err)
	for _, createdAt := range []int64{providerCreatedAt, providerCutoff} {
		_, err = s.UpsertHostedUser(ctx, member.ExternalID, member.Email, createdAt)
		require.ErrorIs(t, err, ErrNotFound)
	}
	_, err = s.UpsertHostedUser(ctx, member.ExternalID, member.Email, providerCutoff+1)
	require.NoError(t, err)
}
