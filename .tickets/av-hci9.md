---
id: av-hci9
status: closed
deps: []
links: []
created: 2026-08-17T03:26:03Z
type: bug
priority: 0
assignee: Max Omdal
---
# Migration version 13 collision breaks every deployed database

Merging #101 (integration/multi-user) reassigned goose version 13. On main before the merge, 013_links_approved.sql was version 13 and was applied to the production database on 2026-08-16 17:44. The merge renumbered that file to 018 and gave 013 to 013_users_sessions.sql. goose records applied migrations by version number alone, so production skips users/sessions forever and 014_state_principal.sql dies on 'no such table: main.users'. The app cannot open its store and the container exits. This is the third instance of the collision documented in internal/store/migration_repair.go.

## Design

Two halves, both in migration_repair.go's existing idiom.

1. Ledger repair, before goose.Up: a database whose ledger has version 13 applied but no users table took the pre-merge 013 (links_approved). Delete that row so 013_users_sessions.sql runs. The condition identifies the collision exactly — if 13 were users/sessions the table would exist — and self-heals a restored backup, which a manual UPDATE on the box would not.

2. 018_links_approved.sql becomes a guarded Go repair at version 18, like the repairs at 8 and 12. The collided database already has artifacts.links_approved from its version-13 run, so the bare ALTER would fail the moment the ledger repair lets migrations proceed past it.

Renumbering is deliberately not the fix: it is what migration_repair.go's header rejects, and the one instance already on the new numbering (authtest, ledger at 16) would re-run 019+ and fail on duplicate columns.

## Acceptance Criteria

1. A database in the collided state (ledger 0-13, artifacts.links_approved present, no users table) migrates to head on open, keeps its links_approved values, and gains users/sessions.
2. A database at 12 or below migrates to head unchanged; a fresh database migrates to head.
3. A database already carrying the post-merge numbering (ledger through 16, users present) migrates to head without re-running anything.
4. Production (aphrodite artifact_viewer-app-1) starts and serves.


## Notes

**2026-08-17T03:34:56Z**

Deployed to production from this branch (ansible -e source_dir=<worktree>). Ledger repair fired: version 13 row cleared, 013-018 applied, ledger now 0-18. 43 artifacts, 25 state rows, 1 links_approved preserved. Pre-migration copy of the DB kept in the volume as /data/app.db.pre-av-hci9.
