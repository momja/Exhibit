---
id: av-wrbu
status: open
deps: []
links: [av-4bzn, av-buyx, av-7k7b, av-q0ub, av-wmp6, av-v991]
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


## Notes

**2026-08-06T03:36:14Z**

Scope addition from av-q0ub: state values need ceilings too, and the policy above does not currently enumerate them (it covers artifact bodies, widgets, and URL fetches).

PUT /api/artifacts/:id/state accepts an unbounded value, and unlike a body or a widget it is written by UNTRUSTED ARTIFACT CODE: the storage shim's setItem is bridged through the host frame to this route, so an artifact can write as much as it likes, as often as it likes, without a person deciding anything. That makes it the one blob-ish input with no human in the loop, and the reason it belongs in this ticket rather than a later one.

Three ceilings to decide, not one -- they fail differently and cap different things:
  1. PER-KEY size. Bounds one value. Closest analogue to the existing body/widget question, and the one a 413 maps onto cleanly.
  2. PER-ARTIFACT total. Bounds one tool's whole store. Matters because reads are INLINED INTO THE RENDER DOCUMENT (architecture.md 3.2): every byte of state is paid on every render of the artifact AND of its widget, and a widget renders once per card, so per-artifact state multiplies across a gallery page exactly the way an oversized widget body does.
  3. PER-USER total. Bounds one principal across every artifact. av-q0ub keyed artifact_state by (artifact_id, user_id, key), so 'whose bytes are these' is now an answerable question and a per-user quota is expressible for the first time. This is the ceiling that matters once a non-owner may write state on someone else's artifact (av-7k7b) -- otherwise a visitor can grow the owner's database.

Question 3 (failure shape) needs an extra answer here that the other routes do not need: localStorage.setItem has a standard way to say 'full' -- it throws QuotaExceededError -- so the shim can translate a 413 into the exception artifacts are already written to handle. That is a better answer than a silent drop, which is what an unbounded route effectively promises today. Question 4 (fall back vs fail) also applies to inlining: state that will not read currently degrades to an empty cache so the artifact still renders (render.go), which is a deliberate choice worth folding into the stated rule.

Ceilings themselves are deliberately not proposed here -- that decision is this ticket's, which is why av-q0ub added state to its scope rather than inventing numbers of its own.
