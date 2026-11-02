---
id: Exh-l0wi
status: in_progress
deps: []
links: []
created: 2026-11-02T14:57:13Z
type: task
priority: 2
assignee: Max Omdal
tags: [agent, prompts]
---
# Agent prompts: allow third-party libraries when they earn their place

The shared artifact guidance (internal/agentguidance), the relay-disabled network paragraph and the create/write tool descriptions tell agents to inline all CSS/JS and avoid external references. In practice the agent never reaches for a library (charting, maps, 3D, parsing) unless the user asks, even when hand-rolling it produces a worse tool. Rewrite the guidance so third-party libraries, fonts and resources are allowed: used sparingly and only when they materially help, pinned to an exact version from a reputable CDN, with the origin approval flow explained, but not barred by default.

## Acceptance Criteria

Pi system prompt and MCP instructions permit pinned CDN libraries when needed; no prompt text says all JS must be inline or forbids external references; widgets may reuse origins the artifact already has approved; prompt tests cover the new guidance.

