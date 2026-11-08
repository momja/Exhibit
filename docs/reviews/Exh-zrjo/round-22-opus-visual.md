# Round 22 — independent Opus visual review (target 9)

Reviewer: **claude-opus-5-5**, 2026-11-04, about 20:46–21:05 UTC (live session).

**Application under review**

- Frozen application **17baf9306f455dbca4882a468e8ba2afe088436f**. Served assets **f1b6e9fd582c**, confirmed from live versioned asset URLs at entry (Library) and at exit (editor).
- Base: origin/main **44def51a**.
- HEAD at exit was **cc4c3ffb** (evidence commits only). A read-only `git diff 17baf930 HEAD -- internal web` was empty.

**What I read**

- `visual-brief.md` and `nine-brief.md`.
- R21 Opus and R21 Sol in full, and R20 Opus.
- Change notes 18–22 and `nine-validation.md`.
- Earlier rounds (5–20 reports, the R10/R18 Opus quota records, the R19 Opus deferral, and change notes 6–15) through header skims and the brief's full summary of findings and responses.

**What I did not read**

- `round-22-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md`
- the ticket and any parent aggregate

My scores are fresh. They were not shaped by any peer rating. Tests did not decide any score.

**Verdict: FAIL against the all-twelve-at-least-9 target.**

- Both R22 repairs are verified live and are good:
  - Share now opens at its top, on first open and on reopen.
  - The editor explanations use the shared helper token, with 12px paragraph gaps.
- Expanding the rest of the editor's **Stored state / Stored assets / Gallery widget** sections shows that this pocket still does not follow the shared grammar used everywhere else:
  - helper notes sit **0px** beneath the controls they describe;
  - a **solid red** destructive button sits among quiet-destructive conventions;
  - phone actions are **36px** where the rest of the same page uses 44px;
  - the inspector mixes **28px- and 40px-wide** icon buttons.

That holds three categories at 8.5 in both schemes. This single-model report establishes no joint pass and makes no council-completion claim. The incoming joint streak is zero.

## Twelve scores

| Category | Light | Dark | Weakest material surface |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | All helper copy is now on the shared token. Measured on the editor:<br>• 1280: 13/19.5.<br>• 320: 14/21.<br>• The receipt uses 14/600 headers.<br>• Page titles are 26/650.<br>Residue: the widget help uses 14/20.3 at 320, against 21 elsewhere. That is trivial. |
| Composition and spacing | **8.5** | **8.5** | **Helper notes after a control get no top gap.** Measured:<br>• Stored state: Save/Export/Erase row bottom y231 → Export note top y231.<br>• Stored assets: Export ZIP bottom y583 → note top y583. In the same section, the empty-state paragraph above those buttons has an 11px gap.<br>• Widget help: the tile ends at y241 and the help starts at y241, followed by about 35px of space before the source. The help therefore reads as belonging to the source below it.<br>• At 320 the Stored state note runs straight off the bottom border of the full-width Erase button. About 48px of air then follows before the divider. |
| Color harmony | **9** | **9** | A coherent slate/blue system in both schemes:<br>• muted presets and default tiles;<br>• integrated source;<br>• token-framed colour well;<br>• restrained shared-space highlight.<br>Dark Erase (`rgb(242,114,106)` fill) is loud, but within the palette. |
| Contrast and emphasis | **9** | **9** | Primaries lead. Share, Duplicate and Find show a clear hierarchy, and the current Find match leads. The Stored-state emphasis cluster is noted under component consistency rather than counted twice. |
| Component visual consistency | **8.5** | **8.5** | Destructive actions use three idioms:<br>• Settings: soft "Delete account" (44px, `btn-danger-soft`).<br>• Editor footer and Edit-tag Delete: quiet outline.<br>• Stored state "Erase all data": solid `btn-danger` (light `rgb(180,35,24)`, dark `rgb(242,114,106)`), placed beside two filled blue primaries (Add key, Save state).<br>The state inspector's record controls are uneven: 28×36 Move up/down beside 40×36 Remove record. The array-level "Remove property" × floats at x366 next to the `runs` label, not at the right edge where the key's trash sits. |
| Responsive visual polish | **8.5** | **8.5** | At 320 the editor stacks Save state, Cancel, Export JSON, Import JSON, Erase all data, Regenerate and Remove as full-width **36px** buttons. The same page's Add key, View/Restore, Return to Library and Delete are **44px**. Every other phone surface keeps the 44px standard, and the R18 response promised it. Share at 320×640, 390×640 and 640×360 is now correct (below). |

