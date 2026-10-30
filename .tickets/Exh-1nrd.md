---
id: Exh-1nrd
status: closed
deps: []
links: []
created: 2026-10-30T04:31:55Z
type: feature
priority: 2
assignee: Max Omdal
---
# Welcome users to an empty artifact library

Replace the plain empty-library message with a welcoming introduction, visual interest, and a prominent action to create a first artifact using the existing /new flow.

## Acceptance Criteria

The empty library has a welcoming responsive design and a clear first-artifact action. Existing search, filter, and shared-gallery empty states stay contextual. Verify light/dark and mobile layouts.


## Notes

**2026-10-30T04:36:28Z**

Implemented a library-only welcome partial with decorative tool cards, a first-artifact CTA to /new, responsive token-based styling, and registered self-hosted Phosphor icons. Updated existing empty-state test. make assets, server build, and focused gallery/shared/new-page/icon/versioned-asset tests passed. Browser verified desktop and 320px/390px phones in light/dark, no overflow, CTA navigation, and live search/clear restoration. Screenshots saved in docs/screenshots/Exh-1nrd/.

**2026-10-30T04:40:53Z**

Simplified per user feedback: removed all welcome text except Welcome to Exhibit and the creation button, including illustration labels and numeric timer. Preserved decorative graphics and accessible heading. Updated screenshots; focused empty-state test and server build pass.
