# Round 12 — independent Sol visual review

Reviewer: **gpt-6.1-sol**. Reviewed 2026-11-04, approximately 10:18–10:27 UTC.

Frozen application: `b1b43d96863c754d872a8c7bed842d99228384e7`; base `origin/main` at `44def51afa43019771fd258bc61b28e3ebf4613b`. Independently confirmed HEAD and merge-base. Final read-only Git inspection still showed that application commit, with changes confined to review/validation documents. App: `http://t3code:18280`; observed asset version `ec71042deca4`.

Read the [fixed visual brief](visual-brief.md), both models’ round 5–11 reports (round 10 Opus is an unavailability record, without scores), the round 6/7/8/10/11/12 change notes and [visual validation](visual-validation.md). Did not read the current peer report, aggregate score/summary/resume contents or the ticket. Previous ratings and supplied test results did not determine these scores.

**Verdict: PASS for Sol. All twelve scores are 8. The active-line selection defect is resolved in fresh live renders. No substantial visual blocker found on the inspected app-owned surfaces.** The incoming joint streak is zero. This report can contribute to a first joint passing round only if the independent peer also passes; a separate fresh joint confirmation remains necessary. It does not declare council completion.

## Twelve independent scores

Each category judges its weakest materially inspected surface, without averaging flaws away. An 8 means polished/coherent without a substantial visual issue; the remaining refinements below do not justify 9–10 ratings.

| Fixed category | Light | Dark | Current visual basis |
| --- | ---: | ---: | --- |
| Typography | 8 | 8 | Clear page, section, label and metadata hierarchy; compatible system controls; readable source and explanatory copy. Dense Share text remains comfortable to read, although lengthy. |
| Composition and spacing | 8 | 8 | Aligned desktop card title areas and Settings facts; clear editor divisions and narrow history grouping. Share choices have full-width measures and the replacement helper occupies its own row. |
| Color harmony | 8 | 8 | Coherent neutral shell and blue emphasis. Light syntax remains fully differentiated; dark source fits the recessed field palette. Default tile tints retain identity without overwhelming the shell. |
| Contrast and emphasis | 8 | 8 | A selected word now visibly stands out on the active source row. The selected head row also retains its cue. Code colors remain readable; headings, selected choices, primary actions and owner attribution have clear emphasis. |
| Component visual consistency | 8 | 8 | Source selection now behaves visually in both editors and themes. Fields, buttons, cards, menus and dialogs share compatible type/surfaces. Phone URL/Copy and tag Add/field pairs align. |
| Responsive visual polish | 8 | 8 | Sampled 1280/390/320 layouts retain readable measures, aligned navigation icons, integrated Search clear, reflowed history and stacked Share choices. The isolated tag-dialog footer wrap is a finish issue, without overlap or clipped actions. |

## Round-11 blocker — verified resolved live

