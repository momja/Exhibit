# Round 15 — independent Sol visual review

Reviewer: **gpt-6.1-sol**, 2026-11-04, approximately 11:49–12:00 UTC.

Frozen application: `0110960247e5fdece7f88699c85afb865824f8e6`, branch `feature/Exh-zrjo/ui-council`, fetched `origin/main` base `44def51afa43019771fd258bc61b28e3ebf4613b`. Independently confirmed the initial HEAD and merge-base. HEAD advanced to `d9fcd8f8428cd52581546baf843b73318c85947a` during inspection; a final read-only diff excluding docs/tickets showed **no application changes** from the frozen candidate. Running preview: `http://t3code:18280`, served asset version **`150ec936001f`**, observed in the loaded asset URLs.

Read the [original visual brief](visual-brief.md), both reports for visual rounds 5–14, the round-10 Opus unavailability record, round 6/7/8/10/11/12/13/15 change notes, and [visual validation](visual-validation.md). Did not read the round-15 peer report, aggregate score/summary/resume contents, or the ticket. Earlier scores and supplied test results did not determine these ratings.

**Verdict: PASS for Sol. All twelve independent scores are 8. No substantial visual issue found in the freshly inspected app-owned surfaces.** The desktop navigation repair holds in rendered light and dark at both 1280px and 768px. Earlier source-selection and native-panel repairs also hold.

This is one model's result. Round 14 failed jointly, so the incoming qualifying streak is zero. Even if the independent peer passes round 15, a fresh joint round 16 on the unchanged application is still required. This report does not declare council completion.

## Twelve scores

Each category considers its weakest materially inspected surface. Strong pages do not compensate for a weak dialog or editor. An 8 means polished/coherent without a substantial visual issue; the localized finishing opportunities below prevent a higher rating.

| Fixed category | Light | Dark | Fresh rendered basis |
| --- | ---: | ---: | --- |
| Typography | 8 | 8 | Clear heading, section, field and metadata hierarchy; readable system controls and differentiated monospace syntax. Native source panels have comfortable label type. Dense Share explanations remain readable. |
| Composition and spacing | 8 | 8 | Active desktop labels now fit without moving later navigation rows. Cards, filters and Search retain sensible space beside the wider rail, including at 768px. Editor sections, history and stacked Share choices remain orderly. |
| Color harmony | 8 | 8 | Neutral shell, blue accents and restrained default tiles agree across cards, menus, dialogs and editor chrome. Dark source is integrated with the recessed field palette. Secondary code hints no longer introduce competing lime/yellow/orange emphasis. |
| Contrast and emphasis | 8 | 8 | Actual selected words remain visible beneath the active row, with quieter echoes. Current find results lead over other matches. Source/gutters, incoming-owner attribution, fields, primary actions and selected methods/choices remain discernible. |
| Component visual consistency | 8 | 8 | Cards, fields, menus and dialogs use compatible type, boundaries and surfaces. Native Find/Replace, Go to line and completion belong to the app's visual system. Small density, footer and Close-style differences remain minor. |
| Responsive visual polish | 8 | 8 | The desktop rail works at 768px without compromising card composition; 390/320px retain their separate aligned tabs. Both editors' Find panels remain contained with visible source. Narrow Search, history, Tags and Share retain readable measures. |

## Round-14 navigation finding — resolved on live evidence

**Routes:** Library `/` → Shared with me `/shared` → Shared by me `/shared-by-me`. Used the actual navigation links at desktop, with settled screenshots and measurements on all three destinations, **1280×800 and 768×900, both explicitly emulated schemes**.

| Live geometry, all three destinations, both desktop widths/themes | Result |
| --- | --- |
| Rail width | **240px** |
| Visible destination row width / height | **215px / 44px** |
| Library / Shared with me / Shared by me row top | **76 / 124 / 172px**, unchanged through navigation |
| Actual text-line rectangles per desktop label | **One**, including each active bold label |
| Active “Shared with me” text | x=50–174.14px, **124.14px** wide |
| Its count | x=210.65–219px, leaving about **36.5px** after the label |
| Document horizontal overflow | **0** in all inspected destination combinations |

The active label has no orphaned “me”; the third row no longer shifts down. Counts remain visibly separate and aligned. The wider rail is proportionate at 1280; at 768 the remaining area still accommodates an orderly two-column card grid, complete headings and the global Search field. I also inspected `/search?q=run` at 768 in both themes and an applied Daily tools filter at 768 dark: the field, chip, filter rail and result cards remain coherent.

**Phone retention, live:** Library and both sharing destinations at **390×844 and 320×740 in both themes**. The four tab icons stay on a common baseline; complete sharing labels occupy the reserved two-line area. The active tint is clear, cards and incoming attribution remain readable, and document overflow is zero. Intentional phone label wrapping is distinct from the repaired desktop reflow.

**Supplementary comparison only:** inspected the existing [round-13 dark desktop capture](../../screenshots/Exh-zrjo/visual-round-13/shared-1280-dark.png) and [current dark desktop capture](../../screenshots/Exh-zrjo/visual-round-15/shared-1280-dark.png). They show the old orphan/shift and the current single-line row, consistent with live inspection. No old application was launched. Live fixture counts are one digit; the supplied two-digit-count/enlarged-label regressions are validation context, not a fresh independent specimen in this review.

