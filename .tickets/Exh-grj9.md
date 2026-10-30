---
id: Exh-grj9
status: closed
deps: [Exh-a9l6]
links: []
created: 2026-10-25T18:10:48Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-35gj
tags: [sharing, store, schema, migration]
---
# Split the shares table into share_links and share_grants

One migration (039, the next above main's 038; check migration_order_test.go, the Go migrations, and the CI unique-migration-number guard) that:

- Creates `share_links(id TEXT PRIMARY KEY, artifact_id TEXT NOT NULL UNIQUE REFERENCES artifacts(id) ON DELETE CASCADE)` and `share_grants(artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE, recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')), PRIMARY KEY (artifact_id, recipient_id)) WITHOUT ROWID` plus `CREATE INDEX share_grants_by_recipient ON share_grants(recipient_id)` (the WITHOUT ROWID secondary index carries the PK columns, so it is covering).
- Copies existing rows: `recipient_id IS NULL` rows into share_links keeping their ids (existing /s/:id URLs must keep working); grant rows into share_grants with synthetic millisecond timestamps preserving their old rowid order.
- Drops `shares` (its shares_counts_sync_* triggers go with it) and drops `artifacts.share_link` / `artifacts.share_grant_count`.
- Lists the link first explicitly, then grants by created_at and recipient_id for millisecond ties; duplication preserves relative grant order.
- Honest goose Down that names what it loses.

Code: every query on shares moves (CreateGrant, CreateAnonymousLink, ReplaceAnonymousLink, GetShare, ListArtifactShares, GetAnonymousShareUnscoped — whose IS NULL guard disappears because grants have no id — readableByViewer, duplicate.go). The gallery card share badge (shareBadgeView / capability cluster) and anything else reading the rollups computes them in the list query with EXISTS / COUNT over the two tables. Account deletion: add both tables to sqlite_account.go (TestEveryTableIsAccountedForInAccountDeletion). Confirm the render token's reserved `s` claim (av-7k7b) only ever means a link id.

Verify with EXPLAIN QUERY PLAN in a test or note: listing grants by recipient is a SEARCH on share_grants_by_recipient.

Docs: PRD §4.4 and §7 schema/sharing text, architecture.md §3.3 and §7, api.md.

