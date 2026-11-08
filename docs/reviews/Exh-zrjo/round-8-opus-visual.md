# Exh-zrjo — Round 8 visual audit (Opus, independent)

Build: frozen `4d725ba2` on `44def51a`, app at `http://t3code:18280` (asset
version `9f7135f0fccb`). Rubric: the six categories in
[visual-brief.md](visual-brief.md), scored separately for light and dark from the
weakest material surface. No round-7 score is carried over.

## Method

- **Own tab**, opened with `reuseExistingTab=false`. Appearance was set with
  `preview_set_appearance` and checked each time with
  `matchMedia('(prefers-color-scheme: dark)')`: `true` in dark, `false` in light.
- **Observed and measured live:**
  - Run Log share sheet (`#share-panel`, opened with `showModal()`, closed with no
    changes) at **390px dark**, top to bottom, and at **320px light**.
  - Run Log `/edit` at **390px dark**, with Tags open, including the source editor and
    the footer.
  - `/search?q=run` at **1280px light**.
- **Not re-inspected this round:** `/new`, `/settings`, the card menu popover, the
  version history list, 1280 dark, and 320 dark Search. My earlier live checks cover
  these (round 6, and round 7 for 320 dark Search). The change notes and the
  narrowly scoped diff since round 7 say nothing else changed. That is supplied
  evidence, not a fresh observation.
- Nothing was saved, created, deleted, granted or switched.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition & spacing | 8 | 8 |
| Color harmony | 8 | 8 |
| Contrast & emphasis | 8 | 8 |
| Component visual consistency | 8 | 8 |
| Responsive visual polish | 8 | 8 |

## Round-7 blocker and minor items: verification (observed)

| Item | Status | Evidence |
|---|---|---|
| R1: Replace link helper wrapping beside the button | **Fixed** | `.share-replace-row` computes `display:flex; flex-direction:column`. At 390 dark, the button spans y 355–387 and the helper 393–407. At 320 light, the button spans 324–356 and the helper 362–376. "The current link stops working." is one line under the button, at the same left edge. |
| Copy and URL field heights | Holds | At 320 light, both are 272–316 (44px). The URL ends in an ellipsis. Dialog `scrollWidth − clientWidth = 0`. |
| Data-mode choices | Holds | They are stacked and full width at 390 dark. The selected card has an accent border and tint, the unselected one is quiet. |
| Create new tag fields stopping 18px short | **Fixed** | Name and the hex row both span 26–364, the full 338px section. |
| Phone footer targets | **Fixed** | "Return to Library" and Delete are both 44px. |
| Grant row divider | Fixed | The divider under `council-guest` is as wide as the section dividers. |

## Remaining observations (none substantial)

- **Objective, minor:** in the phone share sheet, the `council-guest` row ends at
  x=336, while its section and divider end at 351. So Revoke's right edge sits 15px
  inside the Copy and Done buttons above and below it. You can see it, but it does
  not break the layout.
- **Objective, minor:** on phones, Replace link and Revoke are 32px tall, while Copy,
  Done and the fields are 44px. As secondary actions they read as deliberately
  smaller and stay consistent with each other. This is a refinement, not a defect.
- **Aesthetic:** the five-to-eight-line link-access explainer still dominates the top
  of the phone sheet. It is readable, and I don't count it against the score.
- **Positive, observed:** the dark editor has calm recessed fields, clear section
  headings with sentence-case summaries, 44px Add and Create buttons beside 44px
  fields, and a legible source theme. On 1280 light Search, the clear button sits
  inside the field, the cards are even, the posture glyphs line up with the menu
  triggers, and the rail has aligned icons and counts.

## Verdict

All twelve scores are **8**. Light and dark both score 8 in every category. The one
thing holding back round 7, the phone Replace-link helper, now renders on its own
line in both themes at 320 and 390. I checked this on the rendered geometry, not on
the CSS. The minor round-7 objections are also fixed. I found no substantial visual
issue on any surface I inspected. **This is a passing round for Opus.** If Sol also
passes, it is the first joint pass. A second fresh confirmation round is still
required.
