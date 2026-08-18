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

// newMigratedTo opens a fresh database and applies migrations up to version,
// leaving it closed and ready for OpenSQLite to finish the job.
func newMigratedTo(t *testing.T, version int64) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-ledger-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := sql.Open("sqlite", f.Name())
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`)
	require.NoError(t, err)

	registerRepairMigrations() // the repairs at 8 and 12 are part of the sequence
	goose.SetBaseFS(migrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(db, "migrations", version))
	require.NoError(t, db.Close())
	return f.Name()
}

func currentVersion(t *testing.T, s *SQLiteStore) int64 {
	t.Helper()
	v, err := goose.GetDBVersion(s.db)
	require.NoError(t, err)
	return v
}

// collidedDatabase is the state production reached: version 13 was
// 013_links_approved.sql when it migrated, so the ledger says 13 while
// users/sessions have never existed.
func collidedDatabase(t *testing.T) string {
	t.Helper()
	path := newMigratedTo(t, 12)

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	// Exactly what the pre-merge 013_links_approved.sql did.
	_, err = db.Exec(`ALTER TABLE artifacts ADD COLUMN links_approved INTEGER NOT NULL DEFAULT 0`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (13, 1)`)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO artifacts (id, owner_id, title, source_blob_id, tier, links_approved)
		 VALUES ('approved', 1, 'Approved', 'blob-approved', 1, 1)`)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO artifacts (id, owner_id, title, source_blob_id, tier, links_approved)
		 VALUES ('unapproved', 1, 'Unapproved', 'blob-unapproved', 1, 0)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return path
}

// Opening the store must rewind the reused version, run the real 013, and end
// with the approvals that version-13 row was recording.
func TestMigration13CollisionRewindsAndRestores(t *testing.T) {
	path := collidedDatabase(t)

	s, err := OpenSQLite(path)
	require.NoError(t, err, "the collided database must migrate to head")
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	users, err := hasTable(ctx, s.db, "users")
	require.NoError(t, err)
	assert.True(t, users, "013_users_sessions.sql runs after the ledger repair")
	sessions, err := hasTable(ctx, s.db, "sessions")
	require.NoError(t, err)
	assert.True(t, sessions)

	// The column the version-13 row was really recording is dropped and re-added
	// by the ordinary 018, and its values come back from the stash.
	var approved, unapproved int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT links_approved FROM artifacts WHERE id = 'approved'`).Scan(&approved))
	assert.Equal(t, 1, approved, "an existing approval survives the rewind")
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT links_approved FROM artifacts WHERE id = 'unapproved'`).Scan(&unapproved))
	assert.Equal(t, 0, unapproved, "and nothing else is approved on its way through")

	stashed, err := hasTable(ctx, s.db, linksApprovedStash)
	require.NoError(t, err)
	assert.False(t, stashed, "the stash is dropped once the approvals are back")

	// State was re-keyed by 014, whose trigger is what failed before the fix.
	trigger, err := hasTrigger(ctx, s.db, "artifact_state_user_delete")
	require.NoError(t, err)
	assert.True(t, trigger)
}

// Re-opening a rewound database must not rewind it a second time: after the
// first open its version-13 row is the real users/sessions migration.
func TestMigration13RewindHappensOnce(t *testing.T) {
	path := collidedDatabase(t)

	first, err := OpenSQLite(path)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	collided, err := hasReusedVersion13(context.Background(), s.db)
	require.NoError(t, err)
	assert.False(t, collided, "the second open sees an ordinary database")
	assert.Equal(t, int64(18), currentVersion(t, s))

	var approved int
	require.NoError(t, s.db.QueryRow(
		`SELECT links_approved FROM artifacts WHERE id = 'approved'`).Scan(&approved))
	assert.Equal(t, 1, approved)
}

// A crash between the rewind and the restore leaves the stash behind, which is
// why it is a table: the next startup finishes the job.
func TestMigration13RewindResumesAfterCrash(t *testing.T) {
	path := collidedDatabase(t)

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	require.NoError(t, rewindReusedVersion13(context.Background(), db))
	stashed, err := hasTable(context.Background(), db, linksApprovedStash)
	require.NoError(t, err)
	require.True(t, stashed, "the approvals are held aside at this point")
	require.NoError(t, db.Close()) // the process dies here, before goose runs

	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	var approved int
	require.NoError(t, s.db.QueryRow(
		`SELECT links_approved FROM artifacts WHERE id = 'approved'`).Scan(&approved))
	assert.Equal(t, 1, approved, "the interrupted rewind is completed, not lost")
}

// A database on the post-merge numbering (users/sessions already at 13) is not
// collided, so nothing may touch its ledger or re-run against it.
func TestMigration13RewindSkipsPostMergeDatabases(t *testing.T) {
	path := newMigratedTo(t, 16)

	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	var recorded int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM goose_db_version WHERE version_id = 13`).Scan(&recorded))
	assert.Equal(t, 1, recorded, "the version-13 row stays applied")
	assert.Equal(t, int64(18), currentVersion(t, s), "17 and 18 apply on top")

	present, err := hasColumnDB(ctx, s.db, "artifacts", "links_approved")
	require.NoError(t, err)
	assert.True(t, present, "018 adds the column this database never had")
}

