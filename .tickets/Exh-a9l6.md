---
id: Exh-a9l6
status: in_progress
deps: []
links: []
created: 2026-10-25T18:10:48Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-35gj
tags: [sharing, store, api]
---
# Split share creation into CreateGrant and CreateAnonymousLink (fail closed on a missing recipient)

`store.CreateShare` takes a `Share` whose `RecipientID *int64` is nil for the anonymous link. The zero value of a share is therefore the PUBLIC link: a grant call site that forgets RecipientID compiles and makes the artifact public. Make the mistake fail closed, with no schema change.

Scope:
- Replace `CreateShare` with `CreateGrant(ctx, ownerID int64, artifactID string, recipientID int64)` and `CreateAnonymousLink(ctx, ownerID int64, artifactID string)` (keep `ReplaceAnonymousLink`). No pointer recipient anywhere on the write path. A recipient of 0 must be refused (it would fail the users(id) FK, but refuse explicitly before the insert and test it).
- Revoking a grant addresses it by (artifact, recipient), not by grant id, so story 2 can drop grant ids without an API change: add an owner-only revoke route (e.g. `DELETE /api/artifacts/{id}/grants?recipient={userID}`, recipient as a query parameter per av-hh1o) and move the share panel's grant revoke buttons to it. `DELETE /api/shares/{id}` stays for the link. Grant views stop exposing an id the client needs.
- Update all callers: internal/api/shares.go, duplicate.go (copy links and grants through the explicit paths or equivalent SQL), tests, share panel JS, e2e share specs.
- Docs: api.md, architecture.md §3.1/§3.3, docs/sharing if any.

