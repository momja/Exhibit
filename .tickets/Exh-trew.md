---
id: Exh-trew
status: in_progress
deps: [Exh-mg8f]
links: []
created: 2026-10-22T06:50:52Z
type: task
priority: 4
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, portability]
---
# [Experimental] Phase 7: standalone parity: local relay in exported bundles

EXPERIMENTAL; part of Exh-g2vv.

Exported apps that rely on the relay must keep working off Exhibit. The POC's bundle runners (serve.mjs for Node/Deno/Bun, and cmd/exhibit-run) become a local relay enforcing the bundle's network allowlist on every hop. Connections are re-entered locally (an env file or a local connect page); secrets are never exported. A single-file HTML export lists the origins that need a relay. Reuse internal/egress so the rules match Exhibit's. See the POC's packaging findings: unsigned macOS apps hit Gatekeeper, so the bundle stays a ZIP run by node or exhibit-run.

## Acceptance Criteria

- An exported relay-using artifact runs under serve.mjs and exhibit-run with the same allowlist behavior.
- Connections prompt for local secrets.

