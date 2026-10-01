---
id: av-c7hu
status: in_progress
deps: []
links: []
created: 2026-10-01T03:13:09Z
type: bug
priority: 2
assignee: Max Omdal
tags: [ui, agent]
---
# Agent thinking spinner wobbles as it rotates

The thinking indicator on the agent page (`.thinking i`, a `ph-circle-notch` webfont glyph, agent.js) wobbles while it spins: the ring drifts around a small circle instead of turning in place.

The glyph itself is centered. In font units its ring center is (512, 448), the exact center of the 1024-unit em box (ascent 960, descent 64). The offset comes from rasterization: the browser draws the glyph with its baseline snapped to whole device pixels, and the composited layer then rotates about the box center, which the snapped glyph no longer shares. Measured in Chromium and WebKit at DPR 1-3, the ring center sits 0.13-0.21 CSS px above the pivot, a 0.27-0.42 px wobble.

## Design

Draw the spinner as an inline SVG of the same Phosphor icon (the canonical 256-grid circle-notch path, ring centered at 128,128) and rotate the svg element. Vector paths are not baseline-snapped, so the pivot and the ring center coincide by construction. Checked against the vendored glyph: rendered at 1024px the two shapes differ only on edge pixels (true arcs vs the font quadratics). Size it at 13px to match today, fill currentColor.

## Acceptance Criteria

- The ring center and the rotation pivot coincide to within the renderer noise floor (the same measurement on a shape centered by construction) in Chromium and WebKit at DPR 1, 2 and 3.
- Same size, color and alignment as the current icon.
- No CDN reference; the icon stays Phosphor geometry.
- ph-circle-notch leaves icons.txt if nothing else uses it.

