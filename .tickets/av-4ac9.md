---
id: av-4ac9
status: closed
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


## Notes

**2026-08-06T05:24:31Z**

Added a fifth knob the ticket did not name: PUBLIC_OWNER_ID (int, default 1).

This ticket predates av-ep8k, which made owner_id a real query predicate — every artifact read now filters on an owner. 'The library' is therefore no longer a well-defined phrase on an instance that may hold several, and a public instance has to say whose library it publishes. Naming the owner is config, which is exactly this ticket's job; resolving an unauthenticated request TO that owner is av-wmp6's, and is deliberately not implemented here. av-wmp6 inherits a named owner rather than having to invent one.

Also decided here: GET /api/settings/public 404s when public mode is off, rather than returning empty strings. An instance that has not opted into being public should not name itself to an unauthenticated stranger, and 200-with-empty-strings is already the meaningful answer for 'public, but the operator set no name' — reusing it for 'not public' would collapse two states into one body.
