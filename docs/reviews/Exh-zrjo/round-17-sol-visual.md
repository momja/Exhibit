# Round 17 — independent Sol visual baseline for 9/10

Reviewer: **gpt-6.1-sol**, 2026-11-04, approximately 15:35–15:48 UTC.

Frozen application: `0110960247e5fdece7f88699c85afb865824f8e6`, branch `feature/Exh-zrjo/ui-council`, base `origin/main` at `44def51afa43019771fd258bc61b28e3ebf4613b`. Read-only Git checks found HEAD `02494b84041fea6c6b825635b71a26d51abc60a0` and no application differences under `internal/` or `web/` from the frozen application. Preview: http://t3code:18280; loaded asset version **`150ec936001f`**, also confirmed at the end.

Read the [original visual brief](visual-brief.md), [new 9/10 brief](nine-brief.md), both models' visual reports for rounds 5–16, the round-10 Opus unavailability record, round 6/7/8/10/11/12/13/15 change notes, and [validation context](visual-validation.md). Did not read the current peer report, nine aggregates/checkpoints, or ticket contents. Earlier scores and passing tests did not determine these ratings.

**Verdict: FAIL against the new all-twelve-at-least-9 target.** The application is polished, but repeated finishing weaknesses on dialogs, forms and native editor chrome prevent an excellent rating in five categories. Palette harmony independently earns 9 in both schemes. This is not an all-9 review, does not establish a qualifying joint round, and makes no council-completion claim. The incoming qualifying streak for 9 was zero.

## Twelve scores and what earns 9

Scores judge the weakest material surface, not an average. An 8 remains polished/coherent without a substantial issue; 8.5 is between that and excellent. The new goal does not change those meanings. The matching light/dark scores reflect weaknesses reproduced in both actual schemes, not an assumption that the themes are interchangeable.

| Category | Light | Dark | Why this score; specific treatment for 9 |
| --- | ---: | ---: | --- |
| Typography | **8.5** | **8.5** | Headings, readable system controls, metadata and differentiated source are strong. The weakest reading rhythm is Share's six/eight-line access explanation, followed by another accounts explanation, plus inconsistent field-label/placeholder idioms between Add, Duplicate and the editor. For 9: make Share's essential rules concise and scannable without shrinking type; use one field-label hierarchy by role and a consistent placeholder convention. Balance Search's narrow empty-copy line breaks. |
| Composition and spacing | **8** | **8** | Library grids, aligned Settings facts, stable navigation and phone history grouping are orderly. Edit tag's incidental 320px footer wrap, Duplicate's uneven narrow stack, Share's inset Revoke and the newly observed desktop Versions action wrap are visible exceptions. For 9: explicit responsive footer layouts, common section edges, an intrinsic-width desktop history action group, and a lighter upper Share composition. |
| Color harmony | **9** | **9** | The neutral shell, blue emphasis, restrained default tiles, source wells, syntax and overlay surfaces already form an excellent coherent palette in each scheme. The version-preview amber has a clear semantic purpose. No new palette is needed to earn 9 here. Preserve this system while correcting action hierarchy; the prominent tag Delete is an emphasis problem, not evidence that the whole palette needs replacement. |
| Contrast and emphasis | **8** | **8** | Ordinary text, fields, owner attribution, real source selection and current find results are readable. The weak surface is Edit tag: a solid red/coral Delete leads the first footer row while Save sits alone below it. Tags also presents two filled local actions of similar weight. For 9: one clear primary per immediate task, quieter resting destructive actions, and a deliberate Save/Cancel/Delete arrangement. Strengthen the light method-selected state only if its resting distinction remains weak after focus is removed; do not rely on a focus ring as selection. |
| Component visual consistency | **8** | **8** | Shared surfaces/type and the repaired native editor are coherent, but Duplicate's field label/checkbox scale, mixed phone action heights, full-width labels with different alignment, sibling Close styles and above-center completion labels still expose separate implementations. For 9: shared field/choice/footer conventions and a final native-chrome alignment pass, keeping the existing native behavior. |
| Responsive visual polish | **8** | **8** | The 1280/768 rail and separate phone tabs work, cards remain readable, and inspected app-owned regions stay contained. Excellent responsive craft needs more than containment: Edit tag and Duplicate need intentional narrow footers; empty Paste/tag prompts must fit; native completion needs inset/centering; Share needs a more economical phone reading order and consistent action density. |

