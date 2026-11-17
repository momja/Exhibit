---
id: Exh-z8i2
status: closed
deps: [Exh-plvh, Exh-geph, Exh-e736, Exh-91qf]
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Grainy default tiles in the palette

Snap default tile hues (--tile-hue, derived from the artifact id in Go) to a small set of palette colours. Add the grain overlay (SVG feTurbulence noise as a data URI, mix-blend-mode overlay) as a shared utility the sign-in page can reuse. Keep today's rounded card: 10px corners, full-bleed tile, soft shadow, no outlines, no inset mat. Monogram plate: 14px radius, no outline, soft tinted shadow.

## Acceptance Criteria

Default tiles and monograms use palette hues with grain in light and dark; widget tiles unaffected; gallery screenshots attached to the PR.


## Notes

**2026-11-17T03:04:41Z**

Palette hues 8,40,96,136,214,231 (tile.Palette). Monogram contrast AA: light min 6.42, dark min 4.53 (TestTilePaletteKeepsTextContrast). Grain sits under the opaque plate so it cannot lower contrast; hidden in forced-colors. Standalone share tile CSP gained img-src data:.
