---
id: av-reo3
status: open
deps: []
links: [av-b17a, av-3pq6, av-20fk]
created: 2026-08-17T23:59:38Z
type: bug
priority: 0
assignee: Max Omdal
tags: [ingest, refetch, data-loss]
---
# Refetch overwrites an artifact in place, with no way back

`POST /api/artifacts/:id/refetch` replaces an artifact's stored body with whatever the source URL returns. Two distinct problems, and only the second is about content.

**1. It treats any response as the page.** `internal/api/artifacts.go` contains no `resp.StatusCode` check on either fetch path, so a 404 or 500 body is written over the artifact as if it were the new version. The server is saying "this is not the resource"; refetch stores the error page anyway. An empty body and a non-HTML `Content-Type` go the same way, and neither can be an artifact at all.

A total DNS or connection failure *is* handled — `http.Get` errors and the handler 400s before writing — which is why this is easy to miss. It fails safe only when nothing answers.

**2. The write is irreversible, and that is the deeper problem.** `Blob.Put(a.SourceBlobID, …)` overwrites the same blob id in place, and there is no version history ([[av-3pq6]]). Once it has run, the previous body does not exist anywhere.

That matters precisely because **the content itself cannot be judged**. If the source now serves a different page, that is what the source serves — "replaced by a parking page" and "redesigned" are the same observation from here, and sorting them apart would mean inferring intent from content, which this system deliberately does not do (PRD §8.1 already frames refetch as a snapshot update, not a curated one). So the answer to an unwanted update is not to predict it. It is to still have the old one.

Found while investigating a source domain that had been live a day earlier — the case refetch is most likely to be pointed at, since nobody refetches a source they know is dead.

## Design

**Reject what the protocol says is not the page.** Non-2xx, an empty body, and a non-HTML content type each end the refetch with the artifact untouched and the reason reported. These are read off the response, not inferred from it, which is what separates them from the heuristics below.

**Explicitly not doing content judgment.** No parking-page detection, no "this looks like a login wall", no refusing a body that shrank by 90%. A page that legitimately became smaller or simpler is a real update, and a check that blocks it is worse than the problem — it makes a working feature refuse valid input based on a guess. The earlier draft of this ticket proposed a shrinkage guard; it is dropped.

**Write to a new blob, then repoint — the actual fix.** [[av-8gyd]]'s deletion queue makes the safe shape cheap: store the fresh body under a *new* blob id, update the row to point at it, enqueue the old id. Nothing is destroyed until the replacement is durably in place, and a crash mid-refetch leaves the artifact on its old body rather than half a new one. This is the part that holds when the guards do not, which is most of the time.

**Version history is the complete answer** ([[av-3pq6]]). Since an unwanted update is indistinguishable from a wanted one, the only real protection is that the previous version still exists and can be restored. This ticket does not depend on it, but it is where the problem actually ends, and the new-blob-then-repoint shape above is a step toward it rather than away.

**Assets: the trap this ticket exists to flag.** Refetch does not currently touch `artifact_assets` ([[av-20fk]]), so a bad refetch corrupts the body and leaves the payloads intact. When [[av-b17a]] makes refetch run the vendorer, the obvious implementation calls `ReplaceArtifactAssets(…, [])` for a page with no assets — deleting every asset row, condemning every blob through the refcount, and draining the bytes immediately. That is strictly worse than today: the payload used to die inside the body blob it lived in, and would now be actively reclaimed. **A refetch that produced no assets must not be treated as a refetch whose assets were superseded.**

## Acceptance Criteria

- A refetch whose source returns any non-2xx leaves the artifact byte-identical and reports why.
- A refetch returning an empty body, or a non-HTML content type, does the same.
- A refetch whose source returns a *different but valid* page succeeds and replaces the body. That is the feature working, and a test says so, so nobody adds a content heuristic later.
- A successful refetch writes a new blob and repoints the row; the previous body is enqueued for deletion rather than overwritten in place.
- Killing the process mid-refetch leaves the artifact on its previous body.
- A refetch returning a page with no runtime assets does not delete the artifact's existing assets.

## Notes

**2026-08-18T01:12:37Z**

Narrowed after review. Dropped the content-based guards (parking-page detection, shrinkage refusal): a source that serves a different page IS serving a different page, and separating 'replaced' from 'redesigned' means inferring intent from content, which this system does not do. What survives is protocol-level (non-2xx, empty, non-HTML) plus the part that actually matters — the overwrite is in place and irreversible. Since an unwanted update cannot be distinguished from a wanted one, recoverability rather than prevention is the answer.
