---
id: Exh-ituu
status: closed
deps: []
links: [Exh-8vjo, Exh-i0ll]
created: 2026-10-26T05:54:18Z
type: chore
priority: 2
assignee: Max Omdal
tags: [agent, workers, descope]
---
# De-scope standalone workers: remove dormant #189 authority layer

PR #189 (worker authority: fenced leases, supervisor credentials, checkpoints, usage receipts) reached main on 2026-10-21, but nothing constructs agentcontrol or mounts the worker API; the rest of the stack (#190-#192, recovery PR #215) is de-scoped because there is no scaling problem yet. Remove the dormant code. Keep shipped migration 035 and drop its never-written tables in a new forward migration. The full stack survives at 3dff8a1288a3377edb586ac81777f33c2af8dfc9 if ever needed.

## Acceptance Criteria

agentcontrol, the /internal/agent-worker API, store agent authority methods and agentscope execution hooks are removed. Migration 035 is unchanged; a new migration drops agent_usage_receipts, agent_supervisor_credentials, agent_executions and agent_conversations. Hosted account deletion keeps its session fence. Full Go suite and migration-order/upgrade tests pass. Docs no longer describe the removed layer.

