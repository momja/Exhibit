---
id: Exh-dicw
status: closed
deps: [Exh-mety]
links: []
created: 2026-07-12T03:30:17Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Plugin manifest, registry, and UI mount points

exhibit-plugin.json (name, base_url, scopes, mounts: nav / artifact_toolbar), install flow mirroring ingest approval (fetch manifest -> show scopes/mounts -> operator approves -> scoped token minted), plugins table + enable/disable, wrapper pages embedding plugin UI in sandboxed iframes on the plugin's own origin, origin-pinned postMessage host bridge (getContext, requestFetch proxied with scope checks — token never enters the iframe). docs/plugins.md §4.3


## Notes

**2026-07-12T18:03:50Z**

Closed: plugin-ecosystem scope dropped (epic Exh-i0ll repurposed). Manifest/registry/mounts replaced by a single AGENT_URL config link-out (Exh-hz3g).
