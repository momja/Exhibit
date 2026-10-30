---
id: Exh-1e7n
status: in_progress
deps: []
links: []
created: 2026-10-30T06:41:43Z
type: feature
priority: 2
assignee: Max Omdal
---
# Render agent Markdown without HTML or message boxes

Support Markdown in assistant messages with a maintained standard parser. Never render raw HTML. Remove assistant message bubbles; keep person messages boxed. Cover live streams, saved history and resumed conversations.

## Acceptance Criteria

Markdown renders during streaming and in saved/resumed conversations. Raw HTML and unsafe links stay inert. Only person messages have bubbles. Desktop and narrow mobile edge cases verified with screenshots. Run standards/spec review agents, address findings and create a PR with screenshots.