## Material finding: the editor's data and widget sections bypass the shared action and helper grammar

**Observed live.** Route: `/artifacts/461183aa…/edit`, with Stored state, Stored assets and Gallery widget expanded (local disclosure state only).

**1280×800, light**

- Help paragraphs: `form-help`, 13px / 19.5px, `rgb(93,104,120)`, with a 12px gap between paragraphs. This repairs R21 Sol's finding.
- The action rows and their notes touch:

| Element | Box (x, y, w, h) |
| --- | --- |
| Save state | 234, 195, 118, 36 |
| Export JSON | 628, 195, 131, 36 |
| Erase all data | 906, 195, 140, 36 (`btn btn-sm btn-danger`) |
| "Export JSON downloads…" note | top 231 → **0px** below the row |
| Export assets ZIP | y547, h36 |
| "Downloads all saved assets…" note | top 583 → **0px** |

- Add key is 100×44, sitting beside 36px Add property and Save state.
- Record controls: Move up/down are **28×36**; Remove record and Delete this key are **40×36**.

**320×740, dark** (`matchMedia` true)

- Save state, Cancel, Export JSON, Import JSON and Erase all data are each 268×**36**, full width.
- The salmon-filled Erase sits directly above a six-line note with no gap.
- Widget panel: Regenerate (disabled) and Remove are 36px. The help paragraph is 14 / **20.3**, with **0px** above it (tile y109–241, help y241–342).
- On the same page, View/Restore, Return to Library and Delete are 44px, and Delete is a quiet outline.

**Treatment to earn 9.** No ornament is needed.

1. Give `form-help` that follows a control row or block a top gap. The established 8–12px helper gap works. Keep the 12px gap between paragraphs. This applies to the state and asset notes and to the widget help.
2. Use the shared destructive idiom for Erase all data: soft or outline danger, like Settings and the editor Delete. Let Save state remain the single filled primary in that row.
3. Raise phone editor-panel actions to the 44px phone standard, as the rest of the page already does.
4. Size inspector icon buttons uniformly (for example 36×36), and right-align "Remove property" with the other row actions.

The behaviour, the text and the user-defined keys are unchanged by any of this.

## Prior-repair verification (fresh, live)

**Share opening — fixed, in both schemes**

| Viewport | Scheme | Result |
| --- | --- | --- |
| 320×640 | light | `scrollTop 0` (client 477 / scroll 1124); focus on header **Close**, `:focus-visible` false after a pointer open. The first frame shows the info notice, Link access, checkbox and explanation, URL + Copy, Replace link + helper, and Embed tile. |
| 320×640 | dark | Scrolled the body to its end: the grant row, both saved-data cards with radios inside, and the public-link helper; Done stays fixed. Closed with Done, reopened: `scrollTop 0`, focus on Close. |
| 640×360 | dark | Desktop sheet: header 77, body 172, footer 77. Opens at the notice. The body is small but intentional, and the notice and Link access are visible. |
| 390×640 | light | `scrollTop 0`, focus on Close. |

No People autofocus remains.

**Explanation rhythm.** Stored state, Stored assets and Versions paragraphs are on `form-help` with 12px separation, at 1280 light and at 320 dark (14/21). The R21 Sol 0px-between-paragraphs finding is repaired. The remaining issue is only the helper-after-control gap described above.

