---
id: exhibit-tww.2.3
status: closed
deps: [exhibit-tww.1.4, exhibit-tww.2.1, exhibit-tww.2.2]
links: []
created: 2026-07-01T05:09:44Z
type: task
priority: 2
parent: exhibit-tww.2
---
# Pill hover controls: pencil (edit) on left, x (detach) on right

On pill hover, reveal a Phosphor pencil on the LEFT edge and a Phosphor 'x' on the RIGHT edge. Pencil opens the edit-tag modal (tww.2.4). 'x' detaches THIS tag from THIS artifact only via DELETE /api/artifacts/{id}/tags/{tagID}, then removes the pill from the card (optimistic or re-render). Keyboard/focus accessible; controls hidden until hover/focus. Depends on pills (this epic) + Phosphor (this epic) + hardened detach (tww.1.4).

## Acceptance Criteria

Hovering a pill shows pencil (left) and x (right); clicking x detaches only that artifact's association and the pill disappears while the tag still exists globally; controls are reachable by keyboard and don't shift pill layout on hover.


