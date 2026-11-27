---
id: Exh-s8yu
status: closed
deps: []
links: []
created: 2026-11-16T03:29:55Z
type: feature
priority: 3
assignee: Max Omdal
tags: [ui, agent, brand]
---
# Boil the agent's working indicator

While the agent is thinking, the chat shows a rotating circle-notch spinner beside "thinking…" (spinnerIcon in web/gallery/agent.js; .thinking-spinner in agent.css). Replace that spinner with a hand-inked Phosphor pencil-simple that boils: three slightly different drawings of the pencil cycled at 8 fps, the classic hand-drawn line boil.

Why this spot: the agent is literally drawing the user's tool, and its turns run from seconds to minutes. Boil says "alive" without implying progress, a spinner says "mechanical wait". This is the only boiling icon for now; static navigation, card actions and the detail pill stay still.

## Design

Frames are baked at build time, never generated in the browser and never shipped as raster (.webp/.gif): raster cannot follow currentColor or dark mode and blurs between pixel densities.

Bake (web/icons build step, from the pinned Phosphor regular pencil-simple.svg, 256-unit viewBox):
1. Split the path into contours and sample each every 2 units.
2. Drift: move each point by 9 × smooth 2D noise at wavelength 90 (separate seeded fields for x and y).
3. Pressure: offset each point along its outline normal by 3 × noise at wavelength 45. Holes wind opposite to outer contours, so strokes swell and thin like ink.
4. Repeat with three fixed seeds, so output is deterministic and every build produces identical bytes.
5. Simplify with Ramer–Douglas–Peucker at 0.3 units and write relative path data at one decimal. Prototype result: 2.9 KB for all three frames, 1.3 KB gzipped.
Generated output goes into the embedded assets like the rest of the icon build, not committed.

Markup: one inline SVG, fill=currentColor, three <path class="f fN"> frames. Frame 1 is visible by default. Under .is-boiling, each frame runs a 375 ms steps(1) keyframe (visible for the first third) with negative animation-delays of 0, -250 and -125 ms, so exactly one frame shows at a time. Inline, not <img>, so currentColor follows the theme. The server can render it into a <template> that agent.js clones, the way agent-message-template already works, so the JS holds no path data.

Reduced motion: keep boiling, matching av-90ej's reasoning for the spinner (a stopped indicator reads as a frozen turn, and a 13px in-place change is not the motion Reduce Motion targets). Revisit only if the frame flicker proves worse than rotation for someone using the setting.

Prototype and tuning: the Icon Wobble artifact in the Exhibit library (vector mode, Medium + pressure defaults).

Out of scope: in-progress tool chips (static gear), static wobble for other icons, the 404 logo.

## Acceptance Criteria

The thinking indicator shows a boiling pencil instead of the rotating circle-notch; the "thinking…" text and when the indicator appears and disappears are unchanged.
Frames come from a deterministic build step from the pinned Phosphor pencil (rerunning the build produces identical output); no raster files and no client-side generation.
The icon takes the text colour in light and dark modes and stays crisp at 1x, 2x and 3x.
Exactly one frame is visible at any moment (no blank or doubled frames), checked in Chromium and WebKit.
Behaviour under prefers-reduced-motion follows av-90ej (keeps boiling), with the reasoning in an agent.css comment that replaces the spinner one.
Baked asset adds 2 KB gzipped or less.
The av-c7hu rotation workaround (CIRCLE_NOTCH_PATH, the thinking-spin keyframes) is removed if nothing else uses it.
Screenshots or a short screen recording of the indicator attached to the PR, light and dark.


## Notes

**2026-11-16T08:45:52Z**

Shipped as a filmstrip, not three visibility animations: the three frames sit one 256-unit cell apart behind a one-cell viewBox, and one steps(3) translate animation slides the strip. The ticket's per-frame scheme leaves a ~1.25µs gap at each hand-off (the 33.333% keyframe is short of 1/3), and WebKit 27.2 rendered blank frames there (2 of ~24 two-second runs). The noise permutation uses a Fisher-Yates shuffle instead of the prototype's random-comparator sort, so frames differ from /tmp/bake but keep its character. Baked asset: 2479 bytes raw, 1095 gzipped (level 9).
