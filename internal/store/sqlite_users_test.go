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

// --- Administration (av-utap) ------------------------------------------

// Disabling deletes the account's sessions in the same transaction that sets
// the column. That is the half of "disable" a caller could most easily forget,
// which is why it lives in the store rather than in whichever handler happens
// to be calling — the API and the CLI both get it, and neither had to remember.
func TestDisablingAUserDeletesTheirSessionsAndNobodyElses(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	admin, err := s.UpsertUser(ctx, "sub-admin", "admin@example.test")
	require.NoError(t, err)
	require.True(t, admin.IsAdmin)
	member, err := s.UpsertUser(ctx, "sub-member", "member@example.test")
	require.NoError(t, err)

	expiry := time.Now().Add(time.Hour)
	require.NoError(t, s.CreateSession(ctx, &Session{ID: "member-phone", UserID: member.ID, ExpiresAt: expiry}))
	require.NoError(t, s.CreateSession(ctx, &Session{ID: "member-laptop", UserID: member.ID, ExpiresAt: expiry}))
	require.NoError(t, s.CreateSession(ctx, &Session{ID: "admin-laptop", UserID: admin.ID, ExpiresAt: expiry}))

	require.NoError(t, s.SetUserDisabled(ctx, member.ID, true))

	for _, id := range []string{"member-phone", "member-laptop"} {
		_, err := s.GetSession(ctx, id)
		assert.ErrorIs(t, err, ErrNotFound,
			"%s survived the disable — refusing the next login is only half of it, and the "+
				"half that is not the credential the person is actually holding (av-utap)", id)
	}
	_, err = s.GetSession(ctx, "admin-laptop")
	assert.NoError(t, err, "disabling one account must not sign out another")

	assert.True(t, mustGetUser(t, s, member.ID).Disabled)

	// Idempotent, and re-enabling restores the account without restoring the
	// sessions — the person signs in again, which is the correct outcome.
	require.NoError(t, s.SetUserDisabled(ctx, member.ID, true))
	require.NoError(t, s.SetUserDisabled(ctx, member.ID, false))
	assert.False(t, mustGetUser(t, s, member.ID).Disabled)

	assert.ErrorIs(t, s.SetUserDisabled(ctx, 999, true), ErrNotFound)
	assert.ErrorIs(t, s.SetUserDisabled(ctx, 999, false), ErrNotFound)
}

// The guard is in the UPDATE's WHERE clause rather than a read the caller makes
// first, so "is there another admin?" and "write the change" cannot be
// separated by anything. These are the four ways the instance could otherwise
// have been locked out of itself.
func TestTheInstanceKeepsAnAdminWhoCanSignIn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	only, err := s.UpsertUser(ctx, "sub-only", "only@example.test")
	require.NoError(t, err)
	require.True(t, only.IsAdmin)

	assert.ErrorIs(t, s.SetUserAdmin(ctx, only.ID, false), ErrLastAdmin)
	assert.ErrorIs(t, s.SetUserDisabled(ctx, only.ID, true), ErrLastAdmin)
	assert.True(t, mustGetUser(t, s, only.ID).IsAdmin, "a refusal writes nothing")
	assert.False(t, mustGetUser(t, s, only.ID).Disabled)

	// A second admin who cannot sign in is not a second admin. Counting one
	// would let two individually-legal changes leave nobody able to administer
	// the instance.
	second, err := s.UpsertUser(ctx, "sub-second", "second@example.test")
	require.NoError(t, err)
	require.NoError(t, s.SetUserAdmin(ctx, second.ID, true))
	require.NoError(t, s.SetUserDisabled(ctx, second.ID, true))
	assert.ErrorIs(t, s.SetUserDisabled(ctx, only.ID, true), ErrLastAdmin)

	// Enable them and the guard lifts, which is what makes it a guard rather
	// than a prohibition.
	require.NoError(t, s.SetUserDisabled(ctx, second.ID, false))
	require.NoError(t, s.SetUserAdmin(ctx, only.ID, false))
	assert.False(t, mustGetUser(t, s, only.ID).IsAdmin)

	// Demoting somebody who is not an admin changes nothing about who
	// administers the instance, so it is allowed even now that `only` is the
	// one being counted against.
	require.NoError(t, s.SetUserAdmin(ctx, only.ID, false))
	assert.ErrorIs(t, s.SetUserAdmin(ctx, 999, false), ErrNotFound)
	assert.ErrorIs(t, s.SetUserAdmin(ctx, 999, true), ErrNotFound)
}

func mustGetUser(t *testing.T, s *SQLiteStore, id int64) *User {
	t.Helper()
	u, err := s.GetUser(context.Background(), id)
	require.NoError(t, err)
	return u
}
