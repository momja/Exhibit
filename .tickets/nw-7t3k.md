---
id: nw-7t3k
status: closed
deps: []
links: []
created: 2026-09-27T04:40:00Z
type: feature
priority: 2
---
# Move tag editing from the gallery to the edit page
Tags on gallery cards are static pills: no edit/detach controls, no '+' button, no tag modals.

All tag interaction moves to `/artifacts/:id/edit`, in a new collapsible "Tags" section:
- current tags, each with edit (rename/recolor/delete, library-wide) and remove controls
- a dropdown to attach an existing tag or create a new one
- changes save immediately via the tag API and the section re-renders in place (htmx), so unsaved editor buffers survive
