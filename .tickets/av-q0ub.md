---
id: av-q0ub
status: closed
deps: [av-ep8k, av-c5aq]
links: [av-buyx, av-7k7b, av-wrbu, av-wmp6, av-v991]
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


## Notes

**2026-08-06T03:36:50Z**

Shipped.

SCHEMA. Migration 014_state_principal.sql rebuilds artifact_state as (artifact_id, user_id, key, value, updated_at) with PRIMARY KEY (artifact_id, user_id, key) -- a rebuild-and-copy, since SQLite cannot alter a primary key. Backfill joins each row to its artifact and files it under that artifact's owner_id (inner join: a row whose artifact is gone is already unreachable via the 001 cascade, and the new table could not hold it). Adds idx_artifact_state_user.

WHY THE SECOND CASCADE IS A TRIGGER, NOT AN FK. AC#4 needs state to die with its viewer. 'REFERENCES users(id) ON DELETE CASCADE' would have been the obvious spelling and is wrong here: an owner id does not imply a users row. Single-user instances run on the static token as owner 1 with an EMPTY users table -- 013's own note says the first identity to log in BECOMES user 1 -- so with PRAGMA foreign_keys=ON a real FK would reject every state write on the most common deployment. No other owner-bearing column in this schema (artifacts.owner_id, tags.owner_id, collections.owner_id) carries such an FK either. An AFTER DELETE ON users trigger buys the cascade without the referential precondition.

TWO PARAMETERS, NOT ONE. The Store's four state methods now read:
  GetState(ctx, ownerID int64, artifactID string, userID int64)
  SetState(ctx, ownerID int64, artifactID string, userID int64, key, value string)
  DeleteState/ClearState likewise.
ownerID is AUTHORIZATION (the existing owner-scoped EXISTS predicate: another owner's artifact stays indistinguishable from one that never existed). userID is SELECTION (whose rows). They hold the same value at every call site today and will not once av-7k7b lets a non-owner open a shared artifact. artifactID sits BETWEEN them on purpose, so transposing the two is a compile error rather than a cross-tenant read. ownerID stays first after ctx, so TestEveryArtifactScopedMethodTakesAnOwner passes without widening its rule or adding an exemption.

WHERE THE PRINCIPAL COMES FROM. Render: the signed render token (av-c5aq) -- serveDoc's threaded 'principal' now feeds GetState's userID, while authorization still comes off the artifact row the handler already resolved, so no third unscoped accessor appears. ServeShare keeps passing a.OwnerID: the share row is the authorization and a share publishes the artifact as its owner sees it. API: the session, via a small statePrincipals(r) helper that returns it twice and names why the two are still two questions.

SCOPING CHOICE WORTH RECORDING. ClearState (the 'erase all' DELETE) removes the CALLER's rows, not the artifact's. 'Erase my state' is what the state inspector offers; erasing another viewer's state is a different, deliberate act that no route grants.

sessionStorage: unchanged, as designed. It is in-memory, framed-only and produces no artifact_state rows, so there is nothing stored to scope. Stated in security.md 1.2 so it is not re-litigated.

TESTS. internal/store/state_principal_test.go, internal/api/state_principal_test.go, internal/render/state_principal_test.go. AC#1 runs 014 against a POPULATED pre-migration database seeded through the four-column schema, with artifacts under two different owners so a backfill hardcoding owner 1 fails. AC#5 is covered twice (store and API) plus a render-side test that a second device's render inlines the first's write and one key inlines once, not once per device. Relaxing the viewer predicate to always-true fails exactly the AC#2/AC#3 tests and nothing else.

NOT DONE, DELIBERATELY. No DeleteUser Store method -- account deletion is not a feature yet, so AC#4 is asserted against the schema cascade itself. No quota enforcement: added to av-wrbu's scope (per-key, per-artifact, per-user) rather than invented here.
