---
id: Exh-bjqg
status: closed
deps: []
links: []
created: 2026-11-26T05:50:23Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i8q4
tags: [ui, detail]
---
# Artifact bar: pip.js rewrite, placement trim, markup and CSS

Rewrite web/gallery/pip.js per docs/artifact-bar.md §3 and §5: bottom-centre bar, full/compact state machine, popups from fixed anchors, hover, disposer; delete drag/stash/tab/announcer and persisted placement (clear exhibit.pipControls once). Trim pip-placement.js and its test. Remove #pip-grip, #pip-hide, #pip-tab, #pip-announce from components.tmpl; CSS in components.css/detail.css; drop data-pip-corner.

## Acceptance Criteria

pip.test.mjs covers §7 Node list; pip-placement.test.mjs trimmed; appheader_browser_test.go covers bar at bottom centre at 1280x800 and 390x844 and real scroll compacting; detail_controls_test.go control sets unchanged apart from grip/hide.

