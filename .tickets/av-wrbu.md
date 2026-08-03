---
id: av-wrbu
status: open
deps: []
links: [av-4bzn]
created: 2026-08-01T18:49:39Z
type: task
priority: 2
assignee: Max Omdal
tags: [api, design]
---
# Define the service's policy for oversized and degenerate request bodies

There is no size cap on any body the API accepts directly. POST /api/artifacts and PATCH /api/artifacts/:id take an unbounded pasted body; PUT /api/artifacts/:id/widget (av-fafu) does too — an 8 MB widget is accepted and served back verbatim, measured. Only the two URL-fetch paths are bounded (io.LimitReader at 10 MiB in createArtifact and refetchArtifact), and that limit was chosen for fetching, not as a policy.

The widget case is what surfaced this, and it is the sharpest instance rather than a separate problem: an artifact body renders once, on demand, but a widget body renders once PER CARD, so the same oversized document multiplies across a gallery page.

This ticket is to decide the policy before implementing one, because the same question has other unanswered instances in the service:
  - an unresolved/failed outbound fetch during URL ingest — currently a 400, no retry, no partial
  - a blob the store cannot read at render time — currently a 404 for the artifact, silently 'no widget' on the edit page (av-fafu, widgetSource)
  - a request body that is well-formed but degenerate (empty document, fragment rather than a document, non-HTML)
  - the render surface hitting a missing/corrupt blob for an artifact the DB still lists

## Design

Open questions to answer, not a proposed implementation:
  1. Is there ONE ceiling for anything stored as a blob, or per-route ceilings? A single documented limit is easier to reason about and to state in docs; per-route lets the widget be much smaller than an artifact, which is where the real risk is.
  2. Where is it enforced — http.MaxBytesReader at the middleware seam (uniform, covers routes nobody remembers to update) or per-handler (explicit, greppable)?
  3. What is the failure shape? 413 with a JSON error is the obvious answer for the API, but the gallery/agent/edit clients each need to render it, and the agent extension needs to turn it into something the model can act on rather than retry blindly.
  4. Do we FALL BACK or FAIL for unresolved reads? The service currently does both in different places without a stated rule. A blob that will not read is arguably a 404; a widget blob that will not read is arguably 'no widget' (which is what av-fafu does) — but those two answers are inconsistent and neither was chosen deliberately.
  5. Is degenerate-but-small input (a fragment, non-HTML) rejected at ingest or accepted as data? Current behaviour is accept, and the render preamble's injection already tolerates a document with no <head>.

## Acceptance Criteria

A short written policy in docs (likely security.md or architecture.md) covering: the size ceiling(s) and where enforced, the error shape clients can rely on, and the fallback-vs-fail rule for unresolved reads. Implementation follows in whatever tickets the policy implies.

