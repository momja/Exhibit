---
id: exhibit-tww.1.1
status: closed
deps: []
links: []
created: 2026-07-01T05:09:07Z
type: task
priority: 2
parent: exhibit-tww.1
---
# Add color column to tags table + plumb through Tag model (migration 003)

Add a per-tag color so pills can be visually distinguished. Create internal/store/migrations/003_tag_color.sql — USE 003, NOT 002: 002_source_url.sql already exists (committed by exhibit-uek); reusing 002 would collide. Migrations are embedded via go:embed (migrations/*.sql in sqlite.go), so 003 is picked up automatically. The migration adds 'color TEXT NOT NULL DEFAULT ''#6B7280''' to the tags table (goose Up/Down; Down drops the column via table rebuild since SQLite can't DROP COLUMN pre-3.35 reliably — or use ALTER ... DROP COLUMN guarded; match the style of 001/002). Backfill is automatic via the DEFAULT. Add Color field to store.Tag (store.go), and update CreateTag/ListTags queries in sqlite.go to write/read color. Update createTag handler + createTagRequest to accept an optional color (fall back to default).

## Acceptance Criteria

Migration applies on a fresh and an existing DB, existing tags get the default color; Tag struct exposes Color (json 'color'); CreateTag persists color; ListTags returns it. Unit test covers create-with-color and default.


