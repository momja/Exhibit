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
