# Rebased light/dark visual council

The8/10 milestone below is complete. The user subsequently requested9/10;
[current follow-up](nine-summary.md).

Application candidate: `0110960247e5fdece7f88699c85afb865824f8e6`, based on
`origin/main` at `44def51afa43019771fd258bc61b28e3ebf4613b`.
The initial workflow council's old-base results are historical; this follow-up
uses the independent [visual-only rubric](visual-brief.md).

**Completed: two consecutive joint passes, rounds 15 and 16.** Codex Sol and
Claude Opus independently scored every category **8/10 in light and dark** on
the unchanged application `01109602`. All 48 qualifying scores were checked
against the four rendered-review reports. All required validation passes.

The table below applies independently to **both qualifying rounds**:

| Category | Sol light | Sol dark | Opus light | Opus dark |
| --- | ---: | ---: | ---: | ---: |
| Typography | 8 | 8 | 8 | 8 |
| Composition and spacing | 8 | 8 | 8 | 8 |
| Color harmony | 8 | 8 | 8 | 8 |
| Contrast and emphasis | 8 | 8 | 8 | 8 |
| Component visual consistency | 8 | 8 | 8 | 8 |
| Responsive visual polish | 8 | 8 | 8 | 8 |

Round 15: [Sol](round-15-sol-visual.md), [Opus](round-15-opus-visual.md).
Round 16: [Sol](round-16-sol-visual.md), [Opus](round-16-opus-visual.md).
[Full scores and actual task IDs](visual-scores.json).

Opus's first round-16 execution hit its quota and produced no scores. The fresh
post-reset review was independent, with Sol's same-round report and scores
withheld. [The failed attempt remains recorded](round-16-opus-unavailable.md).
Both temporary quota-continuation triggers were deleted first when they fired;
no scheduled automation remains.

## Council progression

Each model gives twelve scores: six categories independently in light and dark.
A joint round passes only when all 24 scores reach 8. Two consecutive joint
passes are required. Scores are judgments of the inspected app-owned surfaces,
with coverage and limitations preserved in the individual reports.

| Round | Application commit | Lowest Sol score | Lowest Opus score | Joint result |
| --- | --- | ---: | ---: | --- |
| 5: rebased baseline | `a65a7cfe` | 7 | 6 | Below threshold |
| 6: shared visual system | `7eaf8354` | 8 | 7.5 | Below threshold |
| 7: phone share refinements | `a242183a` | 8 | 7.5 | Below threshold |
| 8: verified phone layout | `4d725ba2` | 8 | 8 | First joint pass |
| 9: fresh confirmation | `4d725ba2` | 8 | 7 | Below threshold; streak reset |
| 10: restored syntax | `1c86da10` | 7.5 | Unavailable | Below threshold |
| 11: actual selection layer | `7e96d783` | 8 | 7.5 | Below threshold |
| 12: visible active-row selection | `b1b43d96` | 8 | 7.5 | Below threshold |
| 13: integrated editor chrome | `30bdaa5d` | 8 | 8 | First joint pass |
| 14: fresh confirmation | `30bdaa5d` | 8 | 7.5 | Below threshold; streak reset |
| 15: stable desktop rail | `01109602` | 8 | 8 | First joint pass |
| 16: fresh confirmation | `01109602` | 8 | 8 | Second consecutive joint pass |

[Sol round 11](round-11-sol-visual.md) independently measured minimum selected-token
contrast of 4.523:1 light and 4.696:1 dark on the actual painted layer. This is a
single-model pass; Opus subsequently identified the active-line overlap issue.

[All scores and task IDs](visual-scores.json) retain the full twelve-score
tables. [Sol round 8](round-8-sol-visual.md) and
[Opus round 8](round-8-opus-visual.md) record rendered checks in actual media
emulation, rather than accepting source declarations as proof.

## What changed

- Shared typography, field surfaces and control sizing replace inconsistent
  browser defaults. Quiet structural borders and distinct editable boundaries
  reduce dark-mode noise while retaining visible controls.
- Source editors use app surfaces, readable gutters/placeholder/doctype colors,
  and an integrated active-line treatment. Actual focused selections use the app
  background in both themes. Changing the system color preference
  preserves the unsaved source draft.
- Source syntax is fully differentiated in both themes. Actual selection remains
  visible under the translucent cursor row, with quieter repeated-word echoes
  and a clear current search match. Native Find/Replace, Go to line, completion
  and folding use shared type, surfaces and 32px/44px control density.
- Desktop navigation reserves room for full active labels and counts, keeping
  44px rows stable when switching spaces at 768px and 1280px. Phone tabs retain
  their independent layout and enlarged-text wrapping.
