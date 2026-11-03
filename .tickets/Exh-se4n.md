---
id: Exh-se4n
status: in_progress
deps: [Exh-t9bv]
links: []
created: 2026-11-03T04:46:40Z
type: feature
priority: 2
assignee: Max Omdal
tags: [gallery, frontend, mobile]
---
# Hide phone gallery controls while scrolling down

On phones the gallery keeps two bars on screen at all times: the sticky app header (logo, search, filter, account) and the fixed Library / Shared with me / Shared by me tabs at the bottom. Together they take a large share of a small screen while the user is browsing cards.

Hide both while the user scrolls down through the gallery, and bring them back as soon as they scroll up, the way mobile browsers treat their own toolbars.

## Design

One Stimulus controller on #gallery-shell, which is stable across space switches, sets data-chrome-hidden; CSS inside the phone breakpoint translates the header up and the tabs down. Transform only, so layout and scroll position never move. The direction decision is a pure function with a small tolerance so jitter does not flicker the bars. The scroll position is clamped to the scrollable range so iOS rubber-band overscroll is not read as a scroll up. Stacked on Exh-t9bv, which makes the tabs position:fixed.

## Acceptance Criteria

- At phone widths (<=767px), scrolling down past the header hides the header and the space tabs; scrolling up any meaningful distance reveals both.
- Near the top of the page both are always shown. A page too short to scroll never hides them.
- iOS overscroll at either end does not toggle them.
- Focus entering the header or tabs (keyboard, the / shortcut) reveals them; they do not hide while a field in them is being typed in or has keyboard focus.
- Desktop rail and header unchanged. Reduced motion: no slide animation.
- Space switches keep working; the state survives the htmx swap of the tabs.
- Unit tests for the decision function and a browser test at a phone viewport.

