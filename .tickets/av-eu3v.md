---
id: av-eu3v
status: open
deps: [av-wmp6]
links: []
created: 2026-07-09T06:04:24Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-ec0t
tags: [frontend, ui, public-mode, gallery]
---
# Frontend: Public root gallery view (unauthenticated read-only)

Create a read-only variant of the gallery page for unauthenticated visitors when public mode is enabled. The existing server-rendered gallery in internal/api/gallery.go should branch based on the request context (auth state + public mode). Remove: upload form, tag edit pencils, artifact delete buttons, settings links. Keep: artifact cards, titles, dates, tag pills (read-only), search bar (if filtering is desired for public), and the Open link. The gallery should list all artifacts (owner-scoped to the single owner_id=1 in v1).

## Acceptance Criteria

1. Root route '/' renders a clean artifact grid for unauthenticated users when public mode is on. 2. Upload form is absent. 3. Tag management controls (pencil, x, +) are absent. 4. Artifact 'Open' links work and lead to the public artifact view. 5. Search bar remains functional for filtering public artifacts. 6. The page uses the same inline CSS and markup style as the existing gallery.

