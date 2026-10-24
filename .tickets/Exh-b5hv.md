---
id: Exh-b5hv
status: in_progress
deps: []
links: []
created: 2026-10-23T05:16:31Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-f3cp
tags: [frontend, components, testing]
---
# Shared native modal controller with Duplicate as first consumer

Extract shared native-dialog lifecycle into a Stimulus controller: initial and return focus, Escape/backdrop/explicit dismissal through one cancellable feature guard, and disconnect cleanup. Migrate gallery Duplicate while keeping API calls, form state, retry identity, and htmx refresh feature-owned. Approved consumer on 2026-10-23; prerequisite for av-f05n. Preserve existing visual styling and add real-browser lifecycle coverage.

## Acceptance Criteria

Duplicate uses the registered modal controller; keyboard/focus, dismissal guards while saving, errors/retries, and htmx replacement remain functional. Existing Go/Node checks and the new Playwright journey pass; documentation and screenshots reflect the shared behavior.
