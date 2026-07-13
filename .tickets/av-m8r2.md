---
id: av-m8r2
status: open
deps: []
links: [av-ryby, Exh-yvhp]
created: 2026-07-13T05:36:17Z
type: bug
priority: 2
assignee: Max Omdal
tags: [store, migrations, data-integrity]
---
# Migration renumber collision skips downloads_approved on agent-PoC-era DBs

Runtime error after deploy: `SQL logic error: no such column: a.downloads_approved (1)`, surfaced by the gallery's `SELECT ... a.downloads_approved ...` query (av-ryby's column).

## Root cause

Commit 1162b17 ("Exh-yvhp: renumber agent migration 005→007 to avoid collision") moved `005_agent.sql` to `007` and reassigned version **5** to `005_downloads_approved.sql` (av-ryby). goose records applied migrations **by version number**, not by filename. So any database that had already applied version 5 — as the agent PoC's `005_agent.sql` — now sees version 5 as "already applied" and **silently skips** `005_downloads_approved.sql`. The `downloads_approved` column is never added, while `006_clipboard_approved.sql` (a never-before-used version) and `007_agent.sql` (idempotent `CREATE TABLE IF NOT EXISTS`) apply fine — leaving `clipboard_approved` present but `downloads_approved` missing. The gallery query then fails with "no such column".

The renumber commit's safety claim ("a no-op on any DB that already applied it") was true for the agent **table** (idempotent CREATE), but missed that reusing version 5 for a *different* migration makes goose skip that migration on those DBs.

A second, compounding factor in the test environment: the deployed `bin/server` was stale (built before av-ryby/av-hll6 landed), so `data/app.db` had only ever been migrated to version 4 and had neither capability-bridge column. The committed migrations 005/006/007 alone repair that under-migrated case; the version-5 reuse is what threatens agent-PoC-era DBs specifically.

## Fix design: guarded idempotent repair at version 8

Register a Go migration at version **8** (a number no prior migration has used for anything else) that introspects `PRAGMA table_info(artifacts)` and `ALTER TABLE artifacts ADD COLUMN downloads_approved INTEGER NOT NULL DEFAULT 0` **only if the column is absent**. This converges every DB population:

- **agent-PoC-era DB** (v5 was 005_agent): 005 is skipped by goose, 006/007 apply, then v8 adds the missing `downloads_approved`.
- **under-migrated DB** (stopped at v4): 005 adds the column, v8 is a no-op.
- **fresh DB / current-main DB** (v5 was 005_downloads_approved): column already present, v8 is a no-op.

SQLite has no `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, so the guard is procedural — hence a Go migration rather than `.sql`. It is registered globally via `goose.SetGlobalMigrations` so goose's `collectMigrationsFS` collects it alongside the embedded `.sql` migrations (collectGoMigrations supports registered Go migrations with no matching `.go` file in the embed FS). Registration is guarded by `sync.Once` since goose's global registry rejects a duplicate version.

No existing artifact data is lost: the column is additive with default 0 (the "not yet approved" first-use state for the download bridge).

## Acceptance criteria

- An agent-PoC-era DB (version 5 recorded as applied, no `downloads_approved` column) opened via `store.OpenSQLite` converges: both `downloads_approved` and `clipboard_approved` present, and `SELECT downloads_approved FROM artifacts LIMIT 1` succeeds.
- A fresh DB and an under-migrated (v4) DB still migrate cleanly; v8 is a no-op whenever the column already exists.
- `goose_db_version` records version 8 after migration; goose reaches "successfully migrated database to version: 8".
- Existing store and API tests pass; the v8 repair is idempotent (re-running OpenSQLite is a no-op — "no migrations to run").

## Notes

**2026-07-13T05:36:17Z**

Implemented on `bug/av-m8r2/migration-renumber-repair`.

- `internal/store/migration_repair.go` (new): `ensureDownloadsApprovedColumn` introspects `PRAGMA table_info(artifacts)` and adds `downloads_approved INTEGER NOT NULL DEFAULT 0` only if absent; wrapped in a `NewGoMigration(8, ...)` registered once per process via `sync.Once` + `goose.SetGlobalMigrations`.
- `internal/store/sqlite.go`: one-line `registerRepairMigration()` call at the top of `migrate()`, before `goose.SetBaseFS`.
- `internal/store/sqlite_test.go`: `TestMigration008RepairsRenumberCollision` stages a collision DB (migrations 001-004 applied, then a forged `goose_db_version` row claiming v5 is applied with no `downloads_approved` column), opens it via `store.OpenSQLite`, and asserts both columns end up present, goose reaches v8, and `SELECT downloads_approved` succeeds.

Verified across three DB populations through the real `store.OpenSQLite` path: (1) agent-PoC-era DB (v5=005_agent) — v5 skipped by goose, v8 adds the column; (2) the actual test `app.db` copy (under-migrated at v4) — 005 adds the column, v8 no-op; (3) fresh DB — 005 adds it, v8 no-op. All converge to v8 with both columns. Deployed to the test environment: `app.db` migrated to version 8, the gallery query that was failing (`SELECT a.downloads_approved`) now returns HTTP 200. Full `go test ./...` green; `go vet` clean.

Note: the committed migrations 005/006/007 (already on main) alone repair fresh and under-migrated DBs. The v8 repair is specifically for the agent-PoC-era collision case where version 5 was consumed by a different migration; without it those DBs silently lose the `downloads_approved` column on upgrade.
