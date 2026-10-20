---
id: Exh-johq
status: closed
deps: []
links: []
created: 2026-10-20T02:25:12Z
type: feature
priority: 2
assignee: Max Omdal
---
# Replace gallery edit buttons with artifact action menus

Replace each shared gallery card Edit link with an action menu matching the artifact pill menu. Initially include Edit; retain navigation and accessible dismissal behavior.

## Acceptance Criteria

All gallery cards expose an action menu containing Edit; menu styling follows the pill; keyboard, touch, and htmx-added cards work.


## Notes

**2026-10-20T02:28:38Z**

Implemented shared pill-style card action menus with Edit, delegated keyboard/dismissal behavior, touch reachability, and shared styles. Verified focused gallery/detail Go tests (including gallery page-script suite), browser opening/Escape/focus dismissal, htmx search replacement, and phone-width bounds. Screenshot and architecture notes updated.
