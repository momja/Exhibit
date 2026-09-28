---
id: av-3tib
status: open
deps: []
links: [av-fafu]
created: 2026-07-07T06:08:28Z
type: task
priority: 3
assignee: Max Omdal
---
# Gallery card tile — snapshot thumbnails (refreshed: widgets own the slot)

Original plan (2026-07-07): snapshot each artifact and render it as a thumbnail
in its gallery card beneath its title. Stale in both halves since av-fafu shipped:

- The slot belongs to the `cardWidget` tile: live widget frame
  (`RENDER_ORIGIN/w/:id`, pointer-events none) when the artifact has one,
  server-rendered default tile (monogram on an id-derived tint,
  `internal/tile`) when it doesn't. Full design: `docs/widgets.md`.
- The tile *leads* the card (132px well above the title), not beneath it.
- No snapshot pipeline was ever built. `docs/technical_stack.md` §8 defers
  headless-Chromium screenshots as optional; `docs/architecture.md` explicitly
  chose markup over a screenshot pipeline for the no-widget case ("a gallery
  of forty cards must not pay forty frame loads (or a screenshot pipeline) to
  say 'nothing to show'").

## Design (remaining scope, if kept)

- Precedence if snapshots ever exist: widget > snapshot > default tile. Never
  a second tile beside the widget.
- The version-history thumbnail in av-3pq6 is optional garnish — that ticket
  is unblocked from this one (dep removed there).

## Acceptance Criteria

- Either close as superseded by av-fafu (slot shipped; monogram is the
  no-widget face), or build the auto-snapshot fallback for widget-less cards:
  headless-Chromium worker per technical_stack §8, snapshot refresh policy,
  placeholder-while-rendering, ops cost documented.
- Until then the gallery slot is done: widget or monogram, one `cardWidget`
  partial everywhere.


## Notes

**2026-09-28T16:06:00Z**

Refreshed 2026-09-28: slot now owned by widgets (av-fafu); snapshot pipeline optional, precedence widget > snapshot > monogram; av-3pq6 unblocked.
