---
id: Exh-leka
status: in_progress
deps: []
links: []
created: 2026-11-27T06:43:30Z
type: feature
priority: 1
assignee: Max Omdal
tags: [agent, ui]
---
# Generalize agent panel interactions and clarify question completion

Build a shared typed interaction pattern in the agent panel with questions as the first implemented type. Clean up the question completion and skip workflow. Have an independent panel review code and exercise a running demo; address findings, open an unmerged PR, and record a video of the new question UI.

## Design

Keep server-owned validated interaction state, owner-authenticated actions, correlated tool replies, SSE change signals, Go template partials and htmx swaps. Share transport/lifecycle independently of question-specific form behavior. Establish explicit type and revision semantics, cancellation/expiration, reconnect and history behavior. Implement only questions; document the extension seam for approvals, connection setup and status displays without granting new permissions. Favor deliberate submission, editable choices, clear whole-request skip semantics and accessible completion feedback.

## Acceptance Criteria

Questions use a reusable typed interaction envelope/lifecycle, server-rendered interaction endpoint and shared browser delivery module. Completion and skip semantics are clear across single, multi-select and written answers; normal chat redirection remains supported. Preserve authorization, input validation, guardrails, duplicate/stale reply rejection, cancellation, draft preservation, session isolation and safe history. Meaningful Go/browser regressions pass. Independent reviewers exercise a real deterministic demo on desktop/mobile and provide code and UX feedback; findings are addressed. Open and link a PR with screenshots, review evidence and an actual browser recording. Do not merge.

