# Edit tags: two consecutive joint 10/10 reviews

The latest tag-only target is met. Codex Sol and Claude Opus independently awarded all seven categories 10 in light and dark in rounds 36 and 37 on the **same unchanged application**. All 56 original table values, report identities and required coverage were verified. This is a council judgment of the inspected scope, not a universal claim of perfection.

| Category | Sol36 L/D | Opus36 L/D | Sol37 L/D | Opus round 37 L/D |
| --- | --- | --- | --- | --- |
| Typography | 10/10 | 10/10 | 10/10 | 10/10 |
| Composition and spacing | 10/10 | 10/10 | 10/10 | 10/10 |
| Color harmony | 10/10 | 10/10 | 10/10 | 10/10 |
| Contrast and emphasis | 10/10 | 10/10 | 10/10 | 10/10 |
| Component visual consistency | 10/10 | 10/10 | 10/10 | 10/10 |
| Responsive visual polish | 10/10 | 10/10 | 10/10 | 10/10 |
| Tag workflow clarity and feedback | 10/10 | 10/10 | 10/10 | 10/10 |

Application: `f58de10bf88acff268b9339e0c7dae406a93c664`; assets `4beedb277545`. Branch `feature/Exh-zrjo/ui-council`, stacked on [PR #273](https://github.com/momja/Exhibit/pull/273) at `ab0135c9`. Entire runtime/validation scope and live executable identities remained unchanged across both rounds, without exclusions.

## What changed

- The selection check is inset with matching radii and clears the color sample. Named preset radios, custom captions and a separate focus ring make selection legible.
- Pills fit their labels, long names wrap, and Edit/Remove align in separate utility slots. The existing-tag Add row now shares the desktop column edge and remains full width on phones.
- The panel states immediate add/remove persistence; the sheet states library-wide editing and names its action Save tag. Clean, invalid, dirty and pending states are distinct.
- Field errors replace hints nearby without shifting the sheet. Incomplete hex clears the stale sample/caption; Create and Save use consistent validity.
- A rejected name cannot bypass disabled Save through Enter. Partial create/attach failure retains feedback and selects/focuses the same new tag for retry, without a duplicate.
- Delete uses a same-sheet confirmation with safe Keep focus and preserved drafts. Server templates/htmx, Stimulus, native modules and self-hosted Phosphor conventions remain intact.

## Independent evidence

[Sol36](round-36-sol-tags-ten.md), [Opus36](round-36-opus-tags-ten.md), [Sol37](round-37-sol-tags-ten.md), [Opus round 37](round-37-opus-tags-ten.md). [All 56 rating verification](tags-ten-final-rating-verification.json) includes report hashes, actual tables, source identity and coverage. Each reviewer used an independent fresh task and owned tabs; current same-round peer results were withheld. All actual task IDs and attempts remain in [scores](tags-ten-scores.json).

Each reviewed Main at 1280×800,768×900,390×844,320×740 in both actual prefers-color-scheme settings: Tags panel and Edit sheet, including fully exposed lower controls. Each also viewed four isolated long-name/available-tag sample pairs at 1280/320 in both schemes, with local selection/restoration and complete lower controls. No hidden frame, unviewed resize or other-scheme substitution qualified.

**Opus round 37 execution caveat:** the provider task ended failed at a usage limit, after delivery. Public tool activity confirms both tabs closed, source/hash cleanup completed, and the assigned full report write completed at 22:06:57.231UTC; quota followed at 22:06:58.063UTC. The entire authored content exactly matches the saved report. Parent verified all 12 media pairs and actual image coverage. The report qualifies as a completed independent review; the original failed execution was **not** relabeled successful. No missing scores were inferred and no retry was requested. [Delivery audit](round-37-opus-tags-ten-delivery-audit.md), [public event proof](round-37-opus-tags-ten-delivery-audit.json), [original status](round-37-opus-tags-ten-status.json). Earlier no-report quota failures remain unscored.

## Validation

| Check | Result |
| --- | --- |
| Full build | PASS · 17.61s |
| Icon registry/served CSS + native Tag modal | PASS · 20.14s; actual1280/390 browser subtests, no skips |
| Complete browser suite | PASS · 24 tests · 119.39s |
| Standard Go short suite | PASS · exit 0 · 441.10s; API 426.015s / store 188.443s, entire source fingerprint unchanged |
| Gallery Node suite |479 PASS carried from round 35 on unchanged JS/tests; explicitly not a fresh CSS-only rerun |

[Validation and receipts](tags-ten-validation.md), [complete one-file change/carry proof](tags-ten-r36-scope-change.json), [runtime identity](tags-ten-runtime-identity.json). Meaningful regressions failed before functional fixes and pass after; assertions were not weakened. Parent persistence tests use isolated disposable resources and cover pending locks, real409/503/retry, global changes on two artifacts, association-only Remove, Delete preserving artifacts and htmx feedback. Reviewers themselves persisted nothing.

## Actual before/after

- [Palette, pills, scope and feedback](../../screenshots/Exh-zrjo/tags-ten-comparison-round33.html).
- [Stable invalid-color feedback and grouping](../../screenshots/Exh-zrjo/tags-ten-comparison-round34.html).
- [Neutral sample and rejected-name recovery](../../screenshots/Exh-zrjo/tags-ten-comparison-round35.html).
- [Final shared-column alignment](../../screenshots/Exh-zrjo/tags-ten-comparison-round36.html).

These are real source-keyed screenshots, previewed at desktop/phone widths without console errors and published in-thread. Current column measurements show 81px desktop overhang before and 0px after at every measured width/scheme, with no document overflow or tag API writes.

## Limits and handoff

The reviews cover Chromium emulation and local working copies, with explicit source-derived versus personally viewed observations. Native color-dialog UI, Safari, physical phones and screen readers are not certified. Persisted server outcomes are parent disposable-test evidence. Broader historical 9/10 coverage was superseded by the user’s narrower instruction and is not retroactively claimed complete.

The ticket is closed on the feature branch. Main stays clean/on main; no main merge or push and no new PR. Existing PR #273 remains linked and unwatched. The temporary continuation was permanently deleted; no task-owned recurring automation remains. Preview samples are retained.
