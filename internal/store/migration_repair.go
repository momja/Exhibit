package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

// goose records applied migrations by version number alone. A version that was
// once applied is "already applied" forever, whatever file carries that number
// afterwards — so if two different migrations are ever numbered the same, the
// second one is silently skipped on every database that took the first. It has
// happened twice here:
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
// The repair for both is a guarded, idempotent ADD COLUMN registered as a Go
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
