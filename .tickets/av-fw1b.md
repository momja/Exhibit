---
id: av-fw1b
status: closed
deps: []
links: [av-20fk]
created: 2026-08-18T05:56:49Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-1in5
tags: [hosted, backend, storage, store]
---
# Per-owner storage accounting: record blob sizes and total them by owner

There is no size anywhere in the schema. Across all eighteen migrations no table records how many bytes an artifact is, and no query can answer how much an owner is holding — the only way to find out is to stat the blob directory, which knows nothing about owners.

Every limit the hosted version needs is a function of that number, so it comes first. It is also worth having on a self-hosted instance, where "what is actually using my disk" is a question with no answer today.

## Design

**Record bytes at the point they are written**, not by walking storage afterwards. A sweep over the blob store is O(library) and, on an object-store backend ([[av-52ll]]), a paginated network crawl — the wrong shape for a number read on every ingest. The writer knows the length; persist it there.

**Count every blob an owner causes to exist**, which is more than the body: the widget document (`artifacts.widget_blob_id`) and, once [[av-20fk]] lands, URL-addressed runtime assets. A snapshot's vendored wasm payload is the single largest thing the system stores, so an accounting that omits assets measures the wrong order of magnitude.

**Refcounted assets are the real design question.** [[av-20fk]]'s assets are shared and refcounted, so one blob can belong to several artifacts and potentially several owners. "How much does this owner use" then has two defensible answers — charge each referencing owner the full size, or divide it — and they diverge exactly when someone notices. Pick one, write down why, and make it a property of the query rather than of whoever calls it. Full-size-per-owner is the simpler and more defensible default: it is what the owner would have to store alone, it cannot be gamed by deduplication against another tenant's uploads, and it never reports a total that changes because a stranger deleted something.

**Recompute must be possible.** Incremental counters drift — a crash between blob write and row commit, a bug, a manual repair. Ship a recompute path that rebuilds an owner's total from the rows, so the number is correctable rather than authoritative-by-assumption.

**No enforcement here.** This ticket produces the number and nothing reads it as a limit; [[av-10bw]] does that. Landing them separately means the accounting can be verified against a real library before anything starts refusing uploads on its say-so.

## Acceptance Criteria

- Every stored blob's byte length is persisted when it is written — body, widget, and (when it lands) each runtime asset.
- A single query returns an owner's total stored bytes without touching the blob store.
- Deleting an artifact, detaching a widget, or dropping an asset reduces the total correspondingly; `DELETE /api/account` takes it to zero.
- A recompute path rebuilds an owner's total from rows and is idempotent.
- The chosen answer for refcounted shared assets is implemented in the query and its rationale is recorded in `architecture.md` §3.3.
- No request is refused as a result of this ticket.

