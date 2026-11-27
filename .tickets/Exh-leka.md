---
id: Exh-leka
status: closed
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


## Notes

**2026-11-27T07:09:23Z**

Implemented typed session interactions with revision-checked owner actions and shared SSE/htmx delivery; questions are the first type. Added explicit editable Send answers, whole-set skip with draft confirmation, accessible terminal focus/status, distinct Stop/Skip history, and extension documentation. Independent Standards and Spec signoffs PASS at e036f20a981d730bd418e219ea5f102a4b254f72; hands-on UX PASS at691a30a9, with final delta independently regression-tested. All blocking review findings resolved. Validation: go test -short ./... passed; focused real Pi/Chromium tests at1280/390/320 and skip/stop history passed;49 question/composer Node tests passed. Review files and38-second real-browser video are outside checkout under /tmp/exhibit-review/Exh-leka. Video http://t3code:18482/question-ui.mp4; desktop screenshot http://t3code:18482/questions-desktop.png; mobile evidence http://t3code:18482/ux-review/. Main remains clean; PR will be left unmerged.
