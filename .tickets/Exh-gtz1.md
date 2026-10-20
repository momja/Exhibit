---
id: Exh-gtz1
status: open
deps: [av-g8oa]
links: []
created: 2026-10-20T06:22:17Z
type: task
priority: 2
assignee: Max Omdal
parent: av-f3cp
tags: [testing, e2e]
---
# e2e: coverage inventory and test maintenance guidance

Preserve the useful suite-maintenance portion of [av-g8oa](av-g8oa.md) as a later slice of [av-f3cp](av-f3cp.md), so it does not delay the first hosted CI check.

## Scope

- Add `e2e/COVERAGE.md` mapping concrete browser claims to existing Playwright specs, existing Go/chromedp component tests where appropriate, or an explicit gap linked to a ticket. Seed it from current architecture/security/UI behavior and the actual suite; the old proposal is unavailable.
- Distinguish full app journeys from focused component checks. Include the state round-trip, modal/htmx lifecycle, runtime boundary, and broader UI journeys without claiming that planned tests already exist.
- Document a small maintenance rule in the repository's established `AGENTS.md`/testing guidance: when changing covered browser behavior, update the relevant tests and inventory or record a specific gap with given/when/then acceptance criteria. Running Playwright locally is appropriate for frontend changes affecting these flows or CI investigation; do not require it for every code change.
- Record the proposed changed-path coverage guard as a deferred option. A path touch or ticket ID does not itself prove coverage. Do not add a blocking guard, automatic ticket requirement for unrelated changes, or duplicate instructions in a new `CLAUDE.md` in this story. A later proposal should show useful signal on actual PRs before enforcement.

## Acceptance Criteria

1. Each listed claim has a verifiable current spec/component test or a concrete linked gap. Planned coverage is visibly marked as a gap.
2. Guidance explains when contributors should maintain browser tests and how to run the explicit command, without making Playwright part of ordinary backend/agent workflows.
3. The inventory and guidance use the current native-module/Stimulus/htmx architecture and active ticket links. No missing proposal sections are prerequisites.
