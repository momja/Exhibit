package store

import (
	"context"
	"database/sql"
	"sync"

	"github.com/pressly/goose/v3"
)

// migration 005 (downloads_approved) was renumbered out from under migration
// 005_agent.sql in commit 1162b17 ("renumber agent migration 005→007"). The
// renumber was safe for the agent table — 007 is CREATE TABLE IF NOT EXISTS —
// but it reused version 5 for a *different* migration. goose records applied
// migrations by version number, so any database that had already applied
// version 5 as the agent migration now sees version 5 as "already applied"
// and silently SKIPS 005_downloads_approved.sql. The downloads_approved
// column is never added; clipboard_approved (006, never previously used) is.
// Symptom at runtime: "no such column: a.downloads_approved".
//
// This is a guarded, idempotent repair registered as a Go migration at
// version 8 (a version no prior migration has used for anything else). It
// runs on every database goose migrates to >= 8:
//
//   - agent-PoC-era DBs (v5 was 005_agent): downloads_approved is missing →
//     this adds it. clipboard_approved was added by 006, unaffected.
//   - under-migrated DBs (stopped at v4): 005 already added the column →
//     this is a no-op.
//   - DBs built fresh from current main (v5 was 005_downloads_approved): the
//     column exists → no-op.
//
// SQLite has no ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so the guard is
// procedural: introspect PRAGMA table_info(artifacts) and add only if absent.
// A Go migration is used (rather than .sql) precisely because it needs this
// conditional; it is registered globally so goose collects it alongside the
// embedded .sql migrations (collectGoMigrations supports registered Go
// migrations with no matching .go file in the embed FS).

const repairDownloadsApprovedVersion int64 = 8

var registerRepairOnce sync.Once

// registerRepairMigration registers the version-8 repair migration exactly
// once per process. goose.SetGlobalMigrations is global state and rejects a
// duplicate version, so the sync.Once guards repeated OpenSQLite calls (and
// the test process).
func registerRepairMigration() {
	registerRepairOnce.Do(func() {
		m := goose.NewGoMigration(repairDownloadsApprovedVersion,
			&goose.GoFunc{RunTx: ensureDownloadsApprovedColumn},
			nil,
		)
		m.Source = "008_repair_downloads_approved.go"
		if err := goose.SetGlobalMigrations(m); err != nil {
			// A migration at this version is already registered; nothing to do.
			return
		}
	})
}

// ensureDownloadsApprovedColumn adds the downloads_approved column to
// artifacts iff it is not already present. Idempotent and safe to re-run.
func ensureDownloadsApprovedColumn(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(artifacts)`)
	if err != nil {
		return err
	}
	hasColumn := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "downloads_approved" {
			hasColumn = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = tx.ExecContext(ctx,
		`ALTER TABLE artifacts ADD COLUMN downloads_approved INTEGER NOT NULL DEFAULT 0`)
	return err
}
