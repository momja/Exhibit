---
id: av-4wyq
status: open
deps: [av-7jcq, av-qwld]
links: []
created: 2026-08-09T17:26:41Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-g2dx
tags: [multi-user, account, privacy]
---
# Delete my account and library

A person signing in through an identity provider has no shell on the host and no relationship with the operator. Deleting their own library has to be something the product does.

Blocked on two things that are genuinely prior: the settings surface to invoke it from, and `Blob.Delete` — without the latter, 'delete' leaves every artifact body on disk.


## Notes

**2026-08-09T17:27:04Z**

DESIGN

## Deleting in Exhibit does not delete the identity — say so in the UI

The identity provider is the user directory (deployment.md §3.4). Exhibit can erase what it holds; it cannot remove anyone from Authentik or Clerk. `users.external_id` is UNIQUE, so if the row is deleted and that same person signs in again, `UpsertUser` writes a **fresh row with a new owner id** and they land in an empty library.

That is coherent, and it is not what 'delete my account' usually means. The wording has to be 'delete my library and everything in it', and the confirmation should say plainly that signing in again will work and produce an empty account. Getting this wrong is worse than not shipping it: someone deletes, sees the login still works, and reasonably concludes nothing was deleted.

## What has to go

Follow the schema rather than a remembered list — `artifacts` (and their blobs, av-7jcq), `artifact_state`, `artifact_tags`, `artifact_collections`, `artifact_network_origins`, `tags`, `collections`, `shares`, `agent_keys`, `agent_transcripts`, `sessions`, and the `users` row itself. Several of these already cascade from `artifacts`; the deletion should still be written against the schema as it is, and a test should fail if a new owner-scoped table is added without being considered here. The same tripwire shape the epic's siblings use (walk the thing, require an explicit row) applies: enumerate the owner-bearing tables and fail on one that is unaccounted for.

Note `artifact_state` gained a per-viewer `user_id` in av-q0ub with an AFTER DELETE trigger on `users`, so a deleted user's state rows go automatically — including any they accumulated on **another** owner's artifact. That is correct and worth a test, because it is the one place a user's data lives outside their own library.

## Shares are the sharp edge

A share is a capability URL that anyone may hold. Deleting the library revokes every one of them at once, silently, for people who have no account and no way to be told. That is the right behaviour — the alternative is worse — but the confirmation should say how many live shares are about to break, which requires counting them and is a reason this wants the settings surface rather than an API call.

## Irreversible, and there is no undo anywhere near it

There is no soft delete, no trash, and no snapshot (av-1rvm covers state durability separately). Offer export before deletion, or at minimum say clearly that this is permanent — 'take your data with you' is a stated architectural principle (architecture.md §1) and a delete button without an export path beside it is that principle's opposite.
