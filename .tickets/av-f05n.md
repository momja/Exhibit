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


## Notes

**2026-07-15T06:33:03Z**

When building the UI-flow suite, explicitly cover the epi-q0u2 split-script seam end-to-end: the gallery page scripts (index/detail/edit) moved into static assets that read/reassign globals declared by each page's inline bootstrap (TOKEN, ID, SOURCE_URL, allowlist, downloadsApproved, clipboardApproved, DEFAULT_TAG_COLOR). A Go contract test (TestGalleryBootstrapSharesGlobalsWithAssets) pins that both sides name the same globals and stay classic (non-module) scripts, but it can't confirm the flows actually run. e2e should exercise: upload -> approve origins (PATCH + re-render), tag add/edit/delete, eager search grid-swap, download approval, clipboard bridge, allowlist add, and refetch — the flows whose wiring the inline->external split put at risk and which substring/Go tests structurally can't see.
