package store

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goMigrationVersions are the versions with no file in the embed FS: 8 and 12
// heal the collisions migration_repair.go documents, and 23 normalizes stored
// origins. They are listed here because a walk of the migrations directory
// cannot see them, and a number that looks free but is not is how this repo
// has already lost four instances.
var goMigrationVersions = []int64{8, 12, repairOriginNormalizationVersion}

func embeddedMigrationVersions(t *testing.T) map[int64]string {
	t.Helper()
	entries, err := migrationsFS.ReadDir("migrations")
	require.NoError(t, err)

	versions := make(map[int64]string, len(entries))
	for _, e := range entries {
		name := e.Name()
		require.Greater(t, len(name), 4, name)
		v, err := strconv.ParseInt(name[:3], 10, 64)
		require.NoError(t, err, "migration filenames start with a three-digit version: %s", name)
		if prev, dup := versions[v]; dup {
			t.Fatalf("version %d is carried by two files (%s and %s)", v, prev, name)
		}
		versions[v] = name
	}
	return versions
}

// The collision guard, as a walk rather than a list: goose applies a version
// once and forever, so a second migration wearing a number the ledger already
// holds is silently skipped on every database that took the first. That is
// four outages so far (migration_repair.go), every one of them a number reused
// rather than a statement written wrong.
func TestNoMigrationVersionIsUsedTwice(t *testing.T) {
	files := embeddedMigrationVersions(t)
	for _, v := range goMigrationVersions {
		name, clash := files[v]
		assert.False(t, clash,
			"version %d is a Go migration with no file, and %s claims the same number", v, name)
	}
}

// The other ordering failure, and the one this branch walked into: goose
// refuses to run *anything* when it finds an unapplied migration numbered
// below the ledger's high-water mark, so the instance does not start at all.
//
// A branch is where that happens. Numbers get picked while the branch is young
// — 019 and 020 were the next free ones when 018 was the newest file — and by
// the time it lands, main has shipped 021, 022 and a Go migration at 23. The
// reservation only ever held if the branch merged first.
//
// So every migration must sit above every version that can already be in a
// ledger, from either source. Adding one below fails here rather than on the
// first instance to restart.
func TestEveryMigrationIsAboveTheVersionsAlreadyShipped(t *testing.T) {
	// The versions in a deployed ledger before this branch: every .sql file
	// main shipped, plus the Go migrations at 8, 12 and 23. Written out rather
	// than derived, because "what is already deployed" is history and cannot be
	// computed from the current tree — and because a new file numbered into a
	// gap in this list is exactly what must fail.
	shipped := map[int64]bool{}
	for _, v := range []int64{1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 13, 14, 15, 16, 17, 18, 21, 22} {
		shipped[v] = true
	}
	for _, v := range goMigrationVersions {
		shipped[v] = true
	}

	var highWaterMark int64
	for v := range shipped {
		if v > highWaterMark {
			highWaterMark = v
		}
	}

	for v, name := range embeddedMigrationVersions(t) {
		if shipped[v] {
			continue // already applied everywhere; not this test's business
		}
		assert.Greater(t, v, highWaterMark,
			"%s is numbered below the highest version already in a deployed ledger (%d), so "+
				"goose refuses to run anything and the instance does not start. Take the next "+
				"number above every version in this file, .sql and Go alike.", name, highWaterMark)
	}
}

// The end-to-end version of the same claim: a database migrated by an earlier
// release must still open under this one. Built by rewinding this branch's own
// migrations out of a fully migrated database — ledger rows and the objects
// they created — which is exactly the shape of the instance an upgrade meets.
func TestAnInstanceOnTheEarlierReleaseStillStarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)

	ctx := context.Background()
	for _, stmt := range []string{
		"DELETE FROM goose_db_version WHERE version_id > 23",
		"DROP TABLE IF EXISTS pending_blob_deletions",
		"DROP TABLE IF EXISTS artifact_assets",
		"DROP VIEW IF EXISTS blob_references",
		`CREATE VIEW blob_references AS
             SELECT source_blob_id AS blob_id, owner_id FROM artifacts WHERE source_blob_id != ''
             UNION ALL
             SELECT widget_blob_id AS blob_id, owner_id FROM artifacts WHERE widget_blob_id != ''`,
		// After the view, not before: the one being dropped names
		// artifact_assets, and SQLite resolves a view's body when the table it
		// selects from is altered.
		"ALTER TABLE artifacts DROP COLUMN camera_approved",
		"ALTER TABLE artifacts DROP COLUMN microphone_approved",
	} {
		_, err := s.db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, s.Close())

	upgraded, err := OpenSQLite(path)
	require.NoError(t, err, "an instance on the earlier release must still start")
	defer upgraded.Close()

	// And the upgrade actually ran: the queue, the assets table, and the view
	// that charges an asset's bytes to its owner are all there.
	for _, obj := range []string{"pending_blob_deletions", "artifact_assets", "blob_references"} {
		var n int
		require.NoError(t, upgraded.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM sqlite_master WHERE name = ?", obj).Scan(&n))
		assert.Equal(t, 1, n, "%s was not created by the upgrade", obj)
	}
	// ...and so did 027, whose columns the rewind above took back off.
	for _, col := range []string{"camera_approved", "microphone_approved"} {
		var n int
		require.NoError(t, upgraded.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pragma_table_info('artifacts') WHERE name = ?", col).Scan(&n))
		assert.Equal(t, 1, n, "artifacts.%s was not created by the upgrade", col)
	}
}
