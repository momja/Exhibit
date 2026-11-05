# Round 22 — independent Sol visual review, target 9

Reviewer: **gpt-6.1-sol**, 2026-11-04, live review approximately **20:46–20:56 UTC**.

Frozen application **17baf9306f455dbca4882a468e8ba2afe088436f**, served assets **f1b6e9fd582c**, base **44def51afa43019771fd258bc61b28e3ebf4613b**. Initial HEAD was the frozen application; final evidence HEAD was **e0e79cb8997abda47b80aebc8bc75c24b889bf25**. Final read-only application diffs under `internal/` and `web/`, including uncommitted changes, were empty. Versioned live asset URLs matched at entry and the final editor check. Preview: http://t3code:18280.

**Verdict: FAIL against the all-twelve-at-least-9 target.** Both round-21 material findings are repaired in the live application: Share opens and reopens at the beginning on short viewports, and expanded editor explanations now have comfortable type and paragraph gaps. The broader interface is excellent on most inspected surfaces. The remaining material exception is the **Stored state action area**: a conspicuous filled Erase action, competing filled Add/Save actions, a separate compact phone action density, and an Export explanation visually attached to Erase. Composition, Contrast/emphasis and Component visual consistency score **8.5 in each scheme**.

This is an independent single-model result. It establishes neither a joint pass nor council completion. Incoming joint target-9 streak is zero; previous all-8 passes and prior Sol all-9 scores do not carry forward.

## Twelve fresh scores

Scores judge the weakest material surface, without averaging the expanded editor away. Nine means excellent and carefully finished; the remaining issue is localized finishing work, not a broken interface or a request for ornament.

| Fixed category | Light | Dark | Rendered basis / treatment needed for 9 |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Page, section, field and helper roles are clear. Stored assets/state explanations now have comfortable line spacing and separated paragraphs; receipts and native source panels remain readable. |
| Composition and spacing | **8.5** | **8.5** | History columns, sheets, cards and forms are orderly. In the narrow state action stack, Export help begins exactly at Erase's bottom edge, after Import and Erase have intervened since Export. Finish the action/helper grouping and give the note a deliberate gap or place it with its own action. |
| Color harmony | **9** | **9** | Slate neutrals, blue emphasis, restrained default tiles, muted presets and integrated source surfaces are harmonious. The Erase issue concerns the weight and role of an existing semantic color, not an incompatible palette. |
| Contrast and emphasis | **8.5** | **8.5** | Selection, current Find result, selected methods/presets and dialog primaries lead correctly. Stored state still has filled Add key and Save state followed by a prominent filled red/coral Erase. Use a quieter resting destructive entry and subordinate the local Add action so Save owns the editing task. |
| Component visual consistency | **8.5** | **8.5** | Tag, Duplicate, Share, Filters, receipts and source chrome form a coherent family. State Save/Cancel/Export/Import/Erase remain **36px** at both phone widths while neighboring fields/Add key and comparable phone actions are **44px**. Assets export/import retain the same compact exception. Apply the established phone action density to these editor sections. |
| Responsive visual polish | **9** | **9** | Short Share opening/reopening, actual scrolling, fixed sheet chrome, balanced tabs, narrow history, Find whole-unit wrapping and completion inset hold. State actions stack intentionally without overflow; their hierarchy, helper association and density are scored in the categories above rather than deducted again here. |

Matching scores follow actual light/dark inspection, not an assumption that the schemes render identically.

## Remaining material finding: Stored state actions still bypass the finished action grammar

**Observed live:** Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → Stored state → scroll below its existing object/list form. Inspected the complete action area at **1280×800, 320×740 and 390×844, both actual schemes**. The controls and supporting copy are app-owned UI; this finding does not judge user-defined keys, values or the Run Log tool design.

### Hierarchy

The local **Add key** and task-level **Save state** both use filled primary blue. **Erase all data** uses a solid danger fill in the same action area: light **rgb(180,35,24)** with white ink; dark **rgb(242,114,106)** with dark ink. On phones Erase spans the full field width. Its saturated solid surface competes with Save and is particularly conspicuous in dark. On desktop it is a strong red endpoint beside the neutral Export/Import pair.

This differs visibly from the finished Health tag sheet and the artifact footer, whose destructive entry actions use quiet outlines while Save leads. It is the same kind of role/hierarchy drift previously corrected in Tags, now exposed in another material editor section. I did not invoke Erase or inspect its confirmation behavior, and I make no functional-safety claim about it.

### Phone density and helper association

Measured in both schemes, with this browser's **fine pointer** (`matchMedia('(pointer:coarse)').matches === false`):

| Rendered control | 320px width | 390px width |
| --- | --- | --- |
| Add key | 100.06 × **44px** | 100.06 × **44px** |
| Save state, Cancel, Export JSON, Import JSON, Erase all data | Each 268 × **36px** | Each 330 × **36px** |
| Nearby ordinary fields / finished phone sheet and history actions | **44px** | **44px** in the sampled comparison surfaces |

