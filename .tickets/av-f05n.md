---
id: av-f05n
status: in_progress
deps: [av-e1sr, av-g8oa, Exh-b5hv]
links: []
created: 2026-07-07T05:44:49Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-f3cp
---
# e2e: modal save flow across Stimulus and htmx replacement

Third step in [av-f3cp](av-f3cp.md): use the upcoming shared modal to cover one complete UI lifecycle after the [harness](av-e1sr.md) and [CI gate](av-g8oa.md) exist. Broader ingest/sharing/search/tag journeys moved to [Exh-jtv1](Exh-jtv1.md).

## Approved consumer and prerequisite

The first consumer, approved on 2026-10-23, is gallery Duplicate: it already starts from a Stimulus action menu, opens a native dialog, saves through the API, dispatches `exhibit:artifacts-changed`, refreshes server-rendered gallery cards through htmx, and focuses the new card. Its dialog is currently feature-owned, not a shared Stimulus modal. The controller and migration are tracked in [Exh-b5hv](Exh-b5hv.md), which is a prerequisite for this coverage.

This story tests that component when it ships; it does not implement the shared modal or invent a new product flow. The existing `TestActionMenuBrowser` and `TestGalleryActionMenuDuplicationBrowser` remain focused component coverage. Avoid duplicating all their menu/controller internals in Playwright.

## Design

- Drive the real authenticated application with API-seeded fixture artifacts. All actions under test (menu activation, modal input, save, refresh) go through the UI and ordinary server routes. No mocked successful mutation or synthetic replacement markup.
- Use accessible roles/labels and keyboard input. Use the selected feature's documented native-dialog contract for initial focus, focus containment, dismissal guards, and return focus; conventions are in [UI components](../docs/ui-components.md).
- Observe the successful mutation and actual htmx fragment request/DOM replacement. After refresh, interact with a newly inserted/replaced action menu and reopen its modal to prove Stimulus connected the new nodes and that the feature handler still runs exactly once.
- Assert successful-save focus on the intended replacement element. For Duplicate, cancellation restores the visible menu trigger; successful creation focuses the new card title after refresh, then its action menu/dialog is usable. Do not require focusing a removed trigger.

## Acceptance Criteria

1. GIVEN an authenticated gallery with a fixture artifact, WHEN its action menu is activated by keyboard and the chosen action is selected, THEN the modal opens with the correct artifact and initial field focus. Tab/Shift+Tab remain in the modal while open.
2. Escape, explicit Cancel, and backdrop dismissal obey the same feature guard, issue no mutation when cancellation is allowed, and return focus to the connected visible trigger. Test the documented pending-save dismissal behavior as well.
3. WHEN valid input is submitted, THEN exactly one mutation succeeds, the server-rendered htmx region refreshes without full-page navigation, the saved result appears, and focus lands on the feature's intended surviving/new element. Use request/DOM conditions rather than fixed sleeps.
4. WHEN the refreshed component is operated again by keyboard, THEN its menu and modal work with the new artifact data and one user action causes one feature action. Exercise at least two refresh cycles to catch accumulating handlers or stale targets.
5. A failed save leaves actionable error feedback and a usable modal; retry follows the feature's existing idempotency contract and cancellation behavior. A controlled API error may be injected for this error-path case; the successful path uses the real API.
6. The flow runs in the required hosted Chromium suite with failure traces. Existing Go/Node/component tests remain in place; no blanket local Playwright requirement is added.

## Historical scope superseded — 2026-10-20

The July note requested broad flows to protect classic inline-bootstrap/external-script globals. The gallery now includes native modules and Stimulus. Cover the current behavior and import/component contracts, not a rule that scripts must stay classic. Follow-up journeys are tracked in [Exh-jtv1](Exh-jtv1.md); share UI already exists and share expiry no longer exists.
