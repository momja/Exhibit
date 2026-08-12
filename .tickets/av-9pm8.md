---
id: av-9pm8
status: closed
deps: []
links: []
created: 2026-08-05T04:40:21Z
type: bug
priority: 0
assignee: Max Omdal
---
# Migration 011_widget.sql silently skipped: version 11 already consumed on deployed DBs

The production instance (aphrodite, exhibit.maxomdal.com) 500s on every gallery/detail page with 'SQL logic error: no such column: a.widget_blob_id'. Startup logs report 'goose: no migrations to run. current version: 11' — goose believes 011 is applied.

It is not. The deployed DB recorded version 11 on 2026-07-25 for a DIFFERENT migration: one that added an 'artifacts.last_visit DATETIME' column. 011_widget.sql was authored 2026-07-31 (493d63c). goose records applied migrations by version number alone, so the widget migration is now permanently skipped on any database that took the earlier version 11.

The originating migration no longer exists anywhere in the repo — no branch, worktree, or ticket references last_visit, and nothing on main reads the column. It was deployed from a build that has since vanished (deleted Supacode worktree branch and/or the 2026-07-18 remote history rewrite).

This is an exact repeat of the 005 collision that internal/store/migration_repair.go already heals at version 8 (agent migration renumbered 005 to 007 out from under 005_downloads_approved.sql).

## Design

Follow the existing precedent rather than renumbering. Renumbering 011 to 012 would repeat the original mistake: databases that legitimately applied 011 as the widget migration (fresh installs, local dev) would then skip 012 and lose the column.

Instead register a guarded, idempotent Go repair migration at version 12 that introspects PRAGMA table_info(artifacts) and adds widget_blob_id only when absent — SQLite has no ADD COLUMN IF NOT EXISTS, which is why this must be Go rather than .sql. It is a no-op on every DB that got the column from 011.

The two repairs are now the same shape, so factor the column guard into one helper both call.

Leave the orphaned last_visit column in place: it is nullable, unreferenced, and dropping it buys nothing.

## Acceptance Criteria

- A database whose version 11 was the last_visit migration gains widget_blob_id on the next startup and serves the gallery.
- A database that applied 011_widget.sql normally is unaffected (repair is a no-op).
- A fresh database still gets the column from 011.
- Test covers both the collided and the normal DB, asserting the column exists and the repair is idempotent across repeated opens.

