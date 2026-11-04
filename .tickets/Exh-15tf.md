---
id: Exh-15tf
status: open
deps: [Exh-x16u]
links: []
created: 2026-11-04T05:37:11Z
type: task
priority: 3
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, agent, docs, experimental]
---
# Teach the agents to build artifacts that use window.exhibit.ai

Built-in and connected agents should reach for decide() when a tool needs a judgement and complete() when it needs prose, and write artifacts that degrade cleanly when inference is unavailable.

## Design

- internal/agentguidance: when inference is enabled on the instance, describe both calls, the error set, and the rule that an artifact must handle 'unavailable' (shared via public link, opened directly, exported). Prefer decide() for anything that drives logic. Never put saved data in a prompt without the person's knowledge.
- When inference is disabled, the guidance says nothing about it, so the agent doesn't write calls that cannot work.
- An example artifact under examples/ that uses decide() for routing and complete() for a summary, with a widget that reads the saved result.
- docs: a section in docs/agent.md or a new docs/inference.md covering configuration, the payer rule, limits and the approval flow; PRD and architecture entries per the docs convention.

## Acceptance Criteria

- Prompt golden tests cover inference-enabled and inference-disabled guidance.
- The example artifact runs against the mock provider in a browser test.
- Configuration variables documented beside the AGENT_* table.