- Editor sections, history, settings facts, card metadata and mobile tab icons
  have consistent alignment and hierarchy. Default dark tiles use restrained
  color while user-authored tools retain their own design.
- Search has an integrated clear control, clearer owner attribution and
  contextual empty copy. Upstream Search, fixed mobile tabs, scrolling chrome and
  outside-press dismissal are preserved through the rebase.
- Phone Share choices stack at full width. URL and Copy heights match; the
  replacement-link helper sits beneath its button; shorter People placeholder
  and visible instructions avoid clipped copy. New geometry checks specifically
  cover the failed round-7 flex-layout assumption.

The complete responses are recorded in [round 6](visual-round-6-changes.md),
[round 7](visual-round-7-changes.md), [round 8](visual-round-8-changes.md), and
[round 10](visual-round-10-changes.md), [round 11](visual-round-11-changes.md),
[round 12](visual-round-12-changes.md), [round 13](visual-round-13-changes.md),
and [round 15](visual-round-15-changes.md).
Round 9 exposed the missing light syntax
colors; the corrected complete light and dark palettes now retain differentiated
syntax with measured token contrast on field, active-line and selection surfaces.

## Before and after

[Completed scorecard and interactive screenshots](visual-outcome.html).

Rebased-baseline comparisons include upstream changes on both sides. Targeted
regression comparisons use the actual preceding builds identified by their
capture folders. Current captures show `01109602`, the unchanged application
reviewed in rounds 15 and 16.

| Surface | Before | Current |
| --- | --- | --- |
| Dark editor, desktop | [Before](../../screenshots/Exh-zrjo/rebased-before/editor-1280-dark.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/editor-1280-dark.png) |
| Light editor regression from round 9 | [Missing colors](../../screenshots/Exh-zrjo/visual-round-8/editor-1280-light.png) | [Restored colors](../../screenshots/Exh-zrjo/visual-round-15/editor-1280-light.png) |
| Dark Share, phone | [Before](../../screenshots/Exh-zrjo/rebased-before/share-390-dark.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/share-390-dark.png) |
| Dark gallery, desktop | [Before](../../screenshots/Exh-zrjo/rebased-before/gallery-1280-dark.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/gallery-1280-dark.png) |
| Selected word on cursor row, light phone | [Before](../../screenshots/Exh-zrjo/active-line-before/source-word-selection-390-light.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/source-word-selection-390-light.png) |
| Desktop sharing navigation | [Before](../../screenshots/Exh-zrjo/visual-round-13/shared-1280-dark.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/shared-1280-dark.png) |
| Find/replace, dark phone | [Before](../../screenshots/Exh-zrjo/editor-chrome-before/source-find-390-dark.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/source-find-390-dark.png) |
| Repeated words, light phone | [Before](../../screenshots/Exh-zrjo/editor-chrome-before/source-occurrences-390-light.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/source-occurrences-390-light.png) |
| Light Share, phone | [Before](../../screenshots/Exh-zrjo/rebased-before/share-390-light.png) | [After](../../screenshots/Exh-zrjo/visual-round-15/share-390-light.png) |

A [navigation comparison](visual-navigation-comparison.html) isolates the rail
repair. An [interactive comparison](visual-comparison.html) presents actual editor,
gallery and sharing screenshots in both schemes.

The [final capture manifest](../../screenshots/Exh-zrjo/visual-round-15/manifest.json)
contains 110 authenticated states across light/dark and desktop/phone/narrow
layouts. Capture automation asserts actual color-scheme media queries and checks
document overflow and uncaught page errors; individual council reports distinguish
fresh live inspection from supplementary screenshots and previous evidence.

## Verification and scope

See [validation and raw logs](visual-validation.md) for build, Node, Go and
real-Chromium checks. Provider-dependent agent execution, experimental connectors,
arbitrary user-authored tool designs, alternate browser engines and hardware or
screen-reader certification are outside this visual pass.

Remaining minor opportunities include dense Share copy/secondary actions, the
narrow Edit tag footer, placeholder styling, native completion/Close alignment,
Duplicate form idioms, Search empty-state copy, and card-menu vertical alignment.
The reviewers judged these below the material threshold; all observations and
coverage limits remain in their reports. An 8 denotes a polished utility
interface, not perfection.

The reviews are independent samples rather than an exhaustive permutation
audit. Sol's confirmation covered both schemes at all four widths; Opus's
confirmation did not repeat 768px light. Their reports explicitly distinguish
fresh live checks from prior context and states not revisited.

Preview: http://t3code:18280 (render origin 18281), scratch login
`admin` / `changeme`. Data is isolated under `/tmp/exhibit-zrjo-data`.
Implementation is on `feature/Exh-zrjo/ui-council`; main remains clean and has
not been merged or pushed.
