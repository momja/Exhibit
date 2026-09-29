---
id: av-uvc6
status: closed
deps: []
links: []
created: 2026-09-29T05:12:59Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, gallery]
---
# Tighten the gallery into the 01-tight card grid

The gallery reads sparse: a 1200px column of rounded cards, a full-width search row above it, and a card body of title, tag pills, created date, share badge and a "Sandboxed" label under every tile. The density exploration on the paper canvas (frame 01-tight) is the conservative fix: same tile, much less chrome around it.

Apply 01-tight to the gallery page:

- Search moves into the app bar beside the logo. The grid runs the full width of the window (minmax 226px columns, 6px gaps), cards are white with a 1px square-ish border and no shadow.
- Each card is the 132px tile plus one 26px meta row: title, tag dots, the posture glyphs, and an edit pencil that shows on hover or focus.
- No created date and no "Sandboxed" label on the card. A sandboxed private artifact carries no marks at all; absence is the signal, as it already is for sharing (spec 7).
- Tags render as color dots only. The tag name stays reachable as a tooltip and as screen-reader text.
- The share badge's text leaves the card. Its glyph joins the capability glyphs in the one trigger, so sharing stays ambient, and the posture popover gains a Sharing section carrying the full sentence the badge's title used to hold. The public-link glyph becomes a link, since a globe in that cluster already means network origins.
- Hovering a popover trigger paints a gray pill behind its glyphs, so it reads as something to click.

The detail page's toolbar cluster keeps its current form ("Sandboxed" label, no sharing section).

## Acceptance Criteria

- Gallery grid, card and header match frame 01-tight at 1440px, and the page still fits a 390px phone with no horizontal scroll.
- Card meta row: title, tag dots with names as title + sr-only text, capability + share glyphs in one popover trigger, hover/focus pencil linking to the edit page (always visible on touch).
- The popover shows a Sharing section only when the artifact is shared; a sandboxed private artifact renders no trigger on the card.
- The detail toolbar's cluster is unchanged apart from the pill hover.
- Go template tests and the node page-script suite pass; docs (spec 7, architecture 3.5) describe the badge as it now renders.

