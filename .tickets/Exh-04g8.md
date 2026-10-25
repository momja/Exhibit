---
id: Exh-04g8
status: in_progress
deps: []
links: []
created: 2026-10-25T18:32:22Z
type: bug
priority: 2
assignee: Max Omdal
---
# Restore direct ticket pushes with migration checks enabled

The Migration versions required-check ruleset also blocks direct ticket-only pushes to main. Preserve the established ticket workflow while retaining migration collision checks on PRs. GitHub does not support file-path exemptions for required checks; agree on the bypass scope before changing live enforcement.

## Acceptance Criteria

The owner can push tickets directly to main; the migration CI job remains present and required for actors without a bypass; tracked ruleset configuration matches live settings; bypass limitations are explicit.

