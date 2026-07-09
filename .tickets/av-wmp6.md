---
id: av-wmp6
status: open
deps: [av-4ac9]
links: []
created: 2026-07-09T06:04:24Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-ec0t
tags: [backend, auth, public-mode, middleware]
---
# Backend: Conditional auth middleware for public mode

Modify the existing chi auth middleware to become public-mode aware. When PUBLIC_MODE_ENABLED is true, skip auth checks for safe read-only GET routes: GET /api/artifacts, GET /api/artifacts/:id, GET /api/settings/public, and the root gallery route. All mutating routes (POST/PUT/PATCH/DELETE) must remain auth-gated regardless of public mode. The unauthenticated gallery renderer needs to know it is in public mode so it can suppress edit controls.

## Acceptance Criteria

1. Unauthenticated GET requests to /api/artifacts succeed when public mode is enabled. 2. Unauthenticated GET requests to /api/artifacts/:id succeed when public mode is enabled. 3. Unauthenticated POST/PUT/PATCH/DELETE to any API route returns 401 even in public mode. 4. When public mode is disabled, all routes require auth exactly as before. 5. The auth middleware passes a public-mode flag to request context so handlers can branch.

