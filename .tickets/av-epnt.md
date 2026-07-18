---
id: av-epnt
status: open
deps: [av-wmp6]
links: []
created: 2026-07-09T06:04:34Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-ec0t
tags: [frontend, ui, public-mode, render]
---
# Frontend: Public artifact detail view (unauthenticated /artifact/:id)

Ensure that viewing a single artifact works for unauthenticated users when public mode is enabled. The app surface likely has a route that loads the artifact in the sandboxed iframe (pointing to RENDER_ORIGIN). This route must be accessible without auth in public mode. The artifact should render with its per-artifact CSP and storage shim exactly as it does for authenticated users. If the app currently requires auth to reach the artifact viewer page, that check must be bypassed conditionally.

## Acceptance Criteria

1. Unauthenticated users can navigate to an artifact view page (e.g., /artifact/:id or the existing detail route) when public mode is on. 2. The sandboxed iframe src is set correctly to the render origin. 3. Artifact renders with full CSP and storage shim. 4. No management controls (delete, edit title, edit allowlist) are shown to unauthenticated users. 5. When public mode is off, artifact view routes require auth as before.


## Complexity

M
