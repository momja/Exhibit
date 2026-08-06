---
id: av-q0ub
status: in_progress
deps: [av-ep8k, av-c5aq]
links: [av-buyx, av-7k7b, av-wrbu, av-wmp6]
created: 2026-08-05T04:49:37Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-swzv
tags: [security, backend, state, store, multi-user]
---
# Scope artifact_state to a principal, not just an artifact

`artifact_state(artifact_id, key, value, updated_at)` records *which tool* the data belongs to but not *whose data it is*. With one owner that is the same question; with more than one it is not.

This is the schema half of the storage-shim story. The render half (who gets state inlined, and what an anonymous share visitor gets) is av-7k7b + av-blzu; the principal that makes those decisions expressible is here.

## Design

Key state by `(artifact_id, user_id, key)`.

```sql
-- SQLite cannot alter a primary key: rebuild the table and copy.
artifact_state(artifact_id, user_id, key, value, updated_at)
  PRIMARY KEY (artifact_id, user_id, key)
```

Backfill existing rows with the artifact's `owner_id`.

Name the column `user_id`, not `owner_id`. It is the **viewer**, and on a shared artifact the viewer and the owner differ — a name that conflates them will produce confused code at exactly the call sites where the distinction matters.

The principal comes from the signed render token (av-c5aq) on the render path and from the session on the API path. The render surface stays sessionless either way.

Cascades: state currently cascades on artifact delete. A user-scoped table needs a second path — deleting a user must drop their state rows. While every state row belongs to the artifact's own owner this is a no-op, but it should exist before any mode lets a non-owner accumulate rows.

Quotas belong with av-wrbu's policy rather than being invented here, but state is a case that ticket does not currently enumerate (it covers artifact bodies, widgets, and URL fetches). Per-key size, per-artifact total, and per-user total are the three ceilings that matter once state is no longer written by exactly one trusted person. Add state to av-wrbu's scope rather than duplicating the decision here.

`sessionStorage` needs no change: it is in-memory, framed-only, and never persisted (spec §5.2). Worth stating so it does not get re-litigated during the migration.

## Acceptance Criteria

1. Migration rebuilds `artifact_state` with the `(artifact_id, user_id, key)` primary key and backfills existing rows to the artifact's owner, with no state loss on an existing database.
2. Two users' state on the same artifact do not collide, and neither can read the other's through any API route.
3. Render-time inlining selects state by the principal from the render token.
4. Deleting a user removes their state rows.
5. Cross-device sync for a single user is unchanged: the same user on two devices reads and writes one set of rows (this is the property the whole state design exists for — it must be covered by a test, not assumed).
6. `av-wrbu` is updated to include state values in its size-ceiling policy.

