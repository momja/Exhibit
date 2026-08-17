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
// migration numbered above 13 can supply them in time. The repair is therefore
// to the *ledger*, before goose runs at all (repairLedger below): the
// version-13 row in such a database records a migration that is now version 18,
// so it is deleted and 013_users_sessions.sql is left to run normally. Its
// partner is the guarded repair at version 18 — the collided database already
// carries artifacts.links_approved from that same version-13 run, so the bare
// ALTER that used to live in 018_links_approved.sql would fail the moment
// migrations got that far.

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
	{
		// Not a repair for a *skipped* migration: this is 018_links_approved.sql
		// itself, made guarded. The column arrives twice over — under version 13
		// on any database that ran the pre-merge numbering, and under version 18
		// everywhere else — and only the guard lets one file serve both.
		Version:      18,
		Source:       "018_repair_links_approved.go",
		Table:        "artifacts",
		Column:       "links_approved",
		AddStatement: `ALTER TABLE artifacts ADD COLUMN links_approved INTEGER NOT NULL DEFAULT 0`,
	},
}

// usersSessionsVersion is the version 013_users_sessions.sql carries, and the
// version the pre-merge 013_links_approved.sql carried before it.
const usersSessionsVersion = 13

// repairLedger deletes a version-13 ledger row that records the *other*
// migration once numbered 13, so 013_users_sessions.sql can run.
//
// The condition identifies the collision exactly rather than guessing at it: a
// database whose ledger says 13 is applied but which has no `users` table did
// not run 013_users_sessions.sql, because that is the only thing that file
// does. Nothing else in the schema can produce that pair.
//
// It runs before goose, and it is code rather than a one-off UPDATE typed on
// the affected host, for the same reason the column repairs are: a database
// restored from a backup taken before the fix arrives in the collided state
// again, and would need somebody to remember why.
func repairLedger(ctx context.Context, db *sql.DB) error {
	ledger, err := hasTable(ctx, db, "goose_db_version")
	if err != nil || !ledger {
		return err // a fresh database has no ledger and nothing to repair
	}
	var recorded int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM goose_db_version WHERE version_id = ?`,
		usersSessionsVersion).Scan(&recorded); err != nil {
		return err
	}
	if recorded == 0 {
		return nil
	}
	users, err := hasTable(ctx, db, "users")
	if err != nil || users {
		return err
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM goose_db_version WHERE version_id = ?`,
		usersSessionsVersion); err != nil {
		return err
	}
	slog.Warn("repaired migration ledger: version 13 recorded the renumbered links_approved migration, "+
		"so users/sessions never ran; the row is cleared and 013_users_sessions.sql will apply now",
		slog.Int("version", usersSessionsVersion))
	return nil
}

// hasTable reports whether the database defines table (a table or a view).
func hasTable(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx,
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
