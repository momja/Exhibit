---
id: Exh-15tf
status: open
deps: [Exh-gd3g, Exh-x16u]
links: []
created: 2026-11-04T05:37:11Z
type: task
priority: 3
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, agent, docs, experimental]
---
# Teach the agents to build artifacts that use window.exhibit.ai

Built-in and connected agents should reach for decide() when a tool needs a judgment and complete() when it needs prose, and write artifacts that degrade cleanly when either is unavailable.

## Design

- internal/agentguidance: describe only the kinds the instance has configured. Rules: use decide() for anything that drives logic (route, gate, classify, score), keep thresholds in the artifact's code and treat unsure as a real outcome; use complete() for prose; check window.exhibit.ai.available() and handle unavailable (shared via public link, opened directly, exported); never put saved data in a prompt without the person's knowledge.
- With inference disabled, the guidance says nothing about it, so the agent doesn't write calls that cannot work.
- An example artifact under examples/ that uses decide() to triage items and complete() to summarise them, with a widget that reads the saved result, and that still works with either kind missing.
- docs/inference.md covering configuration of both provider families, the payer rule, limits and the approval flow; PRD and architecture entries per the docs convention.

## Acceptance Criteria

- Prompt golden tests cover each combination: neither, complete only, decide only, both.
- The example artifact runs against mock providers in a browser test, and degrades with each kind removed.
- Configuration variables documented beside the AGENT_* table.