## Earlier editor repairs — independently retained

### Active-row word selection and repeated occurrences

**Live surfaces:** `/new` → Paste → `HTML source`; Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → `Artifact source`. Double-clicked an actual rendered word at **1280/390/320, both actual schemes**, confirming native selected text and `.cm-content` focus before accepting paint readings.

Paste used a temporary **186-character, nine-line HTML/CSS/JavaScript specimen without a trailing newline**. Selected the comment word `visitor`, repeated in nearby code/string/comment text. Existing source used `meta` on `<meta charset="utf-8">`, with an echo on the next row.

| Actual paint, both components/all three widths | Light | Dark |
| --- | --- | --- |
| Focused selection rectangle | `rgb(219,231,251)` | `rgb(39,54,74)` |
| Active row above selection | `rgba(240,243,248,.25)` | `rgba(35,43,54,.25)` |
| Selected-active composite | `rgb(224.25,234,250.25)` | `rgb(38,51.25,69)` |
| Echo fill | `rgba(35,85,158,.08)` | `rgba(126,170,240,.08)` |
| Echo composite on an ordinary row | `rgb(237.4,241.4,247.24)` | `rgb(34,40.28,49.56)` |
| Selected Paste comment contrast | **4.656:1** | **5.690:1** |
| Selected existing `meta` contrast | **4.869:1** | **7.385:1** |

These use the actual `elementsFromPoint` stack and computed backgrounds, composed back to front, not a root token substituted for final paint. The visible selected word is stronger than its faint echoes. The absent active-row selection and lime emphasis inversion do not recur. The complete light/dark syntax palettes remain visible: tags, strings, comments, CSS and JavaScript are differentiated.

Initial narrow existing-source coordinates landed in the sticky gutter after horizontal scrolling, yielding a newline selection and `.cm-scroller` focus. Those readings were rejected. Reset internal horizontal scroll, recalculated coordinates without scrolling the oversized source line into the gutter, and repeated successfully with selected `meta` and `.cm-content` focus at both phone widths. This was a review-positioning correction, not a UI defect.

**Select All:** the nonempty final `</script>` row in Paste remains visibly selected at 1280/390/320 in both schemes. In existing source, checked the tail at 390 in both schemes after Control+End, Control+A and Shift+Left, excluding the terminal newline so the head is on nonempty `</html>`. That row retains the blue selection cue under the translucent active band. Existing-source head coverage was 390 only; it was not repeated at every width. Scheme changes retained selected text and the displayed draft.

### Find/Replace and other native chrome

**Live in both components at all three widths/both schemes:** populated Find/Replace (`visitor` → `guest` in Paste; `resizeTimer` → `timer` in existing source), invoked Next, never either replacement action.

- Fields/actions use **14px system-ui**, flat fills, 6px radii and app boundaries. Desktop controls measure **32px**; phone controls and option-label targets measure **44px**. The old tiny gradient controls are absent.
- Current matches have the selection fill and a blue outline; other matches use the weak echo tint. The current result visibly leads.
- The responsive panel separates navigation, options and replacement actions without clipped labels or overlap. Source scroller height remains **269px desktop / 240px phone**, and measured document overflow is **0**.
- Query/replacement values and source survive actual scheme switches. Existing source remains **28,568 characters**, with **“All changes saved”**.

**Go to line, Paste:** inspected at 1280 and 320 in both schemes. Label, field, Go and Close remain complete and contained; entered `3:8` and navigated into CSS without editing. Its bordered Close still differs from Find's borderless Close.

**Completion, Paste, 320/both schemes:** Control+Space at that CSS position opens a populated property list. It has readable 14px system type, **44px rows**, a coherent selected tint and an opaque panel. The popup spans approximately x=58.6–320px: contained, but touching the right edge. No completion accepted. The high phone label alignment remains a minor finishing opportunity.

Paste was cleared with Control+A/Backspace. Final hidden `#body` length was **0**, with the empty placeholder present. Its 320px prompt remains horizontally clipped inside the scrollable editor in both schemes. Existing source was never edited or saved; its final checked length/status remained unchanged.

## Other fresh coverage and retained fixes

All entries below are **live rendered observations**:

