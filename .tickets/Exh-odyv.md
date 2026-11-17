---
id: Exh-odyv
status: closed
deps: [Exh-z8i2, Exh-wjqh]
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# UI polish pass for the new palette and type

Apply the new look across the app: card shadow and hairline values, selected rail item tint, title weights, one Fraunces headline per page on every gallery tab, settings, edit and 404. Buttons use the medium radius. Coordinate with in-flight card/header work (Exh-geph, Exh-e736, Exh-91qf) and start after they merge.

## Acceptance Criteria

All gallery tabs, detail, edit, settings, admin and 404 reviewed in light and dark on desktop and phone; screenshots attached to the PR.


## Notes

**2026-11-17T07:12:45Z**

One font-headline per page: gallery tabs (h1 in #gallery-content, now visible), edit, settings, admin, add, 404, detail pill info. Cards: border removed, shadow-card only (dark ring now white 6%/10% alpha as in soft-round mock). Buttons on --radius-md (agent/detail literal radii tokenised). Fraunces request unchanged (600 only). Rail tint unchanged. No contrast tokens changed.
