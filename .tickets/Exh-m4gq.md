---
id: Exh-m4gq
status: in_progress
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-c2hu
tags: [versions, state, history]
---
# Release conversation identity when Pi exits during registration

Latest CodeRabbit review on #152 identifies a startup/cleanup race: readLoop/cmd.Wait can finish and removeScratch can run before Create adds the session to m.sessions, after which Create registers a closed session. Get hides it but reserveConversation rejects that ID until restart.

## Design

Make registration and final cleanup agree on ownership of the session identity. Preserve waiting for reader completion and final transcript persistence, and ensure failed/duplicate startups still drain and remove their scratch directories. Apply this release fix to #152 and propagate it to #153 before shipping.

## Acceptance Criteria

A deterministic lifecycle regression exercises process completion before registration and proves no closed session remains registered or prevents resume. Duplicate/rejected startup cleanup remains intact. Focused lifecycle race tests and combined-stack Go/script checks pass.

