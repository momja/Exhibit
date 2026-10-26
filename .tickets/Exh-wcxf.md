---
id: Exh-wcxf
status: in_progress
deps: []
links: []
created: 2026-10-26T01:02:13Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-qeuv
tags: [sharing, store]
---
# Store: list artifacts shared with a viewer and shared by an owner

Backend half of the spaces. No UI.

Add to the Store interface and SQLiteStore:

- ListArtifactsSharedWith(ctx, viewer ViewerID, opts SharedListOptions) ([]*SharedArtifact, error): artifacts with a share_grants row naming the viewer, excluding artifacts the viewer owns. Same paging (Limit/Offset), ordering (created_at, id — match ListArtifacts) and FTS query semantics as ListArtifacts. SharedArtifact carries the *Artifact plus the owner's display name (reuse the /profile profileName rule: email, else provider subject, else local name — move the rule into the store or a shared helper rather than duplicating it). Optional OwnerIDs filter (the shared space filters by owner, not by tag — the tags belong to the owner).
- ListArtifactsSharedBy: ListArtifacts with a new ListOptions.SharedOnly bool (artifact has any share_grants row OR a share_links row). Owner-scoped exactly like ListArtifacts.
- SharedWithOwners(ctx, viewer) ([]SharedOwner{ID, Name, Count}): drives the owner filter.
- SpaceCounts(ctx, viewer) (sharedWith, sharedBy int, error): cheap existence/count used to decide whether the spaces nav renders at all.

Rules:
- Reading another owner's artifact is a *viewer* accessor (architecture §3.3: 'Non-owners use parallel accessors, never a widened predicate'). Do not widen ownedArtifact or ListArtifacts' owner predicate. The returned artifacts must be the same ones GetArtifactReadableBy resolves; add a test asserting every listed id resolves through it and that a revoked grant disappears from the list.
- Must use share_grants_by_recipient: add these queries to owner_query_plan_test.go asserting SEARCH share_grants USING ... share_grants_by_recipient and no SCAN of artifacts/share_grants. (See the review note on #209: a correlated EXISTS without an outer artifact_id filter scans the whole child table — do not repeat that.)
- A disabled owner's artifacts: decide and test (recommend: still listed, since GetArtifactReadableBy still resolves them; if it does not, match it).
- No migration expected. If one turns out necessary, take the next number above every version on main AND origin/feature/Exh-g2vv/relay-connectors (which holds 038-042).

## Acceptance Criteria

- Store methods + interface + any fakes compile; go test ./... passes.
- Query-plan tests prove the recipient index is used and nothing scans.
- Revoked grant, deleted artifact, deleted recipient, and self-owned artifact cases tested.
- Shared-by includes link-only and grant-only artifacts, excludes private ones.

