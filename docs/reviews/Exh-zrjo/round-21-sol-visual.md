# Round 21 — independent Sol visual review, target 9

Reviewer: **gpt-6.1-sol**, 2026-11-04, approximately 20:26–20:36 UTC. Frozen application **74544552a816a3ac3a6dd0c04f8d0713fe223cce**, served assets **a48067e4e6f5**, base **44def51afa43019771fd258bc61b28e3ebf4613b**. HEAD started at the frozen application and advanced to evidence commit **26d9530b939d16ce059b19e05e55c5f4f327fb79**. Read-only diffs against the candidate, and the uncommitted diffs, were empty under `internal/` and `web/`. Live asset URLs matched at entry and exit. Preview: http://t3code:18280.

**Verdict: FAIL against the all-twelve-at-least-9 target.** The round-21 receipt, helper, color-control and sheet repairs are genuinely improved. Most of the inspected interface is excellent. However, expanding the editor's Stored assets and Stored state reveals another material pocket of the old helper treatment: long explanations at 12px with normal line height, with separate paragraphs running directly together. This leaves Typography, Composition, and Component visual consistency at **8.5 in each scheme**. It is finishing work on an existing app-owned surface, not a request for ornament or new functionality.

This single-model review establishes neither a joint pass nor council completion. The incoming joint target-9 streak is zero. Earlier all-8 results and round-20 Sol's all-9 result were not inherited.

## Twelve independent scores

Scores consider the weakest material surface, without averaging the expanded editor sections away. Matching theme scores follow actual light/dark inspection.

| Category | Light | Dark | Fresh basis and treatment needed for 9 |
| --- | ---: | ---: | --- |
| Typography | **8.5** | **8.5** | The shared captions, page headings, dialogs, receipts and source chrome now have a careful hierarchy. Stored state/assets still introduce substantial explanatory copy at **12px / normal**, about **14px per line**. Apply the established helper rhythm to these explanations, including the export/import notes. |
| Composition and spacing | **8.5** | **8.5** | Cards, history columns and sheets are orderly. Stored assets merges its explanation, deletion warning and empty message with **zero paragraph separation**; the two export/import notes also touch. Give these distinct roles explicit vertical grouping and a comfortable helper rhythm. No structural redesign is needed. |
| Color harmony | **9** | **9** | Slate neutrals, restrained blue emphasis, muted presets/default tiles, integrated source and the now token-framed native color well are harmonious. No palette replacement is warranted. |
| Contrast and emphasis | **9** | **9** | Primary/secondary/destructive actions and selected states lead appropriately. Actual source selection and the current Find match lead over quiet echoes. Share's complete choices and radios remain together. The remaining explanation issue is reading rhythm/grouping, not a demonstrated contrast failure. |
| Component visual consistency | **8.5** | **8.5** | Receipts, Filters, Tag, Duplicate, Share and native source panels now belong to one family. The editor's Stored state/assets help still bypasses that family, visibly differing from Versions help in the same editor. Use the common helper treatment and section spacing here too. |
| Responsive visual polish | **9** | **9** | Both desktop widths and the two phone widths have deliberate navigation, card, sheet, history and source-panel layouts. Color fields align; phone receipts fit; actions stack intentionally; Share's lower content is reachable. The explanation density is counted under typography/grouping rather than asserted to be a separate responsive breakage. |

## Remaining material finding: expanded editor explanations retain the old dense treatment

**Observed live:** Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → **Stored assets**, `#assets-panel p.text-sm.muted`, at **1280×800 and 320×740, light and dark**. Also observed the opening **Stored state** explanation, `#state-panel p.text-sm.muted`, at **320 dark** while inspecting the existing object/array form without changing a value.

The Stored assets section is not user-authored artifact content. Its explanation, deletion warning, empty-state message, export/import buttons and notes are application UI. With the section expanded:

- The explanation about binary payloads and continued operation uses **12px system-ui, line-height normal**. At 1280 it spans three lines in **42px**, and at 320 it becomes a tight seven-line paragraph.
- The separate deletion warning follows immediately, then the empty-state message. All five paragraphs in this section have computed **margin-top: 0px; margin-bottom: 0px**.
- The two notes below Export assets ZIP / Import assets ZIP also follow directly, with no visual separation between the download explanation and replacement warning.
- The Stored state introduction has the same **12px / normal** treatment. Its controls remain orderly; I am not asking to restyle user-defined object keys as app headings.

Actual 1280 dark geometry, with the complete section visible:

| Paragraph | Top / bottom | Line rhythm | Gap from previous paragraph |
| --- | --- | --- | ---: |
| Binary-payload explanation | **336.5 / 378.5px** | 3 lines in 42px | — |
| Deletion warning | **378.5 / 406.5px** | 2 lines in 28px | **0px** |
| No stored assets message | **406.5 / 420.5px** | 1 line in 14px | **0px** |
| Download explanation, below actions | **468.5 / 482.5px** | 1 line in 14px | — |
| Import replacement note | **482.5 / 496.5px** | 1 line in 14px | **0px** |

Light renders the same compact block. At 320 in both schemes the wrapped explanation and warning read as one continuous small-text mass. This is more than an isolated short caption: the first explanation and warning together contain roughly eighty words, before the empty message and the separate action notes. In the same editor, Versions uses **13px / 19.5px** at desktop with paragraph spacing. The newly repaired Share/receipt helpers are also visibly more comfortable. That contrast within the product is the remaining consistency gap.

**Concrete treatment for 9:** apply the existing helper type/line-height convention to these app-owned explanations (including the corresponding Stored state notes), separate the explanation, warning and empty-state roles with the established paragraph gap, and group the export/import supporting notes so their distinct meanings remain apparent. Preserve the existing text, actions, native data editing and quiet palette. Recheck the expanded sections at 1280 and 320 in both themes. No new feature, stronger color, reduced text size or decorative panel is necessary.

These are live rendered symptoms and computed layout measurements. I did not edit CSS/DOM to simulate a fix or infer the symptom from production source. The small, compressed paragraphs remain readable and contained, so this is **8.5-level finishing**, not a broken-sheet/under-8 defect.

## Round-21 repairs independently verified

- **Card receipt:** Library `/` → Run Log's actual `role=button` posture/sharing trigger. At **1280 light/dark**, the open panel is **x264, y191.09, 320×323px**, bottom **514.09px**, and document height remains **800px**. It opens above the trigger instead of extending the document. Body is **13px / 19.5px** with readable 14px section heads. At **768 both**, it remains contained at x264, y406.69. After the phone breakpoint closed it, I explicitly reopened it: **320 both**, x16–304, y215–600, with **14px / 21px** body copy. At **390 light**, x16–336, y340–704. The wider measure and spacing make the actual Sharing paragraph comfortable. The first unsuccessful CSS locators assumed a native `button`; these were corrected to the observed role-based trigger and were not treated as UI failures.
- **Viewer receipt:** explicitly opened and inspected at **1280, 390 and 320 in both schemes**. The title/date, sentence-case Sandbox posture heading, helper paragraph and Manage link have a clear hierarchy. The settled surfaces are opaque and readable. Some resize operations close this existing viewer popover; those closed frames were not counted as receipt evidence, and I reopened it at the new size. An opening-animation light frame was likewise replaced with a settled check.
- **Share helper family:** at **320 light**, Replace's helper and both People helpers compute to **14px / 21px**, all **rgb(93,104,120)**. The checkbox explanation follows its title as a description, with no old 42px blank gap. Live top/bottom inspection at every width/theme confirms the coherent treatment in dark too.
- **Paste footer:** `/new`, both **1280 and 320, both themes**. The network note now has comfortable helper type. On phone it sits **above** the consecutive full-width Add to library / Cancel actions; it no longer interrupts them.
- **Tag group and color controls:** Create new tag now leads its captions. Edit tag Health at **1280/320, both schemes** has a selected Health preset, common fields and deliberate Save/Cancel/quiet Delete hierarchy. At **320**, Name and the well start at **x21**, Name and hex end at **x299**, well is **44px**, and hex starts at **x73**. Native well border matches the field: **rgb(135,146,162)** light / **rgb(100,112,131)** dark. The seven-pixel jog and pure-gray frame are gone. No preset, color or tag was changed.
- **Filters:** **390 both and 320 light**, opened and dismissed without choosing a filter. Header separator, scroll body, rows and centered Done now share the sheet family's structure and edges.
- **Small finishing:** phone tabs visibly balance to Shared / with me and Shared / by me, including bold active states; Duplicate's helper no longer strands “enabled.” in the desktop or narrow samples. Desktop kebabs now occupy **32×32px** slots and visually align with the first title line/tag/posture group; phone targets remain larger.

