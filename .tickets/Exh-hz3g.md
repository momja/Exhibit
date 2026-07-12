---
id: Exh-hz3g
status: open
deps: []
links: []
created: 2026-07-12T18:03:33Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Exhibit-side seams for an external agent service

What Exhibit core must provide so the agent UI can live on another origin: (1) AGENT_URL config — when set, the gallery header 'Agent' link and the detail-page 'Modify with agent' action point at the external agent UI instead of the compiled-in /agent page; hidden when unset. (2) Embedder-origin allowance for the render-surface bridges: the snippet picker's activation check and postMessage target, and the storage-shim __avState bridge, are currently pinned to APP_ORIGIN — accept a configured additional embedder origin (the agent service's origin) so its chat page can embed render iframes with working snippet capture and state write-through. No manifest/registry/scopes machinery — one configured origin, explicit and static.

