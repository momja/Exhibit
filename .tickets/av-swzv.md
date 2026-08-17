---
id: av-swzv
status: closed
deps: []
links: []
created: 2026-08-05T04:48:32Z
type: epic
priority: 1
assignee: Max Omdal
tags: [security, backend, multi-user]
---
# Epic: Multi-tenant data isolation — make owner_id real

The `owner_id` seam the docs promise (architecture.md §8, spec §4.5) is half-built. Columns exist on `artifacts`, `collections`, `tags`, `agent_keys`, and tags/collections are genuinely owner-scoped in SQL. Artifacts are not: `GetArtifact(ctx, id)` takes no owner, `ListOptions` has no owner field, and every state/widget/share/origin-decision method is keyed by id alone (internal/store/sqlite.go). Today `owner_id` is a column that gets written and never read back as a predicate.

That is invisible while there is one user and load-bearing the moment there are two. This epic makes the seam real, in the order that keeps each step independently useful to the single-user product.

Scope: data isolation and the render-surface principal. Identity *providers* (who the user is, how they log in) are a separate concern — see av-q30x for the single-user login and the child ticket here for the pluggable-provider seam.

Explicitly not in scope: registration/invite UI, orgs, billing, per-user quota enforcement beyond what av-wrbu's policy implies.

## Why owner-scoping goes first

Everything else in this area is either blocked by it or becomes a vulnerability without it:
- av-wmp6 (public mode) makes unauthenticated `GET /api/artifacts` return "the" library — meaningless once there are multiple owners, and a cross-tenant read the day multi-user lands.
- av-e0yj scopes the *agent* to one artifact, but the API it calls has no owner boundary underneath.
- Any hosted/multi-tenant deployment is unsafe until this lands.

## Sequencing

1. Owner-scope the store (prerequisite for all of the below)
2. Signed render tokens — closes the cross-tenant read on RENDER_ORIGIN and carries the state principal
3. `artifact_state` principal scoping
4. Pluggable identity provider seam

