---
id: Exh-1zg6
status: in_progress
deps: []
links: []
created: 2026-11-26T18:47:46Z
type: bug
priority: 2
assignee: Max Omdal
parent: av-f3cp
tags: [e2e, tests]
---
# Playwright phone share layout spec targets removed #share-link-copy id

The Playwright journey e2e/specs/ui-appearance.spec.ts › 'phone share controls and choices compose cleanly in both themes' fails deterministically with 'TypeError: Cannot read properties of null (reading getBoundingClientRect)'. It measures the public link's copy button as #share-link-copy, but #287 (fa2a43ca, 2026-11-15) moved the link into the shared copyField partial, which renders the input as #share-link-url and its button as {{.ID}}-copy = #share-link-url-copy. No template or script defines #share-link-copy any more; the spec is the only reference. Found 2026-11-25 running make e2e locally (27 passed, 2 failed). The other failure in that run, editor-tools.spec.ts (label.row > span counting 12 instead of 6), is already fixed on main by #296 (f9c89565). Browser regression runs only post-merge, and on main it currently fails earlier in the Go/Chromium component step, so this journey has not been reached in CI since #287.

## Acceptance Criteria

- The share layout journey measures the copy button that the share panel actually renders.
- Its layout assertions (helper below Replace, link field and copy control of equal height and at least 44px, stacked wide state choices, no horizontal overflow) hold at 320px and 390px in light and dark, or any assertion that no longer matches the intended copyField design is updated with the reason stated.
- make e2e passes locally against a fresh build of the branch.
- No application code changes unless the run shows a real layout regression, in which case scope is re-confirmed before proceeding.

