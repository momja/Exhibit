# Round 13 — independent Sol visual review

Reviewer: **gpt-6.1-sol**. Reviewed 2026-11-04.

Frozen application: `30bdaa5dfdbed35f3e14cf3e0c2183fffe5bf6d9`; base `origin/main` at `44def51afa43019771fd258bc61b28e3ebf4613b`. Independently checked HEAD and merge-base. App: `http://t3code:18280`; observed asset version `356ca1fa22d1`. The final read-only diff from the candidate contained evidence/ticket files, with no application source changes.

Read the [fixed brief](visual-brief.md), both models’ round 5–12 reports (round 10 Opus is the unavailability record), round 6/7/8/10/11/12/13 change notes, and visual-validation.md. Did not read the current peer report, aggregate scores, summary, resume, or ticket contents. Previous ratings and supplied test passes do not determine these scores.

**Verdict: PASS for Sol. All twelve scores are 8. No substantial visual blocker found in the freshly inspected app-owned surfaces, including ordinary native editor chrome.** This can contribute to a first joint passing round only. The incoming qualifying joint streak is zero; the independent peer must also pass, and a separate fresh joint confirmation on the unchanged application is still required. This report does not declare council completion.

## Twelve independent scores

Each category judges its weakest materially inspected surface; strong pages do not average away a weak editor or dialog. An 8 means polished/coherent without a substantial visual issue. The dense utility forms and remaining finish details do not warrant 9–10.

| Fixed category | Light | Dark | Fresh visual basis |
| --- | ---: | ---: | --- |
| Typography | 8 | 8 | Clear page, section, label and metadata hierarchy. Native find/navigation/completion now use readable system type; source retains readable monospace syntax. The tiny native controls from round 12 are gone. |
| Composition and spacing | 8 | 8 | Editor panels group fields, navigation, options and replacement actions clearly. Desktop cards and Settings facts align; narrow history and stacked Share choices remain orderly. |
| Color harmony | 8 | 8 | Neutral surfaces and blue accents remain coherent across shell, dialogs and source chrome. Repeated occurrences and find hints use restrained blue-family tints; no lime/yellow/orange editor emphasis appeared. |
| Contrast and emphasis | 8 | 8 | The actual selection/current find result leads over secondary occurrences. Active-row selection stays visible. Syntax, gutter, field text, owner attribution and primary actions remain readable and appropriately emphasized. |
| Component visual consistency | 8 | 8 | Native editor fields/buttons, line navigation and completion menus now belong to the same system as app controls. Flat fills, system type, radii, focus treatment and casing agree in both schemes. |
| Responsive visual polish | 8 | 8 | Both source components’ native panels remain contained at 1280/390/320. Phone controls reflow at 44px, with source still visible. Search, fixed tabs, sharing and history retain readable narrow layouts. The isolated Edit tag footer wrap remains a minor refinement. |

## Round-12 findings — independently verified resolved

### Repeated occurrences are quieter than the actual selection

**Observed live:** `/new` → Paste, `.cm-content[aria-label="HTML source"]`; Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → `.cm-content[aria-label="Artifact source"]`. Checked **1280×800, 390×844 and 320×740, both actual schemes**.

In Paste, used a temporary six-line HTML/CSS/JavaScript specimen with a repeated `visitor` identifier, comment, string, function and regexp. Removed the input tool’s added terminal newline, leaving an exact **156-character draft with no trailing newline**. Double-clicked the rendered comment word `visitor`. In existing source, double-clicked the tag name `meta` in `<meta charset="utf-8">`; its other occurrence is on the next row. Verified native selected text and `.cm-content` focus before accepting measurements. An initial coordinate attempt landed on the scroller; those unfocused results were discarded and the check repeated with corrected coordinates.

The selected word has a discernible blue cue; echoes are faint and do not compete. At narrow widths, an occurrence outside the horizontal source viewport was not treated as a visible paint sample. The next-row echo remained visible and was sampled.

| Actual rendered paint, both source components/all three widths | Light | Dark |
| --- | --- | --- |
| Focused selection rectangle | `rgb(219,231,251)` | `rgb(39,54,74)` |
| Active-line fill above selection | `rgba(240,243,248,.25)` | `rgba(35,43,54,.25)` |
| Selected-active composite | `rgb(224.25,234,250.25)` | `rgb(38,51.25,69)` |
| Secondary occurrence fill | `rgba(35,85,158,.08)` | `rgba(126,170,240,.08)` |
| Visible echo composite on the field | `rgb(237.4,241.4,247.24)` | `rgb(34,40.28,49.56)` |

These composites use the real `document.elementsFromPoint` stack and computed backgrounds, from back to front, including the translucent active row. They are not values resolved from root tokens alone. Live screenshots agree with the emphasis difference. Paste’s selected comment foreground `#5e6877` / `#a4aebd` has **4.656:1 / 5.690:1** contrast on the selected-active paint. Existing selected `meta` has **4.869:1 / 7.385:1**. Those are sampled foregrounds, not an exhaustive grammar audit.

