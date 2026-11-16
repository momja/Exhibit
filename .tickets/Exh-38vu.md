---
id: Exh-38vu
status: in_progress
deps: []
links: [Exh-lmor]
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

## The banner, as decided

A bar across the top of the share page: the Exhibit mark at icon size, the
words "made with exhibit", linked to `https://exhibit.dizzard.net`. That
placement is settled — the product owner is not choosing it, and neither is the
implementer. Exh-u1t3 builds it; independent reviewer subagents judge the
result and the iteration continues until they pass. Everything below is the
flag, the per-artifact control and the safety envelope around that bar.

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
3. **The banner itself** on `/s/:shareID` only — top strip, icon-size mark,
   "made with exhibit", linking to `https://exhibit.dizzard.net`. Not on
   `/a/:id`, `/w/:id`, version views, widget documents, or direct-open pages —
   those are framed by our own gallery, which already carries the brand.
4. **The safety envelope around the bar.** The share document is the artifact's
   own HTML in an opaque-origin sandbox, and a share can be framed by a
   third-party blog, so the display still has consequences to settle — how the
   link behaves in a frame, what a `100vh` tool does under a top strip, and how
   far the bar resists an artifact's own script. Exh-u1t3 settles them by
   building the decided default and iterating against reviewer verdicts, not by
   asking the owner to pick a placement.

## What must be true when it ships

- The banner cannot be removed, hidden, or restyled by the artifact's own
  script or CSS beyond the guarantee Exh-u1t3 states and justifies, and cannot
  break the tool it decorates.
- It never steals clicks, focus, or keyboard navigation from the artifact, and
  a full-viewport tool is still usable.
- It reads correctly in light and dark, at 320 px through desktop, and is
  screen-reader sane.
- It adds no network request, no tracker, and no third-party script.
- It is honest: the link goes to the public product origin, and the copy does
  not imply the artifact was authored by us.
- Self-hosted instances that never set the flag see byte-identical share pages
  to today (a pinned test, like the 404 page's).
- The link preview (`og:*` tags and `/s/:id/og.png`) is unchanged: the picture
  stays a picture of the widget and does not become a logo card.

## Sequencing

Exh-u1t3 goes first: it builds the decided banner and gets it through review,
and the flag, the per-artifact column and the API controls follow from what it
learns. The brand mark comes from the brand refresh epic (Exh-od5a) — reuse it,
do not invent a second wordmark.

## Acceptance criteria

1. With the flag off, share pages and their headers are byte-identical to
   today, pinned by test.
2. With the flag on, every share page of an artifact left on *inherit* shows
   the banner; *hide* removes it; *show* adds it even on an instance with the
   flag off (owner's explicit choice wins).
3. The banner is present in the server-rendered document (not added by JS),
   shows the icon-size mark, the text "made with exhibit" and links to
   `https://exhibit.dizzard.net`.
4. Displayed correctly: light/dark, 320 px and desktop, keyboard, screen
   reader, and inside a third-party iframe under `EMBED_ORIGINS`.
5. Documented in `docs/deployment.md` and `docs/security.md` (what it does and
   does not reveal about a share).

