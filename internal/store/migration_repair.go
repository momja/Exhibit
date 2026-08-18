package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/pressly/goose/v3"
)

// goose records applied migrations by version number alone. A version that was
// once applied is "already applied" forever, whatever file carries that number
// afterwards — so if two different migrations are ever numbered the same, the
// second one is silently skipped on every database that took the first. It has
// happened three times here:
//
//   - version 5. 005_agent.sql was renumbered to 007 (commit 1162b17) and
//     version 5 reassigned to 005_downloads_approved.sql. Databases that ran
//     the agent-PoC-era 005 never got downloads_approved; clipboard_approved
//     (006, a number never previously used) arrived normally.
//     Symptom: "no such column: a.downloads_approved".
//
//   - version 11. The deployed instance recorded version 11 on 2026-07-25 for
//     a migration adding artifacts.last_visit — from a build that no longer
//     exists anywhere in this repo (no branch, worktree, or ticket references
//     the column, and nothing reads it). 011_widget.sql was authored
//     2026-07-31, so that database skips it and never gets widget_blob_id.
//     Symptom: "no such column: a.widget_blob_id".
//
//   - version 13. 013_links_approved.sql (av-r0dk) landed on main and was
//     applied to the deployed instance on 2026-08-16; merging the multi-user
//     integration branch hours later renumbered that file to 018 and gave 13
//     to 013_users_sessions.sql. A database that took the first 13 therefore
//     skips users/sessions forever, and the very next migration — 014, whose
//     trigger fires on DELETE FROM users — cannot even be created.
//     Symptom: "no such table: main.users", at startup, before the store opens.
//
// The repair for the first two is a guarded, idempotent ADD COLUMN registered as a Go
// migration at a version no migration has ever used. Renumbering the skipped
// file instead would just repeat the original mistake in the other direction:
// databases that *did* apply it under its current number would skip the
// renumbered copy and lose the column.
//
// SQLite has no ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so the guard is
// procedural — introspect PRAGMA table_info and add only if absent. That is
// why these are Go migrations rather than .sql, and it makes each one a no-op
// on databases that got the column the ordinary way. They are registered
// globally so goose collects them alongside the embedded .sql migrations
// (collectGoMigrations supports registered Go migrations with no matching .go
// file in the embed FS).
//
// Version 13 needs more than a column, and it is the one case that column
// repairs cannot reach: what the collided database is missing is two tables
// every later migration builds on, and goose runs in ascending order, so no
// migration numbered above 13 can supply them in time. So this repair runs
// before goose does, and it *rewinds* rather than patches — see
// rewindReusedVersion13 below.
//
// RULE — repairs heal damaged databases; they never define schema. A repair
// that a fresh install depends on is a migration wearing the wrong name, and
// the whole set stops being deletable: the file could never retire, because
// retiring it would break installs that were never damaged. Concretely, the
// version-13 repair could have been much shorter if 018_links_approved.sql
// became a *guarded* ADD COLUMN like the two above — the collided database
// already carries that column, so the plain ALTER fails on it. That guard is
// what this rule rejects: every fresh database would then get links_approved
// from a file named "repair". Rewinding keeps 018 an ordinary migration, and
// keeps the invariant that a database with no history to fix never executes a
// line of this file.

// columnRepair is one guarded ADD COLUMN: at goose version Version, ensure
// Table has Column, adding it with AddStatement when it does not.
type columnRepair struct {
	Version      int64
	Source       string // the file name goose reports in its logs
	Table        string
	Column       string
	AddStatement string
}

// columnRepairs heal the two version collisions described above. A repair's
// version must sit above the collided version it repairs and below anything
// that depends on the column; both here follow immediately after the number
// that was reused.
var columnRepairs = []columnRepair{
	{
		Version:      8,
		Source:       "008_repair_downloads_approved.go",
		Table:        "artifacts",
		Column:       "downloads_approved",
		AddStatement: `ALTER TABLE artifacts ADD COLUMN downloads_approved INTEGER NOT NULL DEFAULT 0`,
	},
	{
		Version:      12,
		Source:       "012_repair_widget_blob_id.go",
		Table:        "artifacts",
		Column:       "widget_blob_id",
		AddStatement: `ALTER TABLE artifacts ADD COLUMN widget_blob_id TEXT NOT NULL DEFAULT ''`,
	},
}

// usersSessionsVersion is the version 013_users_sessions.sql carries, and the
// version the pre-merge 013_links_approved.sql carried before it.
const usersSessionsVersion = 13

// linksApprovedStash holds the approvals rewindReusedVersion13 takes out of
// artifacts, until restoreRewoundApprovals puts them back. Its existence is
// also the marker that a rewind is half-finished, which is why it is a table
// and not a slice in memory: a crash between the two steps must not lose the
// user's approvals, and the next startup can simply finish the job.
const linksApprovedStash = "repair_links_approved_stash"

