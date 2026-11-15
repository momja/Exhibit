---
id: Exh-91qf
status: in_progress
deps: []
links: []
created: 2026-11-08T19:14:59Z
type: chore
priority: 2
assignee: Max Omdal
---
# Remove the duplicate gallery tab header block

Every gallery destination repeats its active tab label as a page title and subtitle: Starred, Search, Shared, and Activity. The rail already names and highlights the destination, so the block is a second header before the artifacts.

Remove the visible title and subtitle from all four destinations. Keep an accessible-only heading so the main region still has a name, and delete the now-unused description data and styles.

## Acceptance Criteria

Starred, Search, Shared, and Activity render no visible page title or subtitle while each main region keeps an assistive-technology heading; gallery description fields and their CSS are gone; desktop and phone screenshots show the artifact content starting directly under the app header; gallery navigation, empty states, and responsive layouts still pass their tests.
