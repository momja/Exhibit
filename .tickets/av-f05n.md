---
id: av-f05n
status: open
deps: [av-e1sr]
links: []
created: 2026-07-07T05:44:49Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-f3cp
---
# e2e: UI flow tests — ingest approval, share path, gallery search/tags

ingest.spec.ts: through the UI (paste HTML with external references), assert the footprint approval dialog lists the scanner's origins, approve a subset, assert the artifact lands in the gallery with exactly that allowlist. share.spec.ts: share backend exists today (exhibit-7k3 closed) but there is no share button — mint via POST /api/shares, open /s/:id in a context with no credentials, assert render; assert expired/deleted share does not render; switch minting to the UI when a share button ships. gallery.spec.ts: seed artifacts with distinct titles/tags, submit the search form, assert the filtered grid; exercise tag pill add/edit/detach modals. See docs/proposals/e2e_testing.md §5 items 4-6.

## Acceptance Criteria

GIVEN pasted HTML with external origins WHEN uploaded THEN the approval dialog shows the scanned origins and only approved ones land on the allowlist. GIVEN a minted share WHEN /s/:id is opened with no credentials THEN it renders; expired/deleted shares do not. GIVEN seeded artifacts WHEN searching THEN only matches render; tag add/edit/detach round-trips through the modals.

