---
id: Exh-b6r6
status: in_progress
deps: []
links: []
created: 2026-11-08T19:22:06Z
type: bug
priority: 1
assignee: Max Omdal
tags: [frontend, mobile, ui]
---
# Phone app header rides down with the page when iOS overscrolls

On an iPhone (home-screen app), pulling the gallery down at the top limit drags the app header (logo, Add, account) down with the page, leaving a band of page background above it (screenshot 2026-11-06, Starred).

Cause: the standard header is position:sticky in the document flow, and a sticky box is held in content coordinates, so the rubber-band movement of the content carries the bar with it. The phone space tabs were pinned for the same class of problem in Exh-t9bv; the header was left sticky.

## Design

Mirror Exh-t9bv: on phones the header becomes position:fixed against the viewport, and the space it no longer occupies in the flow is held by a slot that wraps it (templates/components.tmpl appHeader). .app-header-slot is display:contents on desktop so the bar stays an in-flow child of the page exactly as today; inside the phone breakpoint the slot is a block whose min-height is the header's measured border box (the top safe-area inset is padding) published as --app-header-height by a small app-header Stimulus controller, with calc(--header-height + safe-area) as the CSS fallback for the frame before it measures and for pages that ship no scripts. The floating viewer header is unaffected (it is already fixed, in its own template). scroll-chrome's data-chrome-hidden transform keeps working: it is still a transform on the bar itself.

## Acceptance Criteria

- At phone widths the standard app header stays glued to the top of the viewport while the page is overscrolled past either end.
- Page content below starts at the header's rendered height: no overlap, no doubled gap, at 320-430px, with safe-area insets, and where the bar is a flex item (agent, login).
- Desktop keeps position:sticky; the slot has no box there.
- Gallery phone chrome (hide on scroll down, reveal on scroll up, focus reveal) unchanged.
- Browser test asserts the header is fixed at phone widths, occupies no layout space of its own, and that content clears it; existing header tests still pass at 1280-320px.

