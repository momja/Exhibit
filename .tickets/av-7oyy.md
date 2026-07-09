---
id: av-7oyy
status: open
deps: [av-eu3v, av-n8v5, av-ra31, av-epnt]
links: []
created: 2026-07-09T06:04:34Z
type: chore
priority: 3
assignee: Max Omdal
parent: av-ec0t
tags: [integration, testing, public-mode, qa]
---
# Integration: Default-safe fallback when public mode disabled

Verify that the default state (PUBLIC_MODE_ENABLED unset or false) leaves the application behaving exactly as it did before this epic. All routes should require authentication, the upload form should remain prominent, and no public-only UI elements should leak. This is a regression-prevention task: run through the existing self-hosted flow (upload, search, tag edit, artifact open, delete) and confirm nothing is broken.

## Acceptance Criteria

1. With public mode disabled (default), unauthenticated requests to /, /api/artifacts, and artifact view routes return 401/redirect to login. 2. Authenticated user experience is unchanged: upload form present, all management controls visible, tag edit works. 3. No public-only hero or tagline appears. 4. All existing tests (if any) pass. 5. Manual smoke test of the full self-hosted flow completes successfully.