The palette scores are fresh judgments of the rendered palette, not a reward for previous repairs or an inference from the old all-8 milestone. No decorative additions or new product features are required for the remaining categories.

## Findings material to the 9/10 target

These are necessary finishing work for an excellent interface, not claims that every observation is a new under-8 usability defect.

### 1. Dialog action layouts still look incidental

**Observed live: Run Log `/edit` → Tags → Edit tag Health, 320×740, both schemes.** All controls remain legible and inside the dialog, but Delete and Cancel form the first row and Save wraps beneath Delete at the left.

- Delete: x=39, y=438.2, **102×44px**.
- Cancel: x=195.6, y=438.2, **85.4×44px**.
- Save: x=39, y=490.2, **90.4×44px**.
- Delete is solid `rgb(204,0,0)` in light / `rgb(242,114,106)` in dark, while the artifact editor's own Delete is outlined. The dark coral has especially conspicuous surface weight.

This arrangement makes the destructive action look like the leading task and the actual commit action like an afterthought. Use an explicit narrow footer: a full-width Save, a consistent Cancel, and a separated quiet destructive action; preserve the conventional split at desktop. The exact order can follow the product's established dialog convention, but it must be designed rather than produced by flex wrapping.

**Observed live: `/` → Run Log card menu → Duplicate.** At 1280 in both themes the modal is readable but its action pair is left-aligned, unlike Add's right-aligned desktop form footer and Share's trailing Done. At 390 dark the pair fits; at **320 light** Cancel wraps above a wider Duplicate artifact button, both at the left and neither spanning the field. Adopt a deliberate narrow action stack and one dialog-footer convention. I did not claim a fresh 320-dark Duplicate check.

**Related fresh observation:** Return to Library at the 320px editor footer and Done in the 390px Filters sheet both have full-width outlines but left-aligned labels. Add's full-width actions center their labels. Either center full-width buttons consistently or give navigation a clearly distinct link treatment. Filters Done should not acquire a different alignment accidentally.

### 2. Share is orderly but too explanation-heavy for excellent phone composition

**Observed live: viewer `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332` → `#share-panel`, 1280/390/320, both schemes, top through bottom.**

At 390 the Link access explanation takes **six lines**; at 320 it takes **eight**, before the URL and its actions. People then repeats the connected-account distinction, and another substantial block follows the data-mode choices. The type remains readable, but repeated prose carries more visual weight than the actual choices and actions. Section headings are relatively small/muted beside those paragraphs.

For 9, keep the essential saved-data/access warning visible while compressing repeated wording into a short rule and a clearly subordinate supporting note. Keep the per-person versus shared-data consequences explicit. Establish a clearer heading → setting → short helper rhythm; do not shrink body text or hide the essential warning to save space.

Phone controls expose the remaining density/alignment mismatch:

- URL/Copy/Done are **44px**; Replace link and Revoke are **32px**.
- At 320, Revoke ends at **x=266**, while the choices/section end at **x=281**: **15px inset**. The same inset is visible at 390 and desktop.
- The complete helper is correctly below Replace link, so this is not the old orphaned-word failure returning.

Give phone secondary actions a consistent 44px visual rhythm, retaining their secondary styling, and align the grant-row action to the same trailing edge as the other controls. This would improve the whole sheet more than adding ornament or stronger borders.

### 3. Desktop Versions has a previously unreported action-width mismatch

**Observed live: Run Log `/edit` → Versions, 1280×800, both actual schemes.** View and Restore stack vertically at the right of the older version, although there is ample desktop space. The current row remains compact while the older row becomes **96px** high.

