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
# Orphaned blob sweep: an operator command that reconciles the blob store against the rows

There is no way to find or reclaim blobs on disk that no row points at, and there are certainly some.

[[av-7jcq]] gave `blob.Store` a `Delete` and wired it into the deletion paths, but it did not backfill: every artifact deleted *before* that shipped left its body on the filesystem forever, exactly as architecture.md §3.1 had accepted for v1. Any deployment predating av-7jcq — production included — is carrying those bytes now, and nothing in the product can tell you how many or remove them.

[[av-20fk]] adds a second, ongoing source. It deletes superseded asset blobs inline, after the transaction commits, so a crash in that window leaves unreferenced bytes behind. Rare, small, and by design: the alternative ordering leaves rows pointing at blobs that are already gone, which is worse.

Both are the same question, and it is one of the few questions here that is actually decidable — a blob either has a row pointing at it or it does not. That is what makes a sweep safe, where GC based on artifact *content* would not be ([[av-20fk]] design).

## Design

A subcommand on the existing binary — `exhibit blobs sweep` — reconciling the blob store against every table that references a blob id: `artifacts.source_blob_id`, `artifacts.widget_blob_id`, and the asset rows from [[av-20fk]]. Anything on disk with no referent is reported.

**Dry run is the default.** It prints what it would delete — count, total bytes, ids — and deletes nothing. `--delete` is required to act. This is a command whose bug deletes user artifacts, so the safe mode is the one you get by not thinking.

**It is not hooked into anything.** Not per artifact change (it is a full scan of the blob store against the rows — absurd to run on an edit), and not at startup. Automatic reclaim at boot means the operation with the worst failure mode in the product runs unattended, on a schedule nobody chose, while the operator is not watching; leaked bytes from a rare crash window are cheap by comparison. An operator who wants it periodic can cron the command, which is the same posture as Litestream and TLS being theirs to compose (technical_stack.md §12).

**Ordering constraint.** Enumerate blobs *first*, then read the rows, and only consider a blob orphaned if it was present in the first pass. A blob written by an ingest running concurrently with the sweep must never be a deletion candidate. Alternatively, skip blobs whose mtime is within a generous window; state which was chosen and why.

**Scope.** Reports and deletes; it does not repair. A row pointing at a *missing* blob is the opposite failure and belongs to whatever surfaces broken artifacts — worth reporting as a separate count, since the scan has the information for free, but not fixing here.

## Acceptance Criteria

- `exhibit blobs sweep` on a store with orphaned blobs reports their count and total size and deletes nothing.
- `exhibit blobs sweep --delete` removes exactly those blobs and no others; artifacts, widgets, and assets still render afterwards, asserted by a test over a store containing all three kinds.
- A blob created after the sweep began enumerating is never deleted.
- The command reports, separately and without acting, rows whose blob is missing.
- Running it against a store with no orphans reports zero and exits successfully.
- Docs cover it under operations, including that it is manual by design and safe to cron.