**Posture receipt (1280 dark).** The panel flips above the card inside the viewport. Headers are 14/600, the body uses comfortable helper type, and the Manage link sits at the bottom. The trigger is now a 32×32 target beside the 32×32 menu.

**Source Find (1280 dark).**

- The field flexes to filled Next, outline Previous and All, then the Close square.
- 18px option checkboxes.
- Replace row: Replace all ends flush with All.
- Source status stayed "All changes saved".

**Other surfaces**

| Surface | Viewport and scheme | What I saw |
| --- | --- | --- |
| Library | 1280 light | Four 236px columns; kebabs in 32px slots, optically level with the title line. |
| Shared with me | 768 dark | Active space label fits; Owners list and owner chip are clear. |
| Search miss | 320 light | Integrated clear, balanced copy, 44px Clear search, active Search tab. |
| Add, Agent mode | 390 dark | Selected segment, sentence-case captions, full-width primary/Cancel stack. |
| Card menu → Duplicate | 390 light | Coherent menu; the sheet has a fixed header, 18px checkboxes with helpers, primary/Cancel stack. Cancelled. |
| Settings | 320 dark | Aligned fact groups; soft destructive Delete account at 44px. |
| Versions | 320 dark | Clean phone grouping, tinted Current badge, 44px View/Restore. |

## Minor opportunities (none decisive on its own)

- Native dark unselected radio renders as a filled grey disc (Share saved-data). Retained, readable.
- At 640×360 landscape, Share's 77px header and footer leave a 172px body; compacting the vertical padding at short heights would help.
- Widget help line-height of 20.3px versus 21px elsewhere.
- Dark disabled "Regenerate" reads as a desaturated primary. This is acceptable, but sits beside the 36px issue.
- Uppercase fact eyebrows versus sentence-case field captions: consistent and defensible.

## Retained judgement calls, reassessed

- **240px rail at 768:** comfortable.
- **Quiet source echoes and active band:** not re-run as a paint matrix. The Find view supports the hierarchy.
- **User-authored tool and widget designs:** excluded. This includes the light Run Log body and the Reading List tile.

## Coverage and limits

**Session and data handling**

- I used only my own tab, `tab_1e_0d23e38c-d413-4e76-b0eb-59687a206343`. It was opened after `preview_status` with `reuseExistingTab:false, open:false`, its exact id was passed on every call, and it is now closed.
- I was already signed in.
- Scheme was set only with `preview_set_appearance`. `matchMedia('(prefers-color-scheme: dark)')` was confirmed false or true as expected at 1280, 640×360, 390 and 320. 768 dark was observed visually, not by `matchMedia`. I used no root-class simulation.
- Dialogs, menus, the popover and Find were dismissed unchanged. I typed only "run" into Find.
- I made no saves, creates, deletes, grants, link replacements, data-mode changes, builds, tests, commits, capture writes or subdelegation. **I wrote only this file.** Disclosures were opened locally only.

**What was observed live, and what was not**

- Evidence is live screenshots plus computed geometry and style. I used no saved captures.
- The 36px phone heights were measured at 320 dark only. Light is inferred from the identical class (`btn-sm`), whose 36px height was measured at 1280 light.
- Sampled rather than exhaustive:
  - Not re-checked this round: Tags panel and Edit tag, Filters, viewer receipt, URL/Paste Add modes, completion, Go to line, selection paint, admin users, Shared by me, 1280 Share.
  - Not exercised: long or arbitrary data, enlarged text, invalid or dirty states.
- Single Chromium engine with emulated viewports. No physical phone or software keyboard, alternate engine, hosted identity or accessibility certification.

## Scores (light / dark)

- Typography: 9 / 9
- Composition and spacing: **8.5 / 8.5**
- Color harmony: 9 / 9
- Contrast and emphasis: 9 / 9
- Component visual consistency: **8.5 / 8.5**
- Responsive visual polish: **8.5 / 8.5**
