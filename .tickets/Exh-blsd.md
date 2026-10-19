---
id: Exh-blsd
status: closed
deps: []
links: []
created: 2026-10-19T04:07:49Z
type: feature
priority: 2
assignee: Max Omdal
---
# Make the agent side panel resizable

Add a draggable, keyboard-accessible divider between agent chat and artifact preview. Remember the preferred desktop width and preserve mobile Chat/Preview tabs.

## Acceptance Criteria

Desktop chat width can be resized with pointer and keyboard; panes retain usable minimum widths; saved width survives reload; narrow-screen tabs remain usable.


## Notes

**2026-10-19T04:13:10Z**

Implemented a persistent desktop divider with pointer capture, keyboard controls, accessible width values, reset, viewport clamping, and mobile-tab preservation. Browser verified dragging across a live artifact preserves its running counter; narrow preview controls wrap without overflow. Screenshots are in docs/screenshots/Exh-blsd/. Focused Go and five resize interaction tests pass; full Go suite is running.

**2026-10-19T04:16:52Z**

Final validation: go test ./... passes after regenerating the stale local Phosphor bundle. Browser checks pass for pointer dragging across the artifact, keyboard controls, reload persistence, viewport clamping, and mobile tabs. Four current screenshots are committed with the implementation.
