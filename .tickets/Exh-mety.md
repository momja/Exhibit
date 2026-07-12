---
id: Exh-mety
status: closed
deps: []
links: [av-q30x]
created: 2026-07-12T03:30:17Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Scoped per-plugin API tokens

tokens table (hash at rest, like agent keys), capability scopes (artifacts:read/write, state:read/write, tags:write, collections:write, shares:write, events:subscribe), required-scope annotation per route at the existing auth-middleware seam (same seam as av-q30x), mint/revoke settings UI. Operator token keeps full scope. docs/plugins.md §4.1


## Notes

**2026-07-12T18:03:50Z**

Closed: plugin-ecosystem scope dropped (epic Exh-i0ll repurposed to agent extraction). The agent service authenticates with the existing static token; scoped tokens can be revisited alongside real auth (av-q30x) if ever needed.
