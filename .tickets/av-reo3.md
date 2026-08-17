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
# Refetch destroys an artifact when the source site is gone but still answers

`POST /api/artifacts/:id/refetch` overwrites an artifact's stored body with whatever the source URL returns, and checks only whether the HTTP *call* errored — never what came back. `internal/api/artifacts.go` contains no `resp.StatusCode` check on either fetch path.

So a site that has gone away but still answers replaces a working artifact with garbage:

| Source now returns | Result |
|---|---|
| Registrar parking page after the domain expires (200) | Artifact becomes the parking page |
| Cloudflare / nginx error page (5xx) | Artifact becomes the error page |
| A login wall (200) | Artifact becomes the login form |
| An empty 200 | Artifact becomes empty |

A total DNS or connection failure *is* handled — `http.Get` returns an error and the handler 400s before writing — which is why this is easy to miss. It fails safe only in the case where nothing answers at all.

The loss is unrecoverable. `Blob.Put(a.SourceBlobID, …)` writes over the same blob id in place, there is no version history ([[av-3pq6]]), and nothing prompts before it happens. Found while investigating a source domain that had been live a day earlier — the exact scenario refetch is most likely to be pointed at.

## Design

**Refuse before writing.** Non-2xx, an empty body, and a `Content-Type` that is not HTML should each end the refetch with the artifact untouched and the reason reported. These are cheap and cover every row in the table above.

**Write to a new blob, then repoint.** The current in-place overwrite is what makes any failure unrecoverable, and [[av-8gyd]]'s deletion queue now makes the safe shape cheap: store the fresh body under a *new* blob id, update the row to point at it, and enqueue the old id. Nothing is destroyed until the replacement is durably in place, and a crash mid-way leaves the artifact on its old body rather than on half a new one. This is worth doing even with the guards above, because the guards are heuristics and this is structural.

**A shrinkage warning, not a refusal.** A body a small fraction of its previous size is a strong signal the source has been replaced by something that is not the tool — but it is a signal, not a fact, and a genuine rewrite can legitimately shrink. Surface it and let the user decide rather than blocking.

**Assets: the trap this ticket exists to flag.** Refetch does not currently touch `artifact_assets` ([[av-20fk]]), so a bad refetch corrupts the body and leaves the payloads intact. When [[av-b17a]] makes refetch run the vendorer, the obvious implementation calls `ReplaceArtifactAssets(…, [])` for a page with no assets — which deletes every asset row, condemns every blob through the refcount, and drains the bytes immediately. That is strictly worse than today: the payload used to die inside the body blob it lived in, and would now be actively reclaimed. **A refetch that produced no assets must not be treated as a refetch whose assets were superseded.** Distinguish "the new page genuinely has none" from "the fetch did not return the page", and when in doubt keep them.

**Not solved by version history alone.** [[av-3pq6]] would make this recoverable, which is worth having, but recovering from an avoidable overwrite is worse than not performing it.

## Acceptance Criteria

- A refetch whose source returns 404, 500, or any non-2xx leaves the artifact byte-identical and reports why.
- A refetch returning an empty body, or a non-HTML content type, does the same.
- A successful refetch writes a new blob and repoints the row; the previous body is enqueued for deletion rather than overwritten in place.
- Killing the process mid-refetch leaves the artifact on its previous body.
- A refetch that returns a page with no runtime assets does not delete the artifact's existing assets.
- Each of the four failure rows in the description is covered by a test — this is a data-loss path, so the guards need to be pinned, not just present.