**Routes:** `/new` → Paste → `.cm-content[aria-label="HTML source"]`; `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → `.cm-content[aria-label="Artifact source"]`.

**Coverage:** both editors at **1280×800, 390×844 and 320×740**, with actual **light → dark → light** switching. Every key combination checked the browser’s media query, native selection, focused contenteditable, active row and rendered selection layer. No root-class substitution.

In Paste I entered a short, temporary HTML/CSS/JavaScript specimen containing comments, tags, attributes, strings, units, a function, an escape and a regexp. The final 183-character specimen had **no trailing newline**. I double-clicked the actual comment word **“council”** using its rendered text coordinates. In the existing editor I double-clicked **“doctype”** on the first source row, without editing the source. `getSelection().toString()` returned the intended word and the row was `.cm-activeLine`.

The word visibly has a pale blue selection in light and a darker blue selection in dark, instead of blending into the full-width active band. At the word, `document.elementsFromPoint` showed the actual paint order: token span → translucent active line → transparent content → selection rectangle → editor background. I composed those computed backgrounds from back to front, including the active-line alpha; I did not substitute a root token for the painted result.

| Both source surfaces, all three widths | Light | Dark |
| --- | --- | --- |
| Actual active-line fill | `rgba(240,243,248,0.25)` | `rgba(35,43,54,0.25)` |
| Actual focused selection rectangle | `rgb(219,231,251)` | `rgb(39,54,74)` |
| Selected-active composited paint | `rgb(224.25,234,250.25)` | `rgb(38,51.25,69)` |
| Active row without selection underneath | `rgb(251.25,252,253.25)` | `rgb(28.25,32.5,38.25)` |
| Selected comment/meta foreground | `rgb(94,104,119)` | `rgb(164,174,189)` |
| Foreground contrast on selected-active paint | **4.656:1** | **5.690:1** |

These measurements agreed at 1280, 390 and 320 for both source components, including the return to light. The selected/unselected colors are visibly different; the fix does not depend merely on a rectangle existing behind opaque paint. The emphasis is restrained in dark, but clearly discernible at the selected word.

**Select All and its head:** in the short Paste specimen, Control+A selects through the visible `</script>` final row. Its text has selection beneath the active-line fill in both schemes at all widths. The existing artifact ends with a newline. I first moved to the end and used Control+A, then used **Shift+Left once** to exclude that final newline and put the selection head on the nonempty `</html>` row. That row was visibly selected at all three widths in both schemes, and its actual overlap paint matched the table. This adjustment changed only the selection, not the source. It avoids claiming that a selected long file’s opening rows prove the head-row overlap.

Switching schemes preserved the selected word/head and exact Paste draft. The specimen still showed **eight light / nine dark** distinct syntax span colors, including differentiated tags, strings and JavaScript. Existing opening HTML/CSS showed **seven light / eight dark**; its selected JavaScript tail also retained differentiated colors. The missing-light-highlighting and stock-selection-fill regressions remain repaired. These are sampled grammar/rendering observations, not exhaustive token certification.

Afterward the Paste buffer was cleared: hidden `#body` length **0**, placeholder present. Existing source stayed length **28,568**, with **“All changes saved”**; it was selected and scrolled only.

## Other retained fixes and fresh observations

All observations here are from fresh live screenshots, with supporting geometry for Share:

- **Share, 390/320, both themes:** inspected top, People, choices and bottom. The replacement helper remains **6px below** the button. URL and Copy are both **44px** high; the resting URL visibly ellipsizes. Choices stack at **312px wide at 390 / 242px at 320**, with clear selected/unselected treatments. Both choices are 113px high at 320. Dialog horizontal overflow measured **0** in all four combinations. People’s complete placeholder, comma helper, full-width divider and sentence-case disclosure remain present. No sharing control or value changed.
- **Editor:** section headings/dividers and source at desktop and phone widths; Tags at 390 in both themes; Edit tag dialog at 320 in both themes; Versions/footer at 320 in both themes. Creation fields reach the section edge and Add/Create pair with 44px controls. Narrow history clearly groups version, label, date, badge and actions. The tag dialog was dismissed unchanged.
- **Library and overlays:** desktop library in both themes; Run Log card menu at 1280 dark; Duplicate at 390 in both themes, dismissed. Card title reservations and quiet dark boundaries hold. Phone library at 390 in both themes and Filters sheet at 390 in both themes retain coherent spacing, labels, color dots and aligned fixed-tab icons.
- **Search:** blank, incoming `q=packing`, and miss `q=not-in-this-library` at 320 in both themes; owned results `q=run` at 1280 light. Clear remains integrated, the owner chip is discernible and empty-state typography has comfortable measures. The three narrow Search states measured zero document overflow in both schemes.
- **Shared spaces:** `/shared` at 390 dark and `/shared-by-me` at 390 light. Card title/attribution, restrained default tile and fixed-tab presentation remain coherent.
- **Settings:** 1280 top/Profile/Administration/Storage/Sign out, both themes; 390 top through Delete account, both themes. Desktop fact columns align and phone facts stack. Heading icons, secondary actions and restrained destructive entry agree with the shell; the disabled-delete explanation is readable.
- **Add:** Agent at 1280 light and 390/320 both themes; URL at 390 both themes; populated Paste matrix above, plus empty Paste at 320 both themes. Method selection, field rhythm and stacked phone actions remain orderly. No agent was started, URL fetched or form submitted.
- **Viewer chrome:** 320 dark toolbar; 390 dark About/posture popover and light More menu. App-owned labels, icons and layered surfaces remain readable over the artifact body. The light Run Log body under dark preference is user content and excluded.

