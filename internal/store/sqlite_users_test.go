package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertUserIsIdempotentAndRefreshesEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, err := s.UpsertUser(ctx, "sub-1", "person@example.test")
	require.NoError(t, err)
	assert.Equal(t, int64(1), first.ID,
		"the first identity to log in becomes owner 1 — the upgrade path for an instance that was single-user")
	assert.False(t, first.CreatedAt.IsZero())

	// Logging in again is the same user, with the email kept current: it is
	// the portable key an instance would need to re-link identities if it
	// ever changed provider.
	again, err := s.UpsertUser(ctx, "sub-1", "renamed@example.test")
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)
	assert.Equal(t, "renamed@example.test", again.Email)

	other, err := s.UpsertUser(ctx, "sub-2", "other@example.test")
	require.NoError(t, err)
	assert.Equal(t, int64(2), other.ID)

	fetched, err := s.GetUser(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, "sub-2", fetched.ExternalID)

	_, err = s.GetUser(ctx, 99)
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = s.UpsertUser(ctx, "", "nobody@example.test")
	assert.Error(t, err, "an identity with no subject is not an identity")
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user, err := s.UpsertUser(ctx, "sub-1", "person@example.test")
	require.NoError(t, err)

	live := &Session{ID: "live", UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, s.CreateSession(ctx, live))
	expired := &Session{ID: "expired", UserID: user.ID, ExpiresAt: time.Now().Add(-time.Minute)}
	require.NoError(t, s.CreateSession(ctx, expired))

	got, err := s.GetSession(ctx, "live")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.UserID)
	assert.WithinDuration(t, live.ExpiresAt, got.ExpiresAt, time.Second)

	// Expired and unknown answer alike: the caller only ever asks whether
	// this request is authenticated.
	_, err = s.GetSession(ctx, "expired")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.GetSession(ctx, "never-existed")
	assert.ErrorIs(t, err, ErrNotFound)

	// Revocation takes effect on the next read, not at the session's TTL.
	require.NoError(t, s.DeleteSession(ctx, "live"))
	_, err = s.GetSession(ctx, "live")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.NoError(t, s.DeleteSession(ctx, "live"), "revoking twice is not an error")

	n, err := s.DeleteExpiredSessions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	err = s.CreateSession(ctx, &Session{ID: "no-expiry", UserID: user.ID})
	assert.Error(t, err, "a session with no expiry would never lapse")
}

func TestDeletingUserCascadesSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user, err := s.UpsertUser(ctx, "sub-1", "person@example.test")
	require.NoError(t, err)
	require.NoError(t, s.CreateSession(ctx, &Session{
		ID: "sess", UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)}))

	_, err = s.db.ExecContext(ctx, "DELETE FROM users WHERE id=?", user.ID)
	require.NoError(t, err)

	_, err = s.GetSession(ctx, "sess")
	assert.ErrorIs(t, err, ErrNotFound, "a user's sessions retire with the user")
}
