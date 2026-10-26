---
id: Exh-2c2e
status: in_progress
deps: [Exh-u9b1]
links: []
created: 2026-10-26T01:02:13Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-qeuv
tags: [gallery, frontend, css]
---
# B2 spaces navigation: left rail on desktop, bottom tab bar on phones

The navigation chosen in av-0k5q (B2).

- One Go partial 'gallerySpaces' (components or gallerygrid template) listing My library / Shared with me / Shared by me with Phosphor icons (ph-books, ph-tray-arrow-down or ph-users, ph-share-network — pick and add to web/icons/icons.txt) and the active space marked aria-current=page. Plain links, no JS: each space is its own page from the previous story.
- Two layouts from the same markup in index.css: desktop/wide — a left rail beside the grid (~180px, labels + counts); narrow — a fixed bottom tab bar (icons + short labels, safe-area-inset-bottom padding, grid gets bottom padding so the last row is not hidden). Breakpoint by width; tab bar targets >=44px. Dark mode from tokens only (no literal colors).
- Render the nav only when SpaceCounts says a second space has content (shared-with > 0 or shared-by > 0). A library with neither renders exactly today's page — assert this in a test.
- Show counts next to Shared with me / Shared by me in the rail (not in the tab bar).
- Keyboard: links in order, visible focus. Screen reader: <nav aria-label="Library spaces">.
- Browser test (existing Chromium helper) at a phone viewport and a desktop viewport: correct layout shown, active state, navigation works. Update screenshots.

## Acceptance Criteria

- One partial, two CSS layouts; no layout JS.
- Hidden when only My library has content.
- Phone: bottom tab bar, content not obscured; desktop: left rail.
- Tests and screenshots updated.

