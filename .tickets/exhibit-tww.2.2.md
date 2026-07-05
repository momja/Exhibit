---
id: exhibit-tww.2.2
status: closed
deps: [exhibit-tww.1.5]
links: []
created: 2026-07-01T05:09:43Z
type: task
priority: 2
parent: exhibit-tww.2
---
# Render tags as colored pills on gallery cards

Display each artifact's tags as colored pills on its gallery card (renderGalleryPage in internal/api/gallery.go). Background = tag color; text color auto-contrasts (black/white by luminance). Depends on backend hydration (tww.1.5) providing id+name+color per artifact. Pills are the container the later interactive controls (pencil/x/+) attach to, so build the static, accessible pill markup + styles first.

## Acceptance Criteria

Each card shows its tags as colored pills with readable text on any background color; artifacts with no tags render cleanly (no empty pill row, but leave room for the trailing '+'); no layout break with many tags (wrap).


