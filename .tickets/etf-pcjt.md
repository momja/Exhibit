---
id: etf-pcjt
status: in_progress
deps: []
links: []
created: 2026-11-22T01:00:03Z
type: bug
priority: 1
assignee: Codex classifier subagent
external-ref: gh-307
parent: Exh-zue3
tags: [inference, validation]
---
# Reject incomplete artifact classifier answers

Malformed decide responses can turn JSON null or omitted choice probabilities into zeros. Consumers cannot distinguish those synthetic values from valid provider judgments.

## Acceptance Criteria

Reject null or missing required numeric fields and absent or null probabilities for any offered choice. Preserve valid numeric zeros and existing range and choice validation. Return the stable provider error for invalid replies and preserve metering. Add focused regression tests with mocked classifier answers.

