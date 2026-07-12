---
id: Exh-bujj
status: closed
deps: []
links: []
created: 2026-07-12T03:30:17Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Event feed: SSE firehose + optional signed webhooks

GET /api/events (scope events:subscribe) emitting artifact.created/updated/deleted, state.changed, share.created/deleted — ids only, plugins fetch bodies per their scopes. Reuse agent SSE plumbing. Webhooks (HMAC-signed, fire-and-forget) are phase 2. docs/plugins.md §4.2


## Notes

**2026-07-12T18:03:50Z**

Closed: plugin-ecosystem scope dropped (epic Exh-i0ll repurposed). No event feed needed for the agent-via-API extraction.
