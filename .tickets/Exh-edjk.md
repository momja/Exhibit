---
id: Exh-edjk
status: closed
deps: [Exh-jlbt]
links: []
created: 2026-07-11T05:19:45Z
type: task
priority: 1
assignee: Max Omdal
parent: Exh-yvhp
---
# Snippet mode: hotkey element picker + screenshot context in the render iframe

Render surface injects a snippet script (like the shim) activated by host postMessage on hotkey. Hover-highlight + click selects an element; captures outerHTML/id/class descriptor chain and rasterizes the element via SVG foreignObject to a PNG data URL; posts back to host pinned to app origin. Chat UI attaches the image + descriptors to the next agent prompt (pi RPC prompt images).

