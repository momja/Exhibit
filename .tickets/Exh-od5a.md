---
id: Exh-od5a
status: closed
deps: []
links: [Exh-bwya]
created: 2026-11-15T02:03:46Z
type: epic
priority: 2
assignee: Max Omdal
tags: [brand, ui]
---
# Brand refresh: painted sign-in, paper/dusk palette, type and grain

Restyle Exhibit around the hero painting as the source of truth: logo pigments in content, ink for controls. Paper ground (#f1e9d6) in light mode, dusk (#14182e) in dark. Schibsted Grotesk for text, Fraunces (WONK off) for one headline per page. Grainy default tiles in today's rounded cards. A painted sign-in page with a one-time descent animation.

Explored with throwaway mocks on the real UI. Paintings, layers, the descent prototype, mock CSS and reference screenshots live in design_files/brand-refresh/ (README maps files to stories; added in PR #281). Decisions: ink accent (#1f2a1c / cream #efe8d6 in dark), slate #2e5482 only for inline text links; meadow green rejected (not a logo colour); no ink outlines, square corners or inset mats on cards (too strong / too nested).

Out of scope: desire paths, wear marks, the 'Well trodden' rail, mats, meadow accent, a dusk CodeMirror theme. Exploratory work stays in Exh-4h60 and Exh-ajri.

## Acceptance Criteria

All child stories closed; each PR attaches light and dark screenshots of the pages it changes (not committed, per AGENTS.md).