| Surface | Coverage and judgment |
| --- | --- |
| Library and sharing spaces | Full destination matrix above. Even title areas, restrained dark boundaries/default tiles and clear incoming attribution. User-authored widget bodies excluded. |
| Card menu / Duplicate | Run Log menu at 390 dark; Duplicate at 390 both themes, dismissed unchanged. Consistent icon/label rhythm, layered surfaces, readable checkbox help and contained footer. Browser-native title selection is vivid blue in dark but readable. |
| Search | Blank, incoming `q=packing`, miss `q=not-in-this-library` at 320 both themes; owned `q=run` at 1280 and 768 both. Integrated clear, discernible owner chip, balanced empty-state hierarchy, aligned fixed tabs; zero overflow measured on narrow states. |
| Filters | Sheet at 390 both themes; applied Daily tools at 768 dark. Clear headings, collection/tag rows, dots and footer; no crowded rail/content relationship. |
| Add | Agent at 390 both themes and 1280 light; URL at 390 both themes and 1280 light; Paste matrix above. Clear method selection, readable fields and stacked phone actions. No execution, fetch or successful form submission. |
| Editor Tags / history | Tags at 390 both and 1280 dark; Edit tag Health at 320 both, dismissed; Versions/footer at 320 both. Full-width creation fields, matching phone Add/field heights and readable history grouping. Known tag-dialog footer issue remains below. |
| Settings | 1280 both through Profile/Administration/Storage/Sign out; 390 both through disabled Delete account explanation. Facts align on desktop and stack on phone; icons, card rhythm and restrained destructive entry agree with the shell. |
| Viewer | 390 toolbar, posture/About popover and More menu, both themes. Settled opaque surfaces and icon/label rhythm remain readable over the light artifact body. Rechecked dark More after its opening animation; its transient opacity was not scored as resting contrast. |
| Share | Top through bottom at 390/320 both themes; additionally desktop dark top/choices and desktop light lower portion. Helper is **6px below** Replace link on phones; URL/Copy are **44px**; choices stack at **312px / 242px** wide. At 320 both choices are 113px high. Full People placeholder/comma helper, section divider and sentence-case disclosure persist; dialog/document horizontal overflow is **0**. No sharing values changed. |

## Minor opportunities, freshly reassessed

None of these creates a substantially weak surface under the stated 8 threshold:

- **Share, both themes, 390/320:** the six-to-eight-line access explanation is dense but retains a readable measure and section separation. Replace link/Revoke remain **32px** against **44px** URL/Copy/Done. At 320, Revoke ends at x=266 while the section ends at x=281, a **15px inset**. More concise copy and unified secondary density/alignment would improve finishing.
- **Edit tag, both, 320:** Save wraps below Delete/Cancel at the left; solid Delete is prominent, especially in dark, and differs from the outlined editor-footer Delete. Prefer a deliberate narrow footer and quieter resting destructive emphasis. Current actions have complete labels, contained geometry and clear separation; the imbalance remains localized.
- **Editor footer, both, 320:** full-width Return to Library is left-aligned, unlike other full-width actions. It is clear as navigation, but centering or a more explicit navigation treatment would be more consistent.
- **Settings, both:** bold `admin` versus regular Storage values remains a small weight inconsistency.
- **Add/native fields, both:** italic Title/Search/Replace placeholders versus normal Description are inconsistent. The light selected Add method has a less emphatic border than the dark ring, but remains identifiable. Empty Paste prompt clipping at 320 is visible; the HTML source label still identifies it, and there is no document overflow.
- **Native source chrome:** phone completion labels sit above center and the popup can touch the viewport edge; Find leaves a gap before Close, and two-line options produce slightly uneven rhythm. Its Close differs from Go's. Controls/text remain complete and contained. Light active-line and secondary-match tints are very faint, but the actual selection/current match remains the stronger, visible cue; this quiet hierarchy is acceptable on fresh inspection.

The previously noted 320px existing-tag placeholder truncation was not separately remeasured this round; it remains prior context, not a new finding.

## Evidence and limits

- Used only my own T3 tab **`tab_w_0d23e38c-d413-4e76-b0eb-59687a206343`**, opened with `reuseExistingTab:false`; passed its exact ID on every subsequent browser call. No other tab was operated.
- Explicit `preview_set_appearance` calls and live `matchMedia('(prefers-color-scheme: dark)').matches` confirmed **false/true at 1280, 768, 390 and 320px**, including key navigation/source states. No root-class simulation.
- Primary evidence is fresh live screenshots plus actual text selection/focus, computed geometry and painted-layer composition. Supplementary capture inspection was limited to the navigation pair linked above; I did not inspect all 110 captures. No capture was written or overwritten.
- No runtime source inspection was used to declare unseen UI fixed. Git metadata/diff checks established application identity. Prior reports/change notes and supplied regressions are context; tests do not establish visual scores. No builds, lint or suites run, including the parent's Go suite; its completion was not independently checked.
- Coverage is sampled outside the stated matrices. No enlarged-label/two-digit-count specimen, arbitrary long data, dirty-Save/invalid/post-save states, applied filter miss, expanded Share embed/switch explanations, native completion detail pane, special-character/bracket variants, or fresh folding check. Go/completion were exercised in Paste rather than both components. Desktop Share was sampled, not exhaustively inspected in every scroll position/theme.
- Resizing preserves a desktop user agent/fine pointer. No physical phones, software keyboards, alternate engines or screen readers; no accessibility certification claimed.
- User-authored tool/widget body designs, credential-dependent agent execution and experimental connectors are excluded. The light Run Log body under dark preference is user content; app-owned chrome/source/cards/default tiles/dialogs remain included.
- No valid saves, creates, deletes, grants, link replacements, data-mode changes, persisted fixture changes, external URL fetches, commits, application edits or subdelegation. Temporary Paste content was cleared; dialogs were dismissed unchanged. **Only this assigned report was written.**