Computed rendered geometry explains the symptom:

| Element | Width / height |
| --- | --- |
| `.version-actions` | **176×72px**, flex with wrapping and 8px gap |
| View | **75.85×32px** |
| Restore | **94.33×32px** |
| Pair plus gap | **178.18px**, exceeding the 176px allocation |
| `.version-row` | **812×96px** |

In the measured scroll position, View begins at y=711.5 and Restore at y=751.5. Screenshots show the same stacked pair in light and dark. This is live DOM/computed-layout evidence; I did not infer the symptom from source or modify the layout to demonstrate it.

For 9, let the desktop action track accommodate the intrinsic group width and keep the pair on one intentional row when space permits. Use a deliberate responsive arrangement when it does not. Do not fix this by clipping labels, shrinking type, or making enlarged labels unable to wrap. The phone version/title/date/badge/action grouping remains coherent and should be retained.

### 4. Forms need one visual grammar, including empty states

**Observed live across `/new`, `/settings`, editor Tags, Duplicate and Edit tag.**

- Duplicate's Title label is **12px regular sentence case**; Add uses small bold uppercase labels, while Artifact title and tag fields use another label treatment. Different information roles can differ, but the same editable-field role should have a clear common hierarchy. For example, use a shared medium-weight sentence-case field label, and retain uppercase micro-labels specifically for metadata/facts.
- Italic Title/URL/People/Search/Replace placeholders coexist with an upright Description prompt. Choose one convention for editable-field prompts; visible labels should carry the meaning.
- **320, both schemes, empty Paste:** `Paste HTML artifact source here…` clips horizontally, with source scroller **288px scrollWidth / 266px clientWidth**. A short `Paste HTML here…` prompt would fit without changing code scrolling.
- **320 dark, Tags freshly observed:** the existing-tag select displays `Choose an existing ta`, cutting the final word rather than presenting a finished prompt. The adjacent Add is 44px and creation fields now reach the section edge, so the older pairing repair holds. Use a shorter prompt such as `Choose a tag…`.
- Settings still presents **bold admin** against regular Storage values. This is a small hierarchy inconsistency, not a legibility defect.
- Duplicate's native checkboxes are **13px**. Fresh measurement also found Share's link checkbox **13px**, so I do **not** repeat the prior claim that Duplicate is smaller than Share. Native source-panel checkboxes are 16px. A shared choice-control scale and label rhythm would finish this family, without requiring custom checkbox graphics.

**Search miss `/search?q=not-in-this-library`, 390, both schemes:** the helper leaves **“tag.”** alone on its final line. Clear search is **36px** beneath a **44px** field. Shorten/balance the helper, e.g. `Try another title, keyword, or tag.`, and apply the same phone action density. The integrated field clear and readable empty-state hierarchy remain good.

### 5. Native editor chrome needs finishing, not another redesign

**Observed live: Run Log source, Find/Replace at 390/both; Go to line and completion at 320/both.** The former tiny gradient chrome remains repaired. These are small but repeated exceptions to excellent component craft:

- At 390, Find ends at **x=245.7** while Close begins at **x=309**, leaving about **63px** between their edges. A `flexible field + fixed Close` first row would use the space more deliberately.
- Equal-width options force Match case and Whole word onto two lines while Regex remains one. The labels would fit more naturally at intrinsic widths; wrap whole option units at the narrow breakpoint instead of squeezing their text into equal thirds. Preserve readable type and the retained source viewport.
- Find's Close is borderless; Go to line's Close is bordered. Use one sibling-panel Close treatment.
- At 320, completion reaches **x=320**, exactly the viewport edge. Its rows are **44px**; the 16.5px label begins **7.5px** below row top, placing it about **6.25px above center**. Center the row contents and retain a small viewport inset. The current popup remains readable and contained; no completion was accepted.