The five-button stack is visibly flatter than the controls immediately above it and the other phone action stacks. This is not one compact icon in a dense row: it includes the full-width task primary and secondary actions. Stored assets' Export assets ZIP / Import assets ZIP also measure **268 × 36px at 320**, so the inconsistency spans the data-management sections.

The newly readable Export paragraph is then placed directly beneath **Erase**, rather than next to Export. In the same controlled scroll position at both phone widths:

- Save begins at y140; Cancel y184; Export y236; Import y280; Erase y324.
- Erase ends **y360**.
- The Export explanation starts **y360**, a **0px box gap**. It wraps to six 21px lines at 320 and five at 390.
- The separate Import explanation follows with the repaired paragraph gap, so the paragraph-to-paragraph repair holds. The remaining problem is the action-to-helper relationship.

The first note visually runs into the bottom of the destructive control even though its text explains Export. At desktop the same note follows the whole horizontal action bar with no top gap. This is a grouping issue supported by live screenshots and geometry, not a speculative source reading.

**Coherent treatment for 9:** finish this existing action area using the established system: Save as the editing primary, secondary local Add and transfer actions, quiet outlined Erase at rest, and 44px phone action density. Give Export/Import help an explicit separation from the action bar or group each note with its corresponding action, preserving their essential warning text and current behavior. Carry the phone density convention into Stored assets too. No palette replacement, decorative panel, new workflow or user-data redesign is necessary. Compact desktop utility actions may remain compact.

## Round-22 repairs independently verified

### Share opening and reopening

Used the real viewer Share control, not `showModal()` directly. At **320×640, 390×640 and 640×360 in light and dark**, inspected the first opening, wheel-scrolled to the bottom, dismissed with Escape, and reopened. Every first/reopen read showed **scrollTop 0** and focus on header **Close**, not People. The notice is the first body content; the old half Replace button under the header does not recur.

| Short viewport, both schemes | Body client / scroll height | Actual scrollTop after wheel | Reopen scrollTop |
| --- | --- | ---: | ---: |
| 320×640 | **477 / 1124px** | **647px** | **0** |
| 390×640 | **477 / 1061px** | **584px** | **0** |
| 640×360 | **172 / 947px** | **774.5px** | **0** |

Landscape necessarily exposes less body at once, but the notice starts correctly, body scrolling works, and Close/Done remain available. Native controls clip with their body; footer hit probes return the footer region. This live session retains a desktop pointer and does **not** independently reproduce the parent's separate coarse-pointer contexts or a software keyboard.

### Expanded editor help

**Stored assets, 1280 and 320, both schemes:** all five paragraphs now compute to **13px / 19.5px desktop**, **14px / 21px phone**, each with **12px bottom margin**. At desktop, the three-line introduction is 58.5px high; its next paragraph begins 12px later. The warning, empty message and separate transfer notes no longer merge into the old 12px/normal block.

**Stored state, 320 both:** the introduction and both transfer notes compute to **14px / 21px**, with **12px paragraph margins**. Inspected both the introductory help and the lower notes, without editing data. The action-area finding above does not negate this typography repair.

**Gallery widget app help, 1280/320 both:** the explanation is readable and separated from the source below. At 320 it computes to **14px / 20.3px**, 12px bottom margin; that small line-height difference from 21px is trivial. The widget's own design was excluded.

**Small refinements:** desktop Tag Add and Create and add tag both measure **44px**. The card posture trigger now occupies **32×32px**, matching the desktop menu slot. The expanded Embed tile helper visibly uses the comfortable helper treatment in both schemes at 320.

## Earlier repairs retained on fresh evidence

- **History:** two existing versions at **1280/768/320, both schemes**. At 1280, both dates start **x715.90625**, both action cells **x859.6484375**. At 768 they start **x393.90625 / x537.6484375**. View/Restore remain together. Phone facts regroup clearly above centered Return to Library and quiet Delete. No View/Restore action was invoked.
- **Share scrolling and paint:** live top/bottom checks at **1280×800, 768×900, 390×844 and 320×740, both schemes**. Actual wheel scrollTop was respectively **292.5 / 192.5 / 380 / 547px**; body client/scroll heights **612/905, 712/905, 681/1061, 577/1124px**. Full People/grant controls, both choices and the public-link rule are reachable. Radios stay with cards and do not paint over the fixed footer. Expanded the switch explanation and reached its final paragraph at **320 both**. Expanded Embed at **320 both**. No sharing value changed.
- **Receipts:** actual Run Log card trigger at **1280 both** opens a **320×323px** panel at **x264/y191.09**, ending y514.09 without extending the 800px document. Readable headers/body and clear section separation. Viewer About/posture and settled More were inspected at **320 both**: opaque, readable and contained above the toolbar. The excluded user tool body remains light under dark preference.
- **Tag sheet:** Health at **320 both** has marked current preset, readable Name/Color captions, aligned color controls and deliberate Save/Cancel/outlined Delete stack. Field and well start x21, field and hex end x299; hex starts x73. Dark field frame is **rgb(100,112,131)**. Dismissed unchanged.
- **Forms/sheets:** Duplicate at **1280/320 both** has comfortable captions/choice descriptions, shared checkbox scale, trailing desktop actions and primary/Cancel phone stack. Filters at **390 both** has the shared header separator, aligned list and centered Done. No changes submitted.
- **Navigation/cards/Search:** active incoming/outgoing desktop spaces at **768 both** have stable 44px rows, complete bold labels and separate counts. The 240px rail still leaves comfortable two-column cards. Library at **1280/768/390 both**, plus **320 dark**, retains reserved title areas and restrained boundaries/default tiles. Incoming/outgoing at **390 both** show coherent attribution and balanced active phone labels. Search `run` at **768 both**, blank/`packing`/`not-in-this-library` at **320 both** retains integrated clear, discernible owner chip and balanced empty states. Narrow measured document overflow is zero.
- **Settings/Add:** Settings at **1280 both** through Sign out and **320 both** through the full disabled-delete explanation has consistent values, fact alignment/reflow and restrained danger treatment. All three Add methods at **1280/320 both** retain persistent selected tint, common captions, upright prompts and complete Paste prompt. The phone Paste note precedes consecutive full-width primary/Cancel actions. No agent execution, URL entry/fetch or creation submission.