### Find/replace is integrated and readable

**Observed live:** Control+F in both source components, populated Find and Replace, at **all three widths in both schemes**. Paste used `visitor` → `guest`; existing source used `resizeTimer` → `timer`. Invoked Next to inspect a real current match; did not invoke either replacement action.

- Fields and action labels compute to **system-ui, sans-serif, 14px**; ordinary fields/buttons are **32px high at 1280** and **44px at 390/320**. Close uses a readable 20px glyph within the corresponding target. Phone checkbox labels are 44px high, with readable two-line Match case/Whole word labels where necessary.
- Buttons are flat (`background-image:none`), with 6px radii and app surfaces/borders/focus treatment. Next has primary emphasis; the other actions remain secondary. Sentence-case labels read Next, Previous, All, Replace, Replace all, Match case, Regex and Whole word.
- Desktop navigation sits beside Find; phones put navigation, options, replacement input and replacement actions in distinct rows. At 320, Previous remains complete, labels do not overlap, and the controls stay within the panel. The Find field leaves some open space before Close, but the grouping remains clear.
- Measured source scroller height with find open: **269px at desktop, 240px on phones**, in both components. Opening search no longer consumes or clips away the source viewport. The lower portion is reached by ordinary page scrolling.
- Noncurrent `.cm-searchMatch` uses the weak tint above. The actual `.cm-searchMatch-selected` has the selection fill and a visible blue outline. On screen, the current result leads; the stock yellow/orange treatment is absent.
- Switching actual light/dark retained the exact Paste draft, Find value and replacement value. Existing source remained **28,568 characters**, with **“All changes saved”**. Document horizontal overflow measured **0** throughout the find matrix.

## Earlier selection/syntax repairs and other native chrome

**Selection overlap remains fixed:** Control+A in the short Paste specimen includes its nonempty final `function … </script>` row. That head row was visible and selection-blue at 1280/390/320 in both themes. In existing source, moved to the end, selected all, then used Shift+Left to exclude its final newline and put the head on `</html>`. The visible head row retains the selected-active composite in the table at every width/theme. This inspected the overlap on the head itself, rather than assuming that selected opening rows prove it. Native selection and draft survive scheme changes; the desktop Paste head was additionally checked light → dark → light.

**Syntax remains differentiated:** rendered HTML tags, strings, CSS values/properties and JavaScript keywords/functions/regexp remain colored in both schemes in the specimen and existing source. The light editor is not the flat, unhighlighted surface from round 9. Source/gutter colors remain readable on the app field surface. I did not independently calculate every syntax token’s contrast.

**Line navigation, live in both editors/all three widths/both schemes:** Control+Alt+G opens the native Go to line panel. Its label, flat field, Go and Close controls share the find panel’s type and density; they remain complete and contained at 320. Entering `2:10` in Paste and `27:5` in the existing source navigated to the expected CSS position without changing content.

**Completion, live in both editors/all three widths/both schemes:** Control+Space at those CSS positions opens a populated property list. The list has a coherent panel, clear selected-row tint and readable 14px system type. Measured rows are approximately **32.5px desktop / 44px phone**. It repositions within the viewport, including a right edge at 320px on the narrow view, with zero document overflow. Scheme changes preserve the open list and source. The last partly visible row indicates a scrollable list, not a clipped action label. No completion was accepted.

**Folding, live in existing source at 320/both schemes:** folded the CSS `body` block. The ellipsis is a small, quiet field-colored control, with readable secondary ink in both schemes. Unfolded it afterward; source and saved status stayed unchanged. Bracket/special-character indicators and completion detail side panels were not separately exercised, so I do not claim fresh verification of those additional styling changes.

## Other app surfaces freshly revisited

These are live rendered observations, not inherited ratings:

