---
id: Exh-ivww
status: in_progress
deps: []
links: []
created: 2026-11-17T05:16:56Z
type: bug
priority: 1
assignee: Max
tags: [gallery, frontend, perf, htmx]
---
# Library tab switches rebuild every card and widget frame

Switching library spaces (Starred / Search / Shared / Activity) replaces #gallery-content with outerHTML, so every switch destroys and rebuilds all 24 cards, one action-menu controller per card, and one live render-origin iframe per artifact that has a widget tile.

Measured on a seeded 24-card library, 390x844, CPU throttled 6x (phone-ish), on main:
- server render 2-26 ms; fragment 76 KB (~20 KB gzipped)
- DOM insert of the fragment: 890 ms with widget iframes, 71 ms without
- click -> first paint: 1301 ms with widgets, 470 ms without

So ~65-70% of the perceived delay is constructing 24 iframe elements (loading=lazy stops the fetches, not the browsing contexts), paid after the response arrives. Server-side caching would recover ~50 ms of a ~1300 ms click.

## Acceptance Criteria

A space switch paints its cards without constructing an iframe per card; frames are created only for cards near the viewport. Click -> first paint on the 24-card 6x-CPU fixture drops from ~1300 ms to ~500 ms. Widget behaviour is unchanged: offscreen frames unload, re-entry reloads through the owner-scoped redirect, health/deadline watch still attaches, and the edit-page preview fragment keeps its own lifecycle. Existing gallery, widget, and navigation browser tests pass or are updated to the new creation timing.


## Notes

**2026-11-17T06:12:07Z**

Implemented on bug/Exh-ivww/tab-switch-speed (9403a39a, da4f483b) with four seeded demo builds for review:
  8091 base (main) / 8092 +frames / 8093 +frames+pending / 8094 +frames+pending+cache (demo-only branch feature/Exh-ivww/fragment-cache-demo).

Measured on the seeded 24-card library (18 with widget tiles), 390x844, CPU 6x slower, fixed insert workload (fetch the same 68 KB fragment, parse, insert):
  base   18 live iframes  insert 218 ms  tab acknowledges: never
  frames  0 live, 18 inert in <template>  insert 27 ms  tab: never
  pending 0 / 18  insert 30 ms  tab acknowledges in 25-95 ms
  cache   0 / 18  insert 28 ms  tab 25-95 ms, and a revisit costs 0 requests (8 ms)
Server render was 2-26 ms in every build; none of this is Go.

The widget frame now ships inside a <template> and gallery-widgets.js clones it in when the card nears the viewport (the moment loading="lazy" waited on). Nothing else about the frame lifecycle changed: the 400px unloading, the reload redirect that mints a fresh credential, the health deadline and the edit/agent preview's inline frame are all as they were.

The pending state marks the tab aria-current at click time and dims the outgoing results (after 150 ms, so a fast switch never flickers); a failed switch restores it. Only deliberate tab clicks anticipate: history traversal and retries keep the old behaviour.

Caching is rejected rather than pending: galleryspaces_browser_test.go asserts that a share granted on the server appears on the next visit, and an eight-second fragment cache makes it appear up to eight seconds late. It is built on its own branch so the trade can be felt; two browser tests fail there on purpose.
