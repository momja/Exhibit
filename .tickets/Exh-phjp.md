---
id: Exh-phjp
status: closed
deps: []
links: []
created: 2026-10-25T15:10:59Z
type: feature
priority: 2
assignee: Max Omdal
---
# Configure Pi thinking level through the environment

Add an instance-level AGENT_THINKING_LEVEL setting for Exhibit Pi sessions, including BYOK, platform, widget, and resumed conversations. Validate Pi thinking levels at startup and document default and resume behavior.

## Design

Read and validate AGENT_THINKING_LEVEL during server startup. Pass an explicit Pi --thinking argument only when configured; leave Pi defaults and saved session levels in effect otherwise.

## Acceptance Criteria

Supported levels are off, minimal, low, medium, high, xhigh, and max. Invalid values fail startup. A configured value reaches Pi for fresh and resumed sessions. Unset preserves existing behavior. Deployment documentation and focused tests cover the contract.


## Notes

**2026-10-25T15:19:26Z**

Implemented AGENT_THINKING_LEVEL startup validation, a shared Pi --thinking override for fresh/resumed sessions, Compose forwarding, and deployment/agent documentation. Unset preserves existing defaults and saved thinking levels. Verified with real Pi RPC state tests, make assets, go test ./internal/agent ./internal/api ./cmd/server -timeout=10m, a built-server invalid-setting startup check, and Compose unset/high/off checks.