- **Library:** light desktop at review entry, dark desktop with Run Log card menu, and 390px library both schemes. Titles reserve orderly areas, card metadata aligns, menu icon/label rhythm is consistent, and dark structural boundaries remain quiet. Default monogram tiles are restrained enough to fit the neutral shell. User-authored widget bodies were excluded.
- **Duplicate:** 390px, both schemes, opened from Run Log’s menu and dismissed unchanged. Title field, copy options, explanatory text and footer remain comfortably composed.
- **Shared spaces:** `/shared` and `/shared-by-me`, 390px, both schemes. Full-width cards, owner attribution and fixed-tab presentation are coherent.
- **Search:** `/search`, incoming `?q=packing`, and miss `?q=not-in-this-library`, 320px in both schemes; owned `?q=run`, 1280px both schemes. Clear remains inside the field, the owner chip is discernible, empty states have readable measures and tabs align above complete wrapping labels. All three narrow states measured zero document overflow.
- **Editor:** Tags at 390px both schemes, Edit tag Health at 320px both schemes, Versions/footer at 320px both schemes. Creation fields reach the section edge; paired Add/field heights agree. Versions group the number, title, timestamp, badge and actions clearly. Tag dialog was dismissed without changes.
- **Settings:** 1280px Profile/Administration/Storage/Sign out in both schemes; 390px top through Delete account in both schemes. Desktop fact columns align, phone facts stack, heading icons agree and the disabled deletion explanation remains readable.
- **Add:** Agent at desktop light and 390px both schemes; URL at 390px both schemes; populated Paste matrix above. Selected methods, field rhythm and stacked phone actions remain coherent. No agent was started, URL entered/fetched or creation submitted.
- **Viewer:** 390px toolbar, posture/About popover and More menu in both schemes. Settled app-owned surfaces remain readable over the light Run Log body. The artifact body staying light under dark preference is user content, excluded from scoring.
- **Share:** 390/320px, both schemes, top through bottom. Replacement helper remains **6px below** its button. URL/Copy are **44px**. Choices stack at **312px / 242px** wide; at 320 both are 113px high. People’s full placeholder and comma helper, full-width divider and sentence-case disclosure remain present. Dialog horizontal overflow is **0** in all four combinations. No sharing value or action was changed.

## Reassessed minor aesthetic opportunities

None of these makes an inspected surface substantially weak under the stated 8 threshold:

- **Share, 390/320, both schemes:** the access explanation remains a large six-to-eight-line block. It is readable and separated well, but shorter copy would make the top of the sheet lighter. Replace link/Revoke remain **32px** versus **44px** fields/Copy/Done. Revoke is visibly inset from the section edge. The compact secondary pair is legible and separated; a more uniform phone density/alignment would improve finishing.
- **Edit tag, 320, both schemes:** Save wraps beneath Delete/Cancel, at the left, while Delete has strong solid emphasis. A deliberate stacked footer and quieter resting Delete would be more balanced. Current labels remain complete, contained and clearly separated; this isolated wrap is untidy rather than a material responsive breakdown.
- **Settings, both schemes:** bold `admin` versus regular Storage values remains a small weight inconsistency.
- **Add, both schemes:** italic Title versus normal Description placeholder remains inconsistent. The earlier horizontally clipped empty source prompt at 320 was not freshly reopened there this round; it remains a prior observation, not newly verified evidence.
- **Completion, 320:** the popup can meet the viewport’s right edge. A small inset would be more comfortable, but the inspected list remains readable and contained.

## Evidence distinctions, coverage and limits

- Used only my own T3 tab, **`tab_r_0d23e38c-d413-4e76-b0eb-59687a206343`**, opened with `reuseExistingTab:false`; supplied its ID on every subsequent preview call. No other tab was read or operated.
- Explicit `preview_set_appearance` and live `matchMedia('(prefers-color-scheme: dark)').matches` confirmed **false/true at 1280, 390 and 320**, including both source components. No root-class substitution. Resizing retains a desktop browser user agent and fine pointer.
- Primary proof is fresh live screenshot inspection plus computed geometry and actual painted-layer composition. Read-only source search was limited to locating native editor command/configuration context; no visual fix is declared from a CSS declaration alone.
- **Supplementary captures:** inspected the supplied 320px light find before/after pair: [before](../../screenshots/Exh-zrjo/editor-chrome-before/source-find-320-light.png), [after](../../screenshots/Exh-zrjo/visual-round-13/source-find-320-light.png). The old capture has tiny gradient controls and yellow/orange results; the current capture has legible app controls and restrained blue results, consistent with the live review. Also inspected the 320px light single-word-selection pair; those particular images do not show repeated identifiers and are not proof of echo hierarchy. Did not inspect all 104 captures or overwrite any capture.
- Coverage is sampled outside the detailed source matrix. Desktop URL/Share, expanded embed/switch disclosures, collection/filter overlays and applied-filter results, invalid forms, dirty-Save emphasis, enlarged text, post-save states, arbitrary long content and every opposite-size variant were not freshly inspected. Actual touch devices, software keyboards and other browser engines were not tested. Completion detail panels and special-character/bracket states remain explicit native-chrome limits above.
- User-authored tool/widget body designs, provider-dependent execution and experimental connectors are excluded. App-owned source, shell, cards/default tiles, dialogs, menus, history/tags and viewer chrome are included.
- No build, lint or test suite was run. Supplied regression results are context only. No valid saves, creates, deletes, grants, link replacements, data-mode changes, fixture changes, app edits, commits, external URL fetches or subdelegation. Temporary Paste content was cleared, with final hidden `#body` length **0** and placeholder present. Existing source stayed unchanged and saved. **Only this assigned report was written.**
