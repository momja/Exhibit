---
id: exhibit-w9y
status: closed
deps: [exhibit-38h, exhibit-eb1, exhibit-qvr]
links: []
created: 2026-05-31T22:30:54Z
type: feature
priority: 0
assignee: Max Omdal
---
# HTTP API: artifact CRUD + ingest endpoint

POST /api/artifacts (ingest), GET /api/artifacts, GET /api/artifacts/:id, PATCH /api/artifacts/:id. Use chi middleware for auth (static token), logging, owner scoping.


