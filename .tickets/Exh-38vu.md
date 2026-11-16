---
id: Exh-38vu
status: open
deps: []
links: []
created: 2026-11-16T06:04:24Z
type: epic
priority: 2
assignee: Max Omdal
tags: [ui, render, shares, hosting, branding]
---
# Attribution on share pages: "Made with Exhibit" behind an instance flag

A share link (`/s/:shareID`) is the only page a stranger opens with no
credential, and today it is deliberately unbranded: no footer, no wordmark, no
link, and a favicon that is a monogram of the artifact's own title. That is
right for a self-hosted tool where the artifact is the user's file, and wrong
for a hosted instance, where every share a user sends is distribution and
currently says nothing about the product that is serving it.

This epic adds an optional, unobtrusive attribution line to share pages. It is
**off unless an instance turns it on**, and the artifact's owner can opt out
per artifact even then.

## Scope

1. **Instance flag**, default off — self-hosted and hosted instances both ship
   with it off; turning it on is an explicit operator decision. Parsed with the
   same fail-closed conventions as the other flags (`envBool` in
   `internal/api/publicmode.go` is the model), and documented in
   `docs/deployment.md` next to `EMBED_ORIGINS`.
2. **Per-artifact control**, tri-state, defaulting to the instance setting:
   *inherit* (default) / *show* / *hide*. Stored on the artifact, surfaced in
   the share panel where the owner makes the decision the badge depends on, and
   exposed through the API with validation in the handler and the store (the
   `share_state_mode` pattern in `internal/api/artifacts.go` + `store.UpdateArtifact`
   is the precedent).
3. **The badge itself** on `/s/:shareID` only. Not on `/a/:id`, `/w/:id`,
   version views, widget documents, or direct-open pages — those are framed by
   our own gallery, which already carries the brand.
4. **A way to settle how it is displayed before building it.** The share
   document is the artifact's own HTML in an opaque-origin sandbox, and a share
   can be framed by a third-party blog, so "just put a div at the bottom" is
   not a design — it is a decision with security and layout consequences. That
   question is a spike, and it is the first child of this epic.

## What must be true when it ships

- The badge cannot be removed, hidden, or restyled by the artifact's own script
  or CSS, and cannot break the tool it decorates.
- It never steals clicks, focus, or keyboard navigation from the artifact, and
  a full-viewport tool is still usable.
- It reads correctly in light and dark, at 320 px through desktop, and is
  screen-reader sane.
- It adds no network request, no tracker, and no third-party script.
- It is honest: the link goes to the public product origin, and the copy does
  not imply the artifact was authored by us.
- Self-hosted instances that never set the flag see byte-identical share pages
  to today (a pinned test, like the 404 page's).
- The link preview (`og:*` tags and `/s/:id/og.png`) is unchanged unless the
  spike decides the picture should carry the mark — that is its call to make,
  and it must not become a logo card instead of a picture of the widget.

## Sequencing

Child 1 is the spike. Implementation children are filed from its findings
rather than guessed now. The brand mark and its usage rules come from the brand
refresh epic (Exh-od5a) — reuse, do not invent a second wordmark.

## Acceptance criteria

1. With the flag off, share pages and their headers are byte-identical to
   today, pinned by test.
2. With the flag on, every share page of an artifact left on *inherit* shows
   the badge; *hide* removes it; *show* adds it even on an instance with the
   flag off (owner's explicit choice wins).
3. Badge is present in the server-rendered document (not added by JS), and the
   artifact cannot remove or occlude it.
4. Displayed correctly: light/dark, 320 px and desktop, keyboard, screen
   reader, and inside a third-party iframe under `EMBED_ORIGINS`.
5. Documented in `docs/deployment.md` and `docs/security.md` (what it does and
   does not reveal about a share).


## Notes

**2026-11-16T21:02:06Z**

Implementation uses a separate opaque-origin srcdoc child below a 36px top bar, rather than a shared-DOM node or mutation observer. Child DOM isolation protects the bar; arbitrary hostile tools can still navigate/blank themselves, exhaust resources, or mislead viewers; third-party framers can crop/cover the share. Cost: browsing context, escaped HTML bytes, extra focus boundary and top-level API compatibility; owners can hide it. Default remains off, explicit show/hide wins. Only live /s/:shareID consults attribution. Chromium native-anchor measurements show popup/top navigation blocked and self navigation replacing embedded tool, so framed attribution deliberately loses href; no sandbox permission is loosened. Evidence: /tmp/exhibit-review/banner/. Independent final-head visual and code/security reviews are parent-owned and still required; implementer does not merge.
