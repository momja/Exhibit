---
id: av-ep8k
status: in_progress
deps: []
links: [av-e0yj, av-wmp6, av-30rj, av-c5aq, av-5imk, av-syug]
created: 2026-08-05T04:48:55Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-swzv
tags: [security, backend, store, multi-user]
---
# Owner-scope the store: artifacts, state, widgets, shares, origin decisions

`artifacts` carries an `owner_id` column that no query filters on. `GetArtifact(ctx, id)` (internal/store/sqlite.go) resolves any id for any caller; `ListArtifacts` builds its WHERE clause from query/tags/collections only — `ListOptions` has no owner field. The same holds for every method hanging off an artifact: state, widget blob, shares, and origin decisions are all keyed by artifact id alone.

Tags and collections already do this correctly (`WHERE owner_id=?` on list/update/delete, `EXISTS` subqueries on the join writes at sqlite.go:594,611) — that is the pattern to copy, not invent.

The handler layer is ready: `ownerMiddleware` puts an owner in the request context and `ownerIDFromCtx` reads it in 14 places. It just resolves to the constant 1 and the store ignores it.

## Design

Add `OwnerID` to `ListOptions` and an owner parameter to the artifact-scoped Store methods, then filter in SQL. For the artifact-child resources (state, widget, shares, origins) the cheap and consistent form is the owner-scoped EXISTS subquery the tag joins already use, so ownership stays enforced in one place per query rather than in a handler-level pre-check that a future caller can forget.

Key invariant, and the thing acceptance tests should assert: **a cross-tenant id is indistinguishable from a nonexistent one.** Return `store.ErrNotFound`, never a 403. A 403 confirms the row exists, which is a membership oracle over artifact ids.

Two call sites need attention beyond a mechanical parameter add:

- The render surface (internal/render) reads via the same Store but has no session and no owner in context. It must not simply pass "whatever owner" — this is exactly what the signed render token child ticket supplies. Until that lands, the render path can keep its current unscoped read behind a clearly named method (e.g. `GetArtifactUnscoped`) so the gap is greppable rather than implicit.
- `serveShare`/`ServeShare` resolve an artifact through a share row. The share row is the authorization there (architecture.md §7), so that path stays owner-independent by design — but it should go through the same explicitly-named unscoped accessor rather than the default one.

Do not change `owner_id` to a string/external id. It stays an integer; the mapping from an identity provider's subject lives in a `users` table in the provider ticket.

## Acceptance Criteria

1. `ListArtifacts` returns only the requesting owner's artifacts; an artifact belonging to owner 2 never appears for owner 1.
2. `GetArtifact` for another owner's id returns `ErrNotFound` (the handler surfaces 404, not 403).
3. The same holds for PATCH, DELETE, refetch, state GET/PUT/DELETE, widget GET/PUT/DELETE, widget/generate, tag and collection attach/detach, and origin-decision reads/writes.
4. Table-driven store tests cover the two-owner case for every method that takes an artifact id; a new method without owner coverage should be visible in review.
5. Existing single-user behaviour is unchanged: with owner fixed at 1, the full existing test suite passes untouched.
6. Render-surface and share reads are routed through an explicitly-named unscoped accessor, so the remaining unscoped surface is greppable and is the subject of the render-token ticket rather than an accident.