// rewindReusedVersion13 returns a database that applied the pre-merge version
// 13 to the state it would have been in had that version never been reused, so
// that the ordinary migration sequence can carry it forward from there.
//
// The condition identifies the collision exactly rather than guessing at it: a
// database whose ledger says 13 is applied but which has no `users` table did
// not run 013_users_sessions.sql, because that is the only thing that file
// does. Nothing else in the schema can produce that pair.
//
// Rewinding means undoing what the *other* version 13 did — dropping
// artifacts.links_approved, holding its values aside — and then deleting the
// ledger row. 013_users_sessions.sql then applies, and so does every migration
// after it including the ordinary 018_links_approved.sql, which re-adds the
// column that 018 has always owned. The alternative was to make 018 skip a
// column that already exists, and that would have put a piece of the schema in
// this file forever (see the RULE above).
//
// It is code rather than a one-off UPDATE typed on the affected host for the
// same reason the column repairs are: a database restored from a backup taken
// before the fix arrives in the collided state again, and would need somebody
// to remember why.
func rewindReusedVersion13(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	// The collision is detected through the same transaction that acts on it,
	// never ahead of it. Two processes can open one database file — a restart
	// overlapping the old container, or the `user` CLI run against a starting
	// server — and both would observe the collision at the same moment. Reading
	// the predicate outside the transaction lets the slower one act on that
	// observation *after* the faster one finished, deleting the version-13 row
	// 013_users_sessions.sql has since recorded and dropping the column 018 has
	// since re-added: a repaired database put straight back into the broken
	// state, minus the stash. Inside the transaction the slower one either sees
	// the repaired database and does nothing, or its write meets the other's
	// commit and the rewind is refused with an error. Neither outcome is
	// destructive, which is the whole property being bought here.
	collided, err := hasReusedVersion13(ctx, tx)
	if err != nil || !collided {
		return err
	}

	// A database that recorded version 13 for something else again — the
	// last_visit build in the header is precedent that such a thing exists —
	// has no column to rewind, and only its ledger row is in the way.
	present, err := hasColumn(ctx, tx, "artifacts", "links_approved")
	if err != nil {
		return err
	}
	if present {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %q AS
			 SELECT id, links_approved FROM artifacts WHERE links_approved <> 0`,
			linksApprovedStash)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`ALTER TABLE artifacts DROP COLUMN links_approved`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM goose_db_version WHERE version_id = ?`, usersSessionsVersion); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Warn("rewound the reused migration version 13: it recorded the renumbered links_approved "+
		"migration, so users/sessions never ran; migrations 013 onward will apply now",
		slog.Int("version", usersSessionsVersion), slog.Bool("links_approved_stashed", present))
	return nil
}

// hasReusedVersion13 reports whether this database applied the pre-merge
// migration numbered 13 instead of 013_users_sessions.sql.
func hasReusedVersion13(ctx context.Context, q rowQuerier) (bool, error) {
	ledger, err := hasTable(ctx, q, "goose_db_version")
	if err != nil || !ledger {
		return false, err // a fresh database has no ledger and nothing to rewind
	}
	var recorded int
	if err := q.QueryRowContext(ctx,
		`SELECT count(*) FROM goose_db_version WHERE version_id = ?`,
		usersSessionsVersion).Scan(&recorded); err != nil {
		return false, err
	}
	if recorded == 0 {
		return false, nil
	}
	users, err := hasTable(ctx, q, "users")
	return !users, err
}

// restoreRewoundApprovals puts the stashed approvals back after the migrations
// have re-added the column, and runs on every startup because the stash is also
// how a rewind interrupted halfway announces itself.
func restoreRewoundApprovals(ctx context.Context, db *sql.DB) error {
	stashed, err := hasTable(ctx, db, linksApprovedStash)
	if err != nil || !stashed {
		return err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(
		`UPDATE artifacts
		    SET links_approved = (SELECT s.links_approved FROM %q s WHERE s.id = artifacts.id)
		  WHERE id IN (SELECT id FROM %q)`,
		linksApprovedStash, linksApprovedStash)); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %q`, linksApprovedStash)); err != nil {
		return err
	}
	slog.Warn("restored the link approvals held aside by the version-13 rewind")
	return nil
}

// rowQuerier is the single-row read both *sql.DB and *sql.Tx provide, so a
// check can be made either on its own or inside the transaction that acts on
// its answer. rewindReusedVersion13 needs the latter; see the note there.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// hasTable reports whether the database defines table (a table or a view).
func hasTable(ctx context.Context, q rowQuerier, table string) (bool, error) {
	var name string
	err := q.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

var registerRepairsOnce sync.Once

// registerRepairMigrations registers every column repair exactly once per
// process. goose.SetGlobalMigrations is global state and rejects a duplicate
// version, so the sync.Once guards repeated OpenSQLite calls (and the test
// process, which opens many databases).
func registerRepairMigrations() {
	registerRepairsOnce.Do(func() {
		migrations := make([]*goose.Migration, 0, len(columnRepairs))
		for _, repair := range columnRepairs {
			m := goose.NewGoMigration(repair.Version,
				&goose.GoFunc{RunTx: repair.run},
				nil,
			)
			m.Source = repair.Source
			migrations = append(migrations, m)
		}
		if err := goose.SetGlobalMigrations(migrations...); err != nil {
			// A migration at one of these versions is already registered;
			// nothing to do.
			return
		}
	})
}

// run adds the repair's column iff it is not already present. Idempotent and
// safe to re-run.
func (r columnRepair) run(ctx context.Context, tx *sql.Tx) error {
	present, err := hasColumn(ctx, tx, r.Table, r.Column)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	_, err = tx.ExecContext(ctx, r.AddStatement)
	return err
}

// hasColumn reports whether table already defines column.
func hasColumn(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	// PRAGMA table_info takes no bound parameters, so the table name is
	// interpolated. Every caller is a compile-time constant in columnRepairs,
	// never user input.
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			found = true
		}
	}
	return found, rows.Err()
}
