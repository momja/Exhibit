---
id: egt-56hv
status: in_progress
deps: []
links: []
created: 2026-11-22T22:03:58Z
type: bug
priority: 2
assignee: Claude Sonnet 5.5
parent: Exh-od5a
tags: [ui, brand]
---
# Restore film grain on default gallery tiles

Regression from Exh-z8i2 (#301, released to main in #308). Gallery tiles for artifacts without a custom widget show the colored monogram gradient with no film grain. The cardWidget template in internal/api/templates/partials.tmpl emits card-widget-default without the grain class, even though the shared grain styles exist.

## Acceptance Criteria

The gallery shows grain on the colored monogram surface in light and dark mode, including the fallback when a custom widget fails. Healthy custom widget frames keep their existing appearance. Verify in the browser and capture screenshots.