## Retained history and Share repairs

**History, live 1280/768/320, both schemes:** two existing versions, no version created or restored. At 1280 both date cells start **x715.90625**, and action cells **x859.6484375**. At 768 they start **x393.90625 / x537.6484375** respectively. View/Restore remain side by side. At 320 the number/title/date/badge-or-actions regroup intentionally, above centered full-width Return to Library and quiet Delete. The old 108.45px date drift and action wrap do not recur.

**Share, actual wheel scrolling at all four widths/both schemes:**

| Viewport | Body client / scroll height | Actual scrollTop after wheel | Footer top |
| --- | --- | ---: | ---: |
| 1280×800 | **612 / 905px** | **292.5px** | **706px** |
| 768×900 | **712 / 905px** | **192.5px** | **806px** |
| 390×844 | **681 / 1061px** | **380px** | **763px** |
| 320×740 | **577 / 1124px** | **547px** | **659px** |

Each row agrees across light/dark. These current heights differ from historical measurements because the helper/checkbox markup changed and the 768 viewport here is 900px tall. Complete People/grant controls, both full saved-data choices, the public-link rule and switch disclosure are reachable. Radios remain with their cards and clip beneath the opaque footer; footer hit probes return the footer, not an offscreen radio. Document horizontal overflow is zero in these checks.

At **320 both**, expanded What happens when I switch? and wheel-scrolled to its end: scrollTop **871px**; the final public-link paragraph ends **y639**, above footer **y659**. Done stays available. Expanded Embed tile additionally at **320 light**: URL/code and Copy pair remain contained, with readable supporting explanation. No copy, replacement, revoke, checkbox/radio or data-mode operation was invoked.

## Other fresh live coverage

| Surface | Actual fresh coverage |
| --- | --- |
| Library/cards | 1280/768 both; 390 dark; 320 both. Restrained boundaries/default tiles, reserved desktop title areas and aligned metadata. Phone scroll and fixed tabs remain coherent. |
| Shared spaces | Real rail clicks through incoming/outgoing at 768 both; both destinations at 320 both. Bold active labels fit, counts remain separate, 44px desktop rows stay stable, owner chip/inset is clear. The generous 240px rail still leaves comfortable two-column cards. |
| Search | Blank, `packing`, `not-in-this-library` at 320 both; `run` at 768 both. Integrated clear, balanced empty-state copy and discernible incoming owner attribution. Narrow document overflow is zero. |
| Menus/Duplicate | Run Log menu at 390 light; Duplicate at 1280/390/320 both, cancelled unchanged. Clear option-title/helper hierarchy, matching checkbox scale, trailing desktop actions and intentional phone primary/Cancel stack. |
| Add | All three methods at 1280/320 both; full Paste phone footer after scrolling. Persistent selection, upright prompts, common captions, complete empty Paste prompt and clear URL snapshot explanation. No agent started or URL fetched. |
| Settings | 1280/320 both, top through Storage, Sign out and the disabled Delete account explanation. Consistent value weights, aligned desktop facts, stacked phone facts and restrained destructive emphasis. |
| Editor | Tags/Health dialog/history as above; populated source; expanded Network & permissions at 320 dark, without changing controls; existing Stored state tree sampled at 320 dark; Stored assets at 1280/320 both, reported above. |
| Viewer | Toolbar and settled receipts at 1280/390/320 both; More at 1280 light and 390 both. Readable app surfaces and icon/label rhythm over the user-authored light body. |

