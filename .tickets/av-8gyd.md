---
id: av-8gyd
status: open
deps: []
links: [av-20fk, av-7jcq]
created: 2026-08-17T04:24:04Z
type: task
priority: 3
assignee: Max Omdal
tags: [storage, blob, ops, cli]
---
# Blob deletion queue: reclaim bytes automatically, plus a one-time sweep for historical orphans

There is no way to find or reclaim blobs on disk that no row points at, and there are certainly some.

[[av-7jcq]] gave `blob.Store` a `Delete` and wired it into the deletion paths, but it did not backfill: every artifact deleted *before* that shipped left its body on the filesystem forever, exactly as architecture.md §3.1 had accepted for v1. Any deployment predating av-7jcq — production included — is carrying those bytes now, and nothing in the product can tell you how many or remove them.

[[av-20fk]] adds a second, ongoing source. It deletes superseded asset blobs inline, after the transaction commits, so a crash in that window leaves unreferenced bytes behind. Rare, small, and by design: the alternative ordering leaves rows pointing at blobs that are already gone, which is worse.

The two need different answers. The ongoing leak should never require a human, and does not have to — the deleting code already knows which blobs it meant to remove, so it can record that intent durably instead of leaving a stray for something to discover later. The historical leak has no such record, so finding it does require a scan, but only once.

## Design

Two mechanisms, because there are two different problems: an ongoing one that should never need a human, and a historical one that is a single cleanup.

### 1. A deletion queue — the ongoing mechanism

Deleting bytes is a two-step operation across two stores (rows in SQLite, files on disk), and it cannot be atomic. Make the *intent* durable instead of trying to detect the failure afterwards:

- The transaction that removes an artifact, an asset generation, or a widget also inserts those blob ids into `pending_blob_deletions(blob_id, created_at)`. One transaction, so the intent is recorded exactly when the reference disappears.
- After commit, delete the files, then delete their queue rows.
- A crash anywhere leaves the queue rows in place. Drain the queue at startup and after each delete operation. `blob.Store.Delete` is already idempotent for a missing id ([[av-7jcq]]), so a re-run costs nothing and needs no compensating check.

**Why this is safer than a scan, not merely more convenient.** The queue only ever contains blob ids something already decided to delete. A bug in the drainer can therefore delete only condemned bytes. A full-scan reconciler infers deletability from the absence of a reference, so a bug in the *inference* — a table it forgot to join, a query that returns nothing under load — deletes live artifacts. That difference is why the ongoing path is automatic and the scan below is not.

It also costs nothing when idle: the queue is normally empty, versus a scan proportional to the whole blob store.

### 2. A one-time sweep — the historical mechanism

[[av-7jcq]] added `Delete` but never backfilled, so every artifact deleted before it shipped left its body on disk. Production is carrying those now and the queue cannot help: they were orphaned before anything recorded an intent to remove them.

`exhibit blobs sweep` reconciles the blob store against every table holding a blob id — `artifacts.source_blob_id`, `artifacts.widget_blob_id`, and the asset rows from [[av-20fk]] — and reports what has no referent. **Dry run is the default**; `--delete` is required to act. This one stays manual precisely because it *does* infer deletability, which is the failure mode described above.

**Ordering constraint:** enumerate blobs first, read rows second, and only consider a blob orphaned if it appeared in the first pass — so a blob written by a concurrent ingest is never a candidate.

**Scope:** reports and deletes, does not repair. A row pointing at a missing blob is the opposite failure; report it as a separate count since the scan has the information for free, but fix it elsewhere.

## Acceptance Criteria

- Deleting an artifact removes its body, widget, and asset blobs with no manual step and no scan.
- Killing the process between the delete transaction and the file unlink leaves the blob ids queued; the next startup removes the files. Asserted by a test that simulates the crash, since this is the only reason the queue exists.
- Draining the queue twice is harmless, and a queued id whose file is already gone drains successfully.
- The queue never contains a blob id that is still referenced by a live row — asserted directly, as it is the property that makes automatic draining safe.
- `exhibit blobs sweep` reports orphan count and total size and deletes nothing; `--delete` removes exactly those and no others, with artifacts, widgets, and assets still rendering afterwards.
- A blob created after the sweep began enumerating is never deleted.
- The sweep reports, without acting, rows whose blob is missing.
- Docs cover both: the queue as automatic and invisible, the sweep as a one-time historical cleanup that is safe to re-run.

## Notes

**2026-08-17T04:39:45Z**

Reworked: the ongoing case is a deletion queue (pending_blob_deletions written in the delete transaction, drained after commit and at startup), not a scan. Safer as well as automatic — the queue only holds already-condemned ids, so a drainer bug cannot reach a live artifact, which was the reason the scan had to be manual. The full scan survives only as a one-time backfill for pre-av-7jcq orphans.
