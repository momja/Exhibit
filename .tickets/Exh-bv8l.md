---
id: Exh-bv8l
status: in_progress
deps: []
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Logo cleanup: flat mark without hanger, tidied hanging variant

Split internal/api/logo.svg into two artworks.

Flat mark (default): the frame only, no wire, nail or drop shadow, tighter viewBox. Used in the header, sign-in and Clerk pages, admin, app and render-origin favicons, PWA/maskable icons and the logo OG card.

Hanging variant: kept where the picture-on-a-wall metaphor helps, currently the 404 hero (notfound.tmpl, via its own data URI; two inline copies on one page would clash on gradient/filter ids, see logo.go).

Hanger problems measured in the current SVG:
- Off-centre: frame centre x≈55.5, nail 56.1, wire apex 56.3; left leg lands 4.6 left of centre, right leg 5.7 right, near the red/white join.
- Three light directions: frame shadow (+1.4,+1.4), wire shadow (+2.2,+0.8), nail shadow (+2.5,+1.7).
- Nail shadow is a blurred grey circle that reads as a smudge, and as a glow on dark grounds.
- Unused gradient definitions remain.

## Acceptance Criteria

Nail, wire apex and frame share one axis; legs land symmetrically on the frame's outer edge; wire stroke matches the frame stroke; one shadow direction (or none on the wire); dark-mode wire/nail lightening retained; unused defs removed. Flat mark used everywhere except the 404 hero. PWA icons and OG logo card regenerated.