## Native source and retained paint

- Entered a temporary mixed HTML/CSS/JavaScript **Paste** specimen. It rendered differentiated syntax in both schemes. Populated Find `visitor`, Replace `guest`, invoked Next but never replacement, and inspected **1280/390/320 both**. Current match visibly leads the quiet other matches; fields fill to Close; Previous is complete; options wrap as whole units (2+1 at 320). Controls are flat/readable, **36px desktop / 44px phone**; phone source viewport remains **240px**. Query and replacement survive real theme changes. Initial role locators failed to populate inputs; those empty frames were discarded as populated-search proof, and CSS locators were used with actual value checks before the successful inspection.
- Go to line at **320 dark in Paste** has complete aligned label/field/Go/Close. Navigated to CSS `2:8` and opened a real completion list with Control+Space: centered readable rows, clear selection, popup **x50.56–312**, retaining the 8px right inset. No completion accepted. The draft was subsequently cleared: hidden `#body` length **0**, placeholder restored.
- Existing Run Log source: actual **partial active-row selection `eta `**, at **1280 both**, visibly leads the next-row echo. Native focus is `.cm-content`; real selection fill is **rgb(219,231,251)** light / **rgb(39,54,74)** dark, active-row alpha **.25**, echo accent alpha **.08**. This is a valid partial selection, not a claimed whole-word double-click or exhaustive painted-contrast matrix.
- Existing-source Go navigation to **27:8**, then Control+Space completion, inspected **1280/320 both**: centered labels, app surfaces and clear selected tint. At 320 the popup is **x50.5546875–312**, **242px** high. Populated existing-source Find/Replace `meta` / `link` at **320 both** has the same coherent treatment. No replacement/completion accepted.
- Run Log remained **28,568 characters / All changes saved** at the final source check. It was only selected, searched and navigated.

## Independence, evidence and limits

Used only my own tab **tab_1c_0d23e38c-d413-4e76-b0eb-59687a206343**, opened after preview_status with `reuseExistingTab:false, open:false`. Its exact ID was supplied on every later browser call. Explicit preview_set_appearance plus live `matchMedia('(prefers-color-scheme: dark)').matches` established **false/true at 1280, 768, 390 and 320**. No root-class simulation or other tab use. The tab was closed after review.

Read visual-brief.md, nine-brief.md, both completed model reports for visual rounds **5–20**, the round-10/18 Opus no-score quota records and round-19 Opus deferral record, all requested change notes **6/7/8/10/11/12/13/15/18/19/20/21**, and nine-validation.md. Did not read round-21 Opus output, nine scores/summary/resume, ticket contents or other parent aggregate/checkpoint files. At the validation-context read, current full Go and 188-state capture refresh were pending; I did not run or verify them. Tests did not determine scores.

Primary evidence is fresh browser images, actual native-panel interaction, computed styles/geometry and real Share wheel movement. **No supplementary saved screenshots were used, and no screenshot/capture files were written.** No runtime production-source inspection was used to declare an unseen presentation fixed. Git checks establish application identity only.

Coverage is broad but sampled, not every state/theme/width permutation. Arbitrary long data/custom colors/many grants, enlarged text, invalid/dirty/post-save states, full stored-data editing, populated stored assets, hosted identity, completion detail panes, special characters and arbitrary bracket/fold variants remain unverified. The visible bracket outlines in the Paste draft are incidental, not an exhaustive bracket audit. No physical phones, software keyboards, alternate engine, screen reader or accessibility certification; freeform resizing retains desktop Chromium/fine pointer. User-authored tool/widget designs and credential-dependent execution are excluded, including the light Run Log body under dark preference.

No builds, suites, commits, app edits, persisted fixture changes or subdelegation. No valid saves, creates, deletes, grants, link replacements, import/export actions or data-mode changes. Temporary Paste cleared, source unchanged, menus/dialogs dismissed. **Only this assigned report was written.**
