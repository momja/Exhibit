---
id: av-4ac9
status: open
deps: []
links: []
created: 2026-07-09T06:04:16Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-ec0t
tags: [backend, config, public-mode]
---
# Backend: Public instance config (env vars + settings API)

Add configuration layer for public instance mode: PUBLIC_MODE_ENABLED (bool), PUBLIC_INSTANCE_NAME (string), PUBLIC_INSTANCE_DESCRIPTION (string). These may live as environment variables and/or a new settings row in SQLite. Expose a GET /api/settings/public endpoint that returns {name, description} for the frontend to consume when rendering the public gallery. The endpoint must be callable without authentication when public mode is enabled.

## Acceptance Criteria

1. PUBLIC_MODE_ENABLED env var is read at startup and accessible to handlers. 2. PUBLIC_INSTANCE_NAME and PUBLIC_INSTANCE_DESCRIPTION are read and stored/accessible. 3. GET /api/settings/public returns {name, description} (empty strings if unset). 4. When public mode is off, existing auth behavior is unchanged. 5. Config is available to the Go gallery renderer without requiring a database round-trip if env-based.