Library/card footers have a related alignment opportunity at 1280/768: the reserved title area works, but the kebab glyph sits visibly lower than the title's first line/tag dot. Align those optical centers while retaining the complete two-line title reservation and usable trigger area. Phone one-line card footers already look more balanced.

## Previous repairs retained on fresh evidence

- **Desktop rail:** `/shared` at 1280 in both themes, and actual link switching through Library/incoming/outgoing at **768 in both themes**. Visible rows remain **44px at y=76/124/172**, with one real label line and separate counts. No orphaned “me” or displaced third row. At 768 the two roughly 232px card columns remain readable.
- **Phone navigation:** 390 Library and 320 incoming/outgoing shares, both schemes. Fixed tabs have aligned icons, reserved wrapping labels and clear active treatment. No app document overflow in measured samples.
- **Source syntax/selection:** existing Run Log at 1280/both. Actual double-click selected `meta` on the active row, with `.cm-content` focused. Selection visibly leads over the next-row echo. Computed selection is `rgb(219,231,251)` / `rgb(39,54,74)`; active fill remains translucent at .25, and echoes remain .08 accent-family tints. The rendered source exposed eight light / nine dark span colors, including base ink. This was not a repeat of the full painted-composition/token-contrast matrix.
- **Find:** at desktop light and 390/both, query `meta`, replacement `link`, Next invoked without replacement. Current match visibly has the outline and stronger fill. Native fields/actions are flat/readable; phone targets are 44px and the source viewport remains **240px**. Query/replacement survived actual scheme changes.
- **Share:** complete People prompt/comma helper; correct sentence-case disclosure; full-width stacked choices; 44px paired URL/Copy. At 390 the helper begins **6px below** Replace link. At 320 the choices measure **242×113px**. Measured dialog horizontal overflow is zero; the old narrow text towers and ragged helper are absent.
- **Editor/history/Settings:** clear section dividers and summaries, full-width tag creation fields, contained phone history, aligned desktop Settings facts and stacked phone facts. The desktop history action-wrap finding above does not negate the aligned timestamp repair.
- **Viewer:** settled More and posture/About popovers at 320/both use readable opaque app surfaces. Opening-animation frames were discarded in favor of settled snapshots. The light Run Log body under dark preference is user-authored content and was excluded.

## Optional refinements and fresh limits on earlier observations

- **240px rail at 768:** it consumes 31.25% of the viewport, but the remaining two columns are comfortable. A fluid track is optional taste/space optimization, not a required 9-level repair in my judgment. Retain intrinsic room for bold labels and counts.
- **Light code hints/active band:** both remain very faint. The selected word/current result leads clearly, so I do not require stronger secondary paint. If changed, preserve the verified selection hierarchy and text readability.
- **Native input text selection:** the vivid browser blue was observed in Duplicate and Find. App-family selection styling would be a small optional integration detail; it is not a reason to replace the palette.
- **Light Add-method selection:** at rest the border/icon treatment is less emphatic than dark's extra ring, but the method remains identifiable. A subtle persistent tint/check is optional if needed after focus is removed; copying a focus ring into the selected style is not necessary.
- Completion details, special-character indicators, arbitrary bracket variants and folding were not freshly exercised this round. Their prior acceptance is not fresh proof of excellence.

## Actual coverage, evidence and limits

Used only my own T3 tab **`tab_13_0d23e38c-d413-4e76-b0eb-59687a206343`**, opened after `preview_status` with `reuseExistingTab:false`; its exact ID was supplied on every subsequent browser call. Explicit `preview_set_appearance` and live `matchMedia('(prefers-color-scheme: dark)').matches` confirmed **false/true at 1280, 768, 390 and 320px**. Sizes were 1280×800, 768×900, 390×844 and 320×740. No root-class substitution.

Fresh coverage was broad and sampled, not an exhaustive cross-product:

| Surface | Live coverage |
| --- | --- |
| Library/cards/spaces | Library at 1280 light, 768/both, 390/both; incoming at 1280/768/320 both; outgoing at 768/320 both. Actual desktop link switches at 768. |
| Card menu/Duplicate | Menu at 1280 light; Duplicate at 1280/both, 390 dark and 320 light, dismissed unchanged. |
| Search | Blank and owned `q=run` at 1280/both; blank at 320 light; incoming `q=packing` at 320 dark; miss at 390/both. |
| Add | Agent and URL at 1280/both; all three methods, including empty Paste, at 320/both. No execution, fetch or submission. |
| Settings | 1280/both top through Sign out; 390/both top through the full disabled Delete explanation. |
| Editor/tags/history | 390 dark editor overview; Tags at 320 dark; Edit tag at 320/both; history/footer at 320/both and desktop/both. Read-only version preview opened and returned to current at 1280/both. |
| Native source | Existing source selection/echoes at 1280/both; Find at desktop light and 390/both; Go/completion at 320/both. No source edits or completion/replacement accepted. |
| Viewer/Share/Filters | Viewer toolbar/More/About at 320/both; default Share top through bottom at 1280/390/320 both; Filters sheet at 390/both. |

Primary evidence is live browser images plus actual DOM geometry/computed styles. I did not inspect production CSS/JS to declare a visual symptom or fix; the Versions explanation comes from computed layout, not a speculative source reading. No supplementary screenshot files were inspected or produced this round. Existing 110 captures and supplied test results remain context only.

The initial shared browser login expired during navigation; I reauthenticated using the supplied scratch credentials, then repeated the affected Search observations. Login redirects were not counted as Search renders or visual defects. Locator mistakes were corrected using actual page selectors; no failed probe was treated as evidence.

Not every opposite-size/theme variant was freshly reviewed: notably 320-dark Duplicate, light incoming Search, dark blank phone Search, 768 forms/dialogs, expanded Share embed/switch explanations, collection-filter results/misses, dirty-Save/invalid/post-save states and enlarged/arbitrary-long content. Paste was inspected empty rather than repeating its populated selection matrix. No physical-phone, software-keyboard, alternate-engine, screen-reader or accessibility certification; freeform resizing retains a desktop user agent/fine pointer.

User-authored widget/tool bodies and credential-dependent/experimental execution are excluded. No builds, tests, lint, commits, application edits, capture writes, persisted fixture changes, external URL fetches or subdelegation. No valid save/create/delete/grant/link replacement/data-mode change. Run Log remained **28,568 characters / “All changes saved”** after source navigation and read-only version preview; empty Paste was **0 characters** and never populated. Dialogs/menus were dismissed unchanged. **Only this assigned report was written.**

## Recommended coherent change groups

1. **Necessary for 9 — finish dialog/action hierarchy.** Give Edit tag and Duplicate intentional phone footers, quiet resting Delete, a shared desktop dialog-footer convention, and consistent full-width button label alignment, including Filters Done and Return to Library. Preserve existing operations and dismissal behavior.
2. **Necessary for 9 — lighten Share's reading hierarchy.** Condense repeated accounts/data explanations while retaining the essential warnings; strengthen section hierarchy; align Revoke to the section edge and give phone secondary actions the same 44px rhythm as their neighbors.
3. **Necessary for 9 — consolidate forms and empty copy.** Reuse field-label, placeholder and choice-control conventions by role; shorten the clipped Paste/tag prompts; balance Search empty copy and its phone action height; normalize comparable Settings value weights.
4. **Necessary for 9 — finish editor/history layout.** Size the desktop history action track intrinsically; center/inset completion rows; let Find use the field-to-Close space and intrinsic option widths; unify native Close styling. Preserve syntax, selection, current-match emphasis and the 240px phone source viewport.
5. **Small final craft pass — card footer alignment and restrained states.** Align desktop card title/glyph centers and verify resting method selection independently of focus. A fluid 768px rail, stronger secondary code tints and custom native-input selection are **optional taste**, not prerequisites; keep the already excellent palette rather than introducing a new one.
