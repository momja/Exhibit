---
id: av-cp7j
status: closed
deps: []
links: [av-ei5h]
created: 2026-09-19T14:47:46Z
type: bug
priority: 1
assignee: Max Omdal
tags: [render, sharing]
---
# Shared widget link 404s when the artifact has no widget

GET RENDER_ORIGIN /s/:shareID/widget answers 404 'not found' for an artifact with no widget of its own (ServeShareWidget returns 404 on empty WidgetBlobID, per av-ei5h's design). The gallery never shows nothing for such an artifact: it renders the default tile (monogram on an id-derived hue). A shared widget link should do the same, so an embed never shows a bare 404 page. Observed on test: https://artifacts.dizzard.net/s/f23dd860-ed6a-439d-8020-6f1fb1ee2fdf/widget 404s while the /s/:id link serves the artifact.

## Acceptance Criteria

/s/:id/widget on an artifact with no widget serves a static default-tile document (same monogram and hue as the gallery card, one definition shared by both), with no script, a CSP that allows nothing but inline style, share framing, no-store. Grant ids and unknown ids still 404. The share panel offers the embed snippet whenever the public link exists, not only when a widget exists. docs/widgets.md updated.

