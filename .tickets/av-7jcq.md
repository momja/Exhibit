---
id: av-7jcq
status: in_progress
deps: []
links: [av-1rvm]
created: 2026-08-09T17:26:41Z
type: bug
priority: 2
assignee: Max Omdal
parent: av-g2dx
tags: [storage, blob, privacy]
---
# Blob.Store cannot delete: artifact bodies survive every deletion

`blob.Store` is `Put` and `Get`. There is no `Delete`.

So no deletion path in the product actually removes an artifact's bytes. `DELETE /api/artifacts/:id` drops the row and lets the FK cascade take tags, collections, shares and state — and leaves the body on the filesystem forever. architecture.md §3.1 records this as accepted for v1 ('The blob body on the filesystem is orphaned in v1').

Accepted for a single-user instance deleting their own artifact is one thing. It stops being acceptable the moment 'delete my account' exists: a deletion that leaves every artifact body on disk is not deletion in any sense a user would recognise, and on a hosted instance it is the kind of claim that has legal weight.

This is the prerequisite for account deletion, not a follow-up to it.

