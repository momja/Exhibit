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

// The state production reached: version 13 was 013_links_approved.sql when it
// migrated, so the ledger says 13 while users/sessions have never existed.
// Opening the store must repair the ledger, run the real 013, and leave the
// links_approved values that version-13 row was recording.
func TestMigration13CollisionRepairsLedger(t *testing.T) {
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
	require.NoError(t, db.Close())

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

	// The column the version-13 row was really recording keeps its value: the
	// guarded repair at 18 sees it present and adds nothing.
	var approved int
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT links_approved FROM artifacts WHERE id = 'approved'`).Scan(&approved))
	assert.Equal(t, 1, approved, "an existing approval survives the repair")

	// State was re-keyed by 014, whose trigger is what failed before the fix.
	trigger, err := hasTrigger(ctx, s.db, "artifact_state_user_delete")
	require.NoError(t, err)
	assert.True(t, trigger)
}

// A database on the post-merge numbering (users/sessions already at 13) is not
// collided, so the repair must leave its ledger alone and re-run nothing.
func TestMigration13RepairSkipsPostMergeDatabases(t *testing.T) {
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
	assert.True(t, present, "the guarded repair at 18 adds the column it never had")
}

// A database that predates the collision entirely, and a fresh one, both reach
// head with links_approved — the ordinary path through the same repair.
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
