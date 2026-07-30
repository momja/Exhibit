---
id: av-u5k7
status: open
deps: []
links: []
created: 2026-07-30T16:53:34Z
type: task
priority: 2
assignee: Max Omdal
tags: [ui, agent]
---
# Spinning Phosphor icon on the agent thinking indicator

The agent chat's thinking indicator (`web/gallery/agent.js:234`, rendered as `el('div','thinking','thinking…')`) is plain italic text. Give it a spinning Phosphor `ph-circle-notch` icon so an in-flight turn reads as motion, matching the icon vocabulary the rest of the agent page already uses (AGENTS.md UI conventions, technical_stack.md §9).

## Design

Render the indicator as `<i class="ph ph-circle-notch"></i> thinking…` instead of a bare text node — build the icon element in JS alongside the existing div rather than injecting HTML.

Add the spin in `web/gallery/agent.css` next to the existing `.thinking` rule: a `@keyframes` 0→360deg rotation on `.thinking i`, ~1s linear infinite, with the icon sized/aligned like `.tool-chip i` (inline-flex row, ~6px gap, 13px icon). Respect `@media (prefers-reduced-motion: reduce)` by dropping the animation.

Phosphor is already loaded on the agent page (`agent.tmpl:12`, self-hosted `/assets/phosphor/regular.css`) — no new asset work. `removeThinking()` already tears the element down, so no lifecycle change.

## Acceptance Criteria

- The thinking indicator shows a rotating `ph-circle-notch` to the left of the 'thinking…' label while a turn is in flight.
- The icon disappears with the indicator on `text_delta`, `tool_execution_start`, and `agent_settled` (existing `removeThinking()` paths).
- No spin under `prefers-reduced-motion: reduce`.
- Icon comes from the self-hosted Phosphor stylesheet; no CDN reference added.
- Screenshot of the agent window mid-thought attached to the PR.

