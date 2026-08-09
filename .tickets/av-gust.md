---
id: av-gust
status: open
deps: []
links: []
created: 2026-08-09T16:18:19Z
type: bug
priority: 1
assignee: Max Omdal
---
# agent has no guardrails

Agent can be prompted to perform tasks, or respond to questions unrelated to the artifact, including explicit content.

This was tested by pasting a system prompt from a roleplaying site. The agent immediately began responding in the role when asked.

Agents should be restricted to _only_ responding to direct inquiries about the artifact. This means questions to deal with the state,
source code or glances are allowed. Everything else is blocked.

When an agent is prompted with unrelated content, the agent should use a deterministic, canned response.
