---
id: Exh-im5m
status: closed
deps: []
links: []
created: 2026-11-03T06:19:53Z
type: feature
priority: 2
assignee: Max Omdal
tags: [mobile, search, htmx]
---
# Mobile Search tab across owned and shared artifacts

Move mobile search out of the top menu into the second bottom navigation tab. Provide a dedicated htmx Search page that searches all artifacts accessible to the current viewer, including grants shared with them. Keep the search implementation reusable for a later desktop port.

## Acceptance Criteria

On mobile Search is the second tab after Library; its own /search page swaps through htmx. No search bar remains in the mobile top menu. Search matches owned and granted artifacts with correct permissions and excludes inaccessible artifacts. Search, clearing, paging, navigation, and history are verified. Run review-agent, fix actionable findings, and create a PR.


## Notes

**2026-11-03T06:34:26Z**

Implemented /search as the second mobile tab using the gallery htmx shell and shared search/card components. One access-aware indexed query pages owned artifacts and incoming grants; shared cards retain recipient widget permissions. Mobile top search is hidden; desktop per-space search remains until the later port. Verified focused Go/store/API/Node/Chromium checks, 320–1280px layouts, history, clearing, no-script form fallback, shared widget state and revoked access. All six Playwright journeys pass on isolated scratch ports 18840/18841 (standard ports were occupied). Captured four screenshots under docs/screenshots/Exh-im5m. review-agent found missing mobile shortcut focus; fixed and verified with unmasked browser assertions. Follow-up review: No findings.
