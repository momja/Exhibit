---
id: Exh-ucwz
status: open
deps: []
links: []
created: 2026-11-16T23:01:54Z
type: chore
priority: 2
assignee: Max Omdal
tags: [deployment, mcp]
---
# Verify hosted MCP rollout and existing grants

Track remaining live acceptance from Exh-7y0c and PR #291. Repository implementation does not establish live deployment. Requires separately authorized operator rollout and Fly/DNS credentials; do not deploy merely to close this task.

## Acceptance Criteria

Confirm current app-host certificate and DNS; HTTP redirects to HTTPS at https://exhibit.dizzard.net/mcp. Inspect/update stale Fly URL overrides, preserve EXHIBIT_SECRET and EXHIBIT_PUBLIC_URL=APP_ORIGIN, deploy only with explicit authority. Confirm Settings and both resource metadata paths plus authorization-server metadata advertise stable URLs without fly.dev. From a clean client complete browser approval and one tool call. Confirm preexisting grants list and revoke; document re-adding clients configured with old URL.

