---
id: Exh-u47y
status: open
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-c2hu
tags: [versions, state, history]
---
# Make history and historical previews reliable across loading and narrow screens

UI findings across #149, #151, #153: widget changes leave history stale; long labels collapse narrow version rows; History fetch errors can hide the Chat return control; unbroken headings overflow; delayed View responses replace later choices or reopen a closed preview; old-version previews load offscreen; expanded unsaved notices clip controls; unsupported media/clipboard promises never settle in version mode.

## Design

Use server-rendered htmx regions for refreshed history and preserve unsaved editor buffers. Synchronize version-view requests and invalidate on close; scroll to the actual inserted viewer. Keep recovery/navigation controls outside fallible fetched fragments. Reflow metadata/actions and wrap headings. Make compact viewers scroll or allow frames to shrink. Reject unavailable historical media/clipboard locally as unsupported, rather than leaving promises pending; retain the no-write preview boundary.

## Acceptance Criteria

Browser checks at 1280×800, 375px and 320×568 cover long histories/messages, widget save, failed/slow requests, reordered preview responses, close while pending, navigation recovery, and expanded unsaved notices with all artifact controls reachable. Tests exercise real htmx event behavior. Historical media/clipboard awaits reject predictably and previews still cannot mutate live state.