## Native source coverage

- **Existing source, 1280 both:** double-clicked the actual rendered `meta` word on its active row. Native selection returned **meta**, with `.cm-content` focused. Real selection fills are **rgb(219,231,251)** / **rgb(39,54,74)**; active-line paint retains **.25 alpha**, echo paint **.08 accent alpha**. The selected word visibly leads its next-row echo. Both syntax palettes remain differentiated. This was not an exhaustive token-contrast or painted-layer calculation.
- **Existing Find/Replace at 1280/390/320 both:** entered `meta` / `link`, invoked Next without replacement. Readable flat controls, fixed Close, complete Previous, whole-unit options (2+1 at 320), and clear current-result emphasis remain. Phone source viewport is **240px**. Values survived scheme changes.
- **Existing Go at 320 both**, navigation to `27:8`, then **Control+Space completion at 320 both**: matching panel controls, centered readable list and clear selected tint. Popup **x50.5546875–312**, height **242px**, preserving the 8px right inset. No completion accepted.
- **Paste at 320 both:** temporary six-line HTML/CSS/JavaScript specimen showed differentiated syntax and visible Select All through the nonempty closing script row. Populated Find/Replace `visitor` / `guest` and inspected the responsive controls, without replacement. Cleared afterward: hidden `#body` length **0**, placeholder restored. Existing Run Log source remained **28,568 characters / All changes saved** at the final check.

## Minor opportunities and limits

The repeated public-link sentence inside the expanded switch explanation remains harmless redundancy. Native dark unselected radios have a gray disc; the selected radio and full selected card clearly distinguish the state. Uppercase facts/navigation eyebrows versus sentence-case editable captions are a consistent role distinction. The generous rail, faint source orientation/echo hints and utilitarian completion marker do not require redesign for 9.

Used only my own T3 tab **tab_1d_0d23e38c-d413-4e76-b0eb-59687a206343**, opened after `preview_status` with `reuseExistingTab:false, open:false`; its exact ID was supplied on every later browser call. Explicit `preview_set_appearance` plus live `matchMedia('(prefers-color-scheme: dark)').matches` confirmed **false/true at 1280, 768, 390 and 320**, as well as the short/landscape Share checks. No root-class simulation. The tab is closed.

Read the required briefs, both completed models' visual reports for rounds **5–21**, the round-10/18 Opus no-score quota records and round-19 deferral record, all specified change notes through **22**, and `nine-validation.md`. Large historical reads were followed by smaller reads where output was truncated. Did not read current Opus output, parent scores/summary/resume, ticket or aggregate/checkpoint contents. When validation context was read, current full Go/capture refreshes were pending; prior passes were historical. No test decided a score.

Primary evidence is fresh live browser images, DOM/computed geometry and normal view interactions. **No supplementary saved screenshots were used, and no capture files were written.** No runtime application source was used to declare a visual fix. Git checks establish application identity. Some initial role locators were wrong (unsupported exact attribute or Filters rather than Filter); these were corrected using live DOM/locators. Unchanged/closed frames from failed probes were not counted as successful destination/overlay evidence.

Coverage is broad but sampled, not every route/theme/size/state permutation. Unverified: arbitrary long values/custom colors/many grants, enlarged text, invalid/dirty/post-save states, populated asset rows, full state editing, native completion detail panes, special characters and arbitrary bracket/fold variants, hosted identity and admin account management. Short Share opening/reopening was independently exercised in the fine-pointer session; the parent's coarse-pointer validation is supplied context only. No physical phone, software keyboard, alternate engine, screen reader or accessibility certification. User-authored tool/widget designs and credential-dependent execution are excluded.

No builds, suites, commits, application edits, capture writes, persisted fixture changes or subdelegation. No valid saves, creates, deletes, grants, link replacements, transfer/erase operations or data-mode changes. Temporary Paste cleared; existing source/data unchanged; dialogs dismissed. **Only this assigned report was written.**