## Reassessed aesthetic opportunities — no material blocker

- **Share, light/dark, 390/320:** the access explanation occupies six lines at 390 and eight at 320. It dominates the upper sheet, but line height, measure and section breaks remain clear. Shortening it would improve the composition’s lightness. Replace link/Revoke remain **32px** versus **44px** fields/Copy/Done; Revoke ends **15px inside** the section edge. The compact pair remains legible, separated and internally consistent. These are density/alignment refinements, not a substantially weak surface or a real-device touch-target certification.
- **Edit tag, light/dark, 320:** Save wraps below the Delete/Cancel row, at the left. A deliberate stacked footer would produce a better action hierarchy; the solid Delete is also visually strong. All labels remain complete, controls are contained and spacing is clear. I judge this isolated layout untidy but still within the defined 8 threshold.
- **Settings, both themes, 1280/390:** bold `admin` versus regular Storage values is a small weight inconsistency, without confusing hierarchy.
- **Add, both themes:** italic Title versus normal Description placeholders remain inconsistent. The empty source prompt is clipped horizontally at 320 within the scrollable code field; its visible HTML source label and opening prompt still identify it. Neither causes document overflow or a material legibility problem, but a shorter prompt would be cleaner.

## Evidence distinctions and limits

- Used only my own T3 tab, **`tab_o_0d23e38c-d413-4e76-b0eb-59687a206343`**, opened with `reuseExistingTab:false`, passing its ID on every subsequent preview call. No other tab was operated. Explicit `preview_set_appearance` and live `matchMedia('(prefers-color-scheme: dark)').matches` confirmed false/true at **1280, 390 and 320**, especially throughout the source-selection matrix. Browser resizing retains a desktop user agent and fine pointer.
- Scores use fresh rendered screenshots and computed paint/geometry. **Supplementary capture comparison only:** inspected the existing light-1280 and dark-320 word-selection before/after PNG pairs. They show the old absent cue and new visible word cue, consistent with my live findings: [light before](../../screenshots/Exh-zrjo/active-line-before/source-word-selection-1280-light.png), [light after](../../screenshots/Exh-zrjo/visual-round-12/source-word-selection-1280-light.png), [dark before](../../screenshots/Exh-zrjo/active-line-before/source-word-selection-320-dark.png), [dark after](../../screenshots/Exh-zrjo/visual-round-12/source-word-selection-320-dark.png). I did not inspect all 80 supplied captures. No capture was written or overwritten.
- No runtime source inference was used to declare a visual fix. Read-only Git checks established the candidate/base and absence of application changes during review. Historical reports, change notes and regression results provided context; passing tests were not visual proof. No build, lint or test suite was run, including the parent’s Go run.
- Coverage is sampled, not every route/state/theme/width permutation. Desktop URL/Share, opposite-theme shared-space/popover samples, expanded embed/switch disclosures, applied filter results/misses, invalid form states, dirty-Save emphasis, enlarged text and post-save states were not freshly inspected. Actual touch devices, software keyboards, alternate engines and arbitrary long data were not tested.
- App-owned shell/cards/default tiles, source/editor/history/tags, menus, dialogs and viewer chrome are included. User-authored tool/widget body designs, provider-dependent execution and experimental connectors are excluded.
- No valid saves, creates, deletes, grants, replacements, data-mode changes, persisted fixture changes, app edits, commits, external URL fetches or subdelegation. **Only this assigned report was written.**
