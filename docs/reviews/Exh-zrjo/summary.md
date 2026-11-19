# UI council outcome (historical)

**Superseded for current evaluation:** these results describe the earlier local-main base and workflow rubric. The user requested a rebase and a visual-only light/dark council. See [the follow-up visual brief](visual-brief.md) and subsequent round-5+ reports.

Completed rebased light/dark evaluation: [visual council outcome](visual-summary.md).

The fixed threshold is met: **both Codex Sol and Claude Opus scored every category 8/10 in two consecutive independent rounds (3 and 4)**. All required checks passed. The application stayed frozen at `006b14ef` for both passing rounds.

Scores below are **Sol / Opus**, using the unchanged [scope and rubric](brief.md).

| Category | Baseline | Round 2 | Round 3 | Round 4 |
| --- | --- | --- | --- | --- |
| Visual hierarchy | 7 / 6 | 8 / 8 | 8 / 8 | 8 / 8 |
| Component consistency | 7 / 6 | 8 / 8 | 8 / 8 | 8 / 8 |
| Navigation/discoverability | 7 / 7 | 8 / 8 | 8 / 8 | 8 / 8 |
| Workflow feedback | 6 / 5 | 7 / 7 | 8 / 8 | 8 / 8 |
| Accessibility | 6 / 6 | 8 / 8 | 8 / 8 | 8 / 8 |
| Responsive behavior | 7 / 7 | 8 / 8 | 8 / 8 | 8 / 8 |

## What changed

- Shared header sizing, stronger focus/control boundaries, relative typography and consistent Library naming.
- Visible gallery context, labelled primary Add, readable rounded cards and stable visible card actions.
- Contextual empty states with htmx recovery, pending search feedback and live result announcements.
- Named source editors and field-associated validation with focus; guarded pending creation and working-copy recovery.
- Concise permission summary, quiet clean Save, responsive version history and separated deletion.
- Named floating viewer, larger touch controls, direct desktop editing and predictable keyboard menu behavior.

Round 2 caught a false upload announcement on invalid input. That was fixed and specifically covered by regression assertions before the two passing rounds. See [responses to round 1](round-2-changes.md) and [responses to round 2](round-3-changes.md).

## Evidence

- [Sol confirmation](round-4-sol.md) and [Opus confirmation](round-4-opus.md)
- [Sol first pass](round-3-sol.md) and [Opus first pass](round-3-opus.md)
- [Machine-readable scores and delegated task IDs](scores.json)
- [Validation and raw test output](validation.md): full Go short suite passed, 459 Node tests passed, 9 E2E tests passed, broad real-Chromium matrix passed.
- [Final screenshot manifest](../../screenshots/Exh-zrjo/round-3/manifest.json): 36 authenticated desktop/phone states, both themes, no captured document overflow or console errors.
- [Before desktop](../../screenshots/Exh-zrjo/baseline/gallery-1280.png) / [after desktop](../../screenshots/Exh-zrjo/round-3/gallery-1280-light.png)
- [Before phone](../../screenshots/Exh-zrjo/baseline/gallery-390.png) / [after phone](../../screenshots/Exh-zrjo/round-3/gallery-390-light.png)
- [Before validation fix](../../screenshots/Exh-zrjo/round-2/add-error-320-light.png) / [after validation fix](../../screenshots/Exh-zrjo/round-3/add-error-320-light.png)

Reviewer reports preserve observed, source-inferred and untested distinctions. Optional refinements remain documented there; neither reviewer identified a substantial unresolved blocker. Credential-dependent agent execution, experimental connectors, artifact-authored designs and hardware/screen-reader certification were outside this pass.

## Preview and reproduction

The isolated sample instance runs at http://t3code:18280 (render origin on port 18281), with the scratch login `admin` / `changeme`. Its data lives in `/tmp/exhibit-zrjo-data`, separate from existing instances. It includes dev samples, tags, a collection, incoming/outgoing shares and history.

After building, the existing repository browser workflow is `npm test --prefix e2e`. Capture the prepared sample instance with:

```sh
node e2e/capture-ui.mjs docs/screenshots/Exh-zrjo/round-3
```

The capture script uses `EXHIBIT_UI_ORIGIN`, `EXHIBIT_UI_USER` and `EXHIBIT_UI_PASSWORD` overrides when needed. It asserts authentication, expected routes, UI markers and horizontal fit before saving images. No production data is used by the fixture or regression tests.

Implementation and evidence are committed on `feature/Exh-zrjo/ui-council`. Main remains clean; application changes have not been merged or pushed to main.
