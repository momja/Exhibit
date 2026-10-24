---
id: Exh-h5va
status: in_progress
deps: []
links: [Exh-co2m, Exh-g2vv]
created: 2026-10-22T02:24:56Z
type: feature
priority: 1
assignee: Max Omdal
---
# Function-based serverless backend, Exhibit Reader, and benchmarks

Implement function-based serverless artifact backend (WinterCG fetch handler in Wasm/QuickJS sandbox with network allowlist + SSRF guard + connector support), single-file HTML export with embedded worker manifest, Exhibit Reader standalone runner, interactive demo artifact, and performance benchmarks.


## Notes

**2026-10-22T16:02:30Z**

Reconciliation decision for Exh-g2vv: the relay plus native/managed credential engines supersedes synchronous artifact backends for external API and user-account access. Declarative schedules cover fetch-to-state refresh without executing artifact code on the server. Keep arbitrary backend execution parked; inbound webhooks, shared server authority or server-side computation need a separate concrete requirement and review. See docs/relay-connectors.md on feature/Exh-g2vv/relay-connectors. This note does not change this separate experiment’s status.
