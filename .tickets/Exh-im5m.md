---
id: Exh-im5m
status: in_progress
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

