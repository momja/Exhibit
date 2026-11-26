---
id: Exh-hdke
status: open
deps: []
links: []
created: 2026-11-26T21:29:28Z
type: bug
priority: 1
assignee: Max Omdal
parent: Exh-i8q4
tags: [e2e, ui, detail]
---
# Artifact bar (#313) broke three Playwright journeys: Share button never settles, stale Hide controls

After #313 (be813fd5, artifact bar) merged, make e2e on a fresh build of main fails three journeys that passed before it. Found while updating PR #314 (Exh-1zg6), whose diff touches neither spec. (1-2) e2e/specs/share-opening.spec.ts:4 'Share opens at the beginning and waits for deliberate text entry' (mouse and touch) time out at 30s. getByRole('button', {name: 'Share'}) resolves to <button class="pip-btn" id="share-open">, but Playwright reports 'element is not stable' on every retry. The artifact bar's controls apparently keep moving (shrink/resize), so a click never lands. Decide whether that is a real defect (a bar that never settles is also hard to tap) or a test that must wait for a settled state. (3) e2e/specs/ui-feedback.spec.ts:80 'history shares desktop columns and fits a narrow phone with enlarged text' waits for a 'Hide controls' button after pressing End in the More menu. Exh-i8q4 removed that item by design ('no stash tab and no Hide controls item'), so the journey is stale. Browser regression runs only post-merge, so these surface on the next push to main.

## Acceptance Criteria

- The cause of the Share button's instability is identified, and either the bar settles or the journey waits for the documented settled state, with the reason stated.
- share-opening passes in both the mouse and touch projects.
- The ui-feedback history journey asserts the artifact bar's actual menu, with no 'Hide controls'.
- make e2e on a fresh build of main has no failures beyond any tracked in Exh-wmw7.

