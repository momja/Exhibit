package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Local credentials on the users row (av-rzvf).

const (
	aliceID   = "local:alice@example.test"
	aliceHash = "$2a$10$abcdefghijklmnopqrstuv0123456789012345678901234567890a"
	bobID     = "local:bob@example.test"
	bobHash   = "$2a$10$vutsrqponmlkjihgfedcba9876543210987654321098765432109b"
)

func TestLocalAccountsAreUsersRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	alice, err := s.CreateLocalUser(ctx, aliceID, "alice@example.test", aliceHash)
	require.NoError(t, err)
	bob, err := s.CreateLocalUser(ctx, bobID, "bob@example.test", bobHash)
	require.NoError(t, err)

	// Two accounts, two owner ids, in the same space UpsertUser hands out.
	assert.Equal(t, int64(1), alice.ID)
	assert.Equal(t, int64(2), bob.ID)
	sso, err := s.UpsertUser(ctx, "sub-1", "sso@example.test")
	require.NoError(t, err)
	assert.Equal(t, int64(3), sso.ID)

	// Lookup returns the account and its own hash, never the other's.
	found, hash, err := s.LookupLocalCredential(ctx, aliceID)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, found.ID)
	assert.Equal(t, aliceHash, hash)
	assert.True(t, found.HasPassword)

	// An identity with no password is ErrNotFound here, not a row with an
	// empty hash — the login path must not have an empty string to compare.
	_, _, err = s.LookupLocalCredential(ctx, "sub-1")
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = s.LookupLocalCredential(ctx, "local:nobody@example.test")
	assert.ErrorIs(t, err, ErrNotFound)

	n, err := s.CountLocalCredentials(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "the SSO identity is a user but not a local credential")
}

// "Add a user" must not be a way to silently take over an existing one.
func TestCreateLocalUserRefusesATakenName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.CreateLocalUser(ctx, aliceID, "alice@example.test", aliceHash)
	require.NoError(t, err)

	_, err = s.CreateLocalUser(ctx, aliceID, "alice@example.test", bobHash)
	assert.ErrorIs(t, err, ErrDuplicateName)

	// The original password is untouched.
	_, hash, err := s.LookupLocalCredential(ctx, aliceID)
	require.NoError(t, err)
	assert.Equal(t, aliceHash, hash)

	// An SSO identity's external id is in the same namespace, so it cannot be
	// shadowed by a local account either.
	_, err = s.UpsertUser(ctx, "sub-1", "sso@example.test")
	require.NoError(t, err)
	_, err = s.CreateLocalUser(ctx, "sub-1", "sso@example.test", aliceHash)
	assert.ErrorIs(t, err, ErrDuplicateName)
}

// Changing a password keeps the row, and therefore the library it owns.
// Clearing one leaves the account without a local login rather than deleting
// it — which is how an account becomes SSO-only without losing anything.
func TestSetLocalPasswordKeepsTheAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	alice, err := s.CreateLocalUser(ctx, aliceID, "alice@example.test", aliceHash)
	require.NoError(t, err)

	require.NoError(t, s.SetLocalPassword(ctx, alice.ID, bobHash))
	found, hash, err := s.LookupLocalCredential(ctx, aliceID)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, found.ID)
	assert.Equal(t, bobHash, hash)

	require.NoError(t, s.SetLocalPassword(ctx, alice.ID, ""))
	_, _, err = s.LookupLocalCredential(ctx, aliceID)
	assert.ErrorIs(t, err, ErrNotFound, "no password left to compare against")

	still, err := s.GetUser(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.test", still.Email, "the owner survives losing its password")
	assert.False(t, still.HasPassword)

	assert.ErrorIs(t, s.SetLocalPassword(ctx, 99, aliceHash), ErrNotFound)
}

// The rule lives in the one INSERT that makes a users row, so it holds for
// every door into the instance and cannot be forgotten by a caller.
func TestFirstUserIsAdminWhicheverPathCreatesIt(t *testing.T) {
	t.Run("a provisioned account first", func(t *testing.T) {
		s := newTestStore(t)
		ctx := context.Background()
		first, err := s.CreateLocalUser(ctx, aliceID, "alice@example.test", aliceHash)
		require.NoError(t, err)
		second, err := s.UpsertUser(ctx, "sub-1", "sso@example.test")
		require.NoError(t, err)
		assert.True(t, first.IsAdmin)
		assert.False(t, second.IsAdmin)
	})

	t.Run("an identity first", func(t *testing.T) {
		s := newTestStore(t)
		ctx := context.Background()
		first, err := s.UpsertUser(ctx, "sub-1", "sso@example.test")
		require.NoError(t, err)
		second, err := s.CreateLocalUser(ctx, aliceID, "alice@example.test", aliceHash)
		require.NoError(t, err)
		assert.True(t, first.IsAdmin)
		assert.False(t, second.IsAdmin)

		// A repeat login does not re-run the rule and must not demote anyone.
		again, err := s.UpsertUser(ctx, "sub-1", "renamed@example.test")
		require.NoError(t, err)
		assert.True(t, again.IsAdmin)
	})
}

func TestListUsersIsTheInstanceDirectory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	users, err := s.ListUsers(ctx)
	require.NoError(t, err)
	assert.Empty(t, users)

	_, err = s.CreateLocalUser(ctx, aliceID, "alice@example.test", aliceHash)
	require.NoError(t, err)
	_, err = s.UpsertUser(ctx, "sub-1", "sso@example.test")
	require.NoError(t, err)

	users, err = s.ListUsers(ctx)
	require.NoError(t, err)
	require.Len(t, users, 2)
	assert.Equal(t, "alice@example.test", users[0].Email, "oldest first, so admin first")
	assert.True(t, users[0].IsAdmin)
	assert.True(t, users[0].HasPassword)
	assert.False(t, users[1].HasPassword)
}

// The upgrade path. A database from before av-rzvf holds one local row keyed
// on the constant 'local'; after 016 it must be keyed on its own name, so the
// operator's configured LOGIN_USERNAME still resolves to the library it
// already owns instead of to a fresh empty account.
func TestMigration016RekeysTheLocalRowAndPromotesTheFirstUser(t *testing.T) {
	f, err := os.CreateTemp("", "test-mig016-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite", f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`)
	require.NoError(t, err)

	registerRepairMigrations()
	goose.SetBaseFS(migrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(db, "migrations", 14))

	// An instance as av-q30x left it: the local credential's row, and an SSO
	// identity that logged in afterwards.
	_, err = db.Exec(`INSERT INTO users (id, external_id, email)
	                  VALUES (1, 'local', 'Curator'), (2, 'sub-1', 'sso@example.test')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(db, "migrations", 16))

	var externalID string
	var isAdmin bool
	var hash sql.NullString
	require.NoError(t, db.QueryRow(
		`SELECT external_id, is_admin, password_hash FROM users WHERE id=1`).
		Scan(&externalID, &isAdmin, &hash))
	assert.Equal(t, "local:curator", externalID,
		"re-keyed on its own normalized name, so the configured LOGIN_USERNAME still finds it")
	assert.True(t, isAdmin, "owner 1 adopted the existing library; it is the instance's admin")
	assert.False(t, hash.Valid, "the password is still in the environment, not on the row")

	require.NoError(t, db.QueryRow(
		`SELECT external_id, is_admin FROM users WHERE id=2`).Scan(&externalID, &isAdmin))
	assert.Equal(t, "sub-1", externalID, "a provider subject is not rewritten")
	assert.False(t, isAdmin, "only the first")
}
