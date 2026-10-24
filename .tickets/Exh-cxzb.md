---
id: Exh-cxzb
status: closed
deps: []
links: []
created: 2026-10-24T17:19:31Z
type: chore
priority: 1
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, relay]
---
# [Experimental] Drop read/write distinction from connections and relay consent

Connections and relay consent currently split "read" from "write" by HTTP method: a connection without allow_writes may only send GET/HEAD, and the per-artifact relay grant has separate reads and writes consent. That boundary is inaccurate in both directions (GETs with side effects pass; read-only POSTs such as GraphQL queries are refused) and cannot be made accurate generically: only the provider's credential scope decides what a connection can do. It also says nothing about confidentiality, the larger risk. Remove the notion entirely rather than describe it more carefully.

## Acceptance Criteria

Native, OAuth and vendor connections carry no allow_writes setting or read/write capabilities; the saved-connection permission editor and PATCH /api/connections/:id are removed. Relay consent is one decision per artifact and viewer, covering any supported method. Existing full (read+write) relay grants carry over; read-only grants are cleared so the viewer is asked again under the new wording. Operator vendor config that still sets allow_writes fails with a clear message. Copy in UI, agent prompt and docs states that the provider's credential scope is the only limit on what a connection may do. Tests and screenshots updated.


## Notes

**2026-10-24T17:47:55Z**

Removed allow_writes from native, OAuth and vendor connections, the connection capabilities metadata, the saved-connection permission editor and PATCH /api/connections/:id (superseding Exh-oeaf), and the relay reads/writes pair. Relay consent is now one {allowed} grant per artifact and viewer covering GET, HEAD, POST, PUT, PATCH and DELETE. Migration 042 keeps grants that already covered every method, clears reads-only grants so viewers are asked again under the new copy, and strips allow_writes from stored recipes. CONNECTORS_VENDOR_PROVIDERS that still sets allow_writes refuses to load and names the provider. UI, agent prompt, docs and examples now say the provider's credential scope is the only limit. Screenshots refreshed from an isolated local instance; the demo instance was not redeployed. GET/HEAD-only throttle retries are unchanged (retry safety, not permission).