// A database that predates the collision entirely, and a fresh one, both reach
// head with links_approved — from 018_links_approved.sql, with no repair
// involved. The rule this pins: repairs heal damaged databases and never
// define schema, so an install with no history to fix depends on none of them.
func TestMigrationsReachHeadWithoutCollision(t *testing.T) {
	for name, path := range map[string]string{
		"pre-collision": newMigratedTo(t, 12),
		"fresh":         "",
	} {
		t.Run(name, func(t *testing.T) {
			p := path
			if p == "" {
				f, err := os.CreateTemp("", "test-fresh-*.db")
				require.NoError(t, err)
				f.Close()
				t.Cleanup(func() { os.Remove(f.Name()) })
				p = f.Name()
			}
			s, err := OpenSQLite(p)
			require.NoError(t, err)
			t.Cleanup(func() { s.Close() })

			assert.Equal(t, int64(18), currentVersion(t, s))
			present, err := hasColumnDB(context.Background(), s.db, "artifacts", "links_approved")
			require.NoError(t, err)
			assert.True(t, present)
		})
	}
}

// hasColumnDB is hasColumn against a *sql.DB rather than a migration's *sql.Tx.
func hasColumnDB(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck // read-only
	return hasColumn(ctx, tx, table, column)
}

func hasTrigger(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var got string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'trigger' AND name = ?`, name).Scan(&got)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// One database file can be opened by two processes at once — a restart that
// overlaps the container it replaces, or the `user` CLI run against a starting
// server. Both would see the collision. The one that acts second must find the
// repaired database and leave it alone; deleting the version-13 row the winner's
// 013_users_sessions.sql just recorded would put the database straight back into
// the state this whole file exists to undo.
func TestMigration13RewindIsANoOpOnARepairedDatabase(t *testing.T) {
	path := collidedDatabase(t)

	// The connection the loser is holding: opened while the database is still
	// collided, and still open after the winner has finished with it.
	loser, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	loser.SetMaxOpenConns(1)
	t.Cleanup(func() { loser.Close() })
	collided, err := hasReusedVersion13(context.Background(), loser)
	require.NoError(t, err)
	require.True(t, collided, "the loser observes the collision before the winner acts")

	winner, err := OpenSQLite(path)
	require.NoError(t, err)
	require.NoError(t, winner.Close())

	require.NoError(t, rewindReusedVersion13(context.Background(), loser),
		"the rewind re-reads the database it is about to act on")

	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	users, err := hasTable(ctx, s.db, "users")
	require.NoError(t, err)
	assert.True(t, users, "the repaired schema survives")
	var recorded int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM goose_db_version WHERE version_id = ?`, usersSessionsVersion).Scan(&recorded))
	assert.Equal(t, 1, recorded, "and so does the genuine version-13 row")
	var approved int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT links_approved FROM artifacts WHERE id = 'approved'`).Scan(&approved))
	assert.Equal(t, 1, approved)
}

// The same collision, raced for real over independent connections. Whoever wins,
// and whether or not the loser's write is refused outright, the file must be a
// correctly migrated database once the dust settles.
func TestMigration13RewindSurvivesConcurrentRepairs(t *testing.T) {
	path := collidedDatabase(t)

	other, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	other.SetMaxOpenConns(1)
	t.Cleanup(func() { other.Close() })

	start := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-start
		// A refusal is an acceptable outcome here; a partial rewind is not.
		_ = rewindReusedVersion13(context.Background(), other)
	}()

	close(start)
	winner, err := OpenSQLite(path)
	<-done
	if err == nil {
		require.NoError(t, winner.Close())
	}

	s, err := OpenSQLite(path)
	require.NoError(t, err, "the raced database still migrates to head")
	t.Cleanup(func() { s.Close() })

	users, err := hasTable(context.Background(), s.db, "users")
	require.NoError(t, err)
	assert.True(t, users)
	assert.Equal(t, int64(18), currentVersion(t, s))
	var approved int
	require.NoError(t, s.db.QueryRowContext(context.Background(),
		`SELECT links_approved FROM artifacts WHERE id = 'approved'`).Scan(&approved))
	assert.Equal(t, 1, approved, "the approval is not lost to the race")
}
