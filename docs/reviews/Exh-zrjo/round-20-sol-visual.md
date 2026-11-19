# Round 20 — independent Sol visual review

Reviewer: **gpt-6.1-sol**, 2026-11-04, approximately 16:58–17:08 UTC. Frozen application **8504bc516e4be2ed7fd8369b7f3823632cc6272f**, served assets **33556024040a**, base **44def51afa43019771fd258bc61b28e3ebf4613b**. Read-only Git inspection found evidence HEAD **33c117d3e929e71cef995f7ef9e8521ccfec250b**, an empty application diff under `internal/` and `web/` against the frozen candidate, and the expected merge-base. The only untracked material at that check was the parent's round-20 capture directory. Live asset URLs were reconfirmed at exit. Preview: http://t3code:18280.

**Verdict: PASS for Sol at the target-9 bar. All twelve independent scores are 9.** No material visual weakness remains in the freshly inspected surfaces. This is an excellent, restrained utility interface: the forms, sheets and native source controls now show the same care as the Library. The history fix was verified rather than assumed to close the review. This report does **not** establish a joint passing round, a consecutive joint streak, or council completion. Incoming joint target-9 streak is zero, and an absent peer result is not approval.

## Twelve scores

Each score judges the weakest materially inspected surface, without averaging a weak sheet or editor into stronger pages. Nine permits rare trivial refinements; it does not require decoration or a new feature.

| Fixed category | Light | Dark | Fresh basis |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Page/section/field hierarchy is clear and consistent. Upright prompts, common captions, readable helpers and differentiated source make even the dense sheets comfortable to scan. |
| Composition and spacing | **9** | **9** | History facts now share columns. Sheets have deliberate header/body/footer structure, comfortable choice measures and intentional phone action stacks. Cards, Settings facts and creation forms retain coherent gutters and rhythm. |
| Color harmony | **9** | **9** | Slate neutrals, restrained blue emphasis, muted presets/default tiles and integrated source surfaces agree. Dark app surfaces do not inherit a foreign editor/control palette. |
| Contrast and emphasis | **9** | **9** | Primary actions and selected states lead; destructive entry actions stay quiet. Duplicate option names lead helpers. Actual source selection is visible on the active row and stronger than the echo. |
| Component visual consistency | **9** | **9** | Tag, Duplicate and Share form a consistent modal family. Fields, native choices, menus, Find/Replace, Go and completion use compatible type, surfaces, boundaries and action density. |
| Responsive visual polish | **9** | **9** | Phone sheets and history regroup deliberately, complete Find labels wrap as units, and completion retains a real viewport inset. Desktop navigation fits at 768, with comfortable two-column cards. |

## Current history repair — independently verified

Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → Versions, `.version-row`, `.version-when`, `.version-actions`; live **1280×800 and 768×800, both actual schemes**.

| Width | Current date x | Older date x | Current action-cell x | Older action-cell x |
| --- | ---: | ---: | ---: | ---: |
| 1280 | 715.90625 | 715.90625 | 859.6484375 | 859.6484375 |
| 768 | 393.90625 | 393.90625 | 537.6484375 | 537.6484375 |

Both themes returned these values. The older View/Restore buttons also have the same y (603px in the final scrolled measurement). The former 108.45px metadata stagger is absent: this reads as a shared history list again, while the actions retain their natural width. Current is a quiet accent badge within the common trailing region. The live fixture contains **two versions**; I did not create a third or claim a three-row live check.

At **320/both**, I scrolled fully to the bottom and inspected the retained phone grouping: version number, title, date, Current or side-by-side View/Restore, then centered full-width Return to Library and outlined Delete. No history action was invoked. Initial snapshots after resizing showed only part of history; those were not accepted as proof of the lower layout, and the bottom was subsequently inspected directly.

## Share containment and composition — retained repair holds

Run Log viewer → Share, `#share-panel .modal-body`, live **1280×800, 768×800, 390×844 and 320×740, both actual schemes**. At every combination I inspected the top, issued a real downward container wheel gesture, then inspected the lower choices and footer.

| Width | Body clientHeight / scrollHeight | Actual scrollTop after wheel | Footer top |
| --- | --- | ---: | ---: |
| 1280 | 612 / 919 | 306.5 | 706 |
| 768 | 612 / 919 | 306.5 | 706 |
| 390 | 681 / 1070 | 389 | 763 |
| 320 | 577 / 1133 | 556 | 659 |

Measurements agree in light/dark. Both complete data choices, People/grant row, public-link rule and disclosure are reachable. Radios remain attached to the cards and clip with the scrolling body. Footer hit probes return Done or the footer region, not an offscreen radio. There is no detached choice indicator over Done. Expanded “What happens when I switch?” at **1280/both**, then wheel-scrolled to the final public-link paragraph; the footer remains available.

The warning is concise and subordinate, headings introduce their content clearly, URL/Copy pair coherently, Replace has its own helper line on phones, and Revoke aligns with its section. This is now an intentional long settings sheet. I found no need to remove essential distinctions or add ornament for a 9. No checkbox/radio/value/access action was changed.

## Other fresh rendered evidence

- **Library/spaces/Search:** Library at 1280 light, 768/both and 320/both; incoming/outgoing at 768/both; incoming at 390/both. Active bold rail labels fit and counts remain separate; the two 232px columns at 768 are comfortable. Reserved card title areas, incoming owner inset, tag dots and posture/menu affordances are coherent. Phone tabs have aligned icons and complete wrapping labels. Search `run` at 768/both, `packing` at 390/both, blank and `not-in-this-library` at 320/both have integrated clear, discernible owner attribution and well-measured empty-state typography. Search is visibly active in the rail.
- **Menus/Duplicate:** Mortgage Calculator card menu at 320 dark has even icon/text rhythm and a settled opaque surface. Duplicate at 320/both has common Title treatment, app-colored input selection, readable option titles/helpers, quiet unchecked controls and an intentional primary/Cancel stack. Cancelled unchanged.
- **Tags:** expanded Tags and Health Edit tag at 320/both. Short tag prompt fits; Add is secondary; creation fields use the available width. Muted preset circles and selected check are clear. Health's actual color is selected in the dialog. Name/native-color/hex controls and full-width Save/Cancel/outlined Delete have consistent rhythm and emphasis. Dismissed unchanged.
- **Add:** all three methods at 1280/both and 320/both. One page heading, Library return, persistent selected-method tint, common captions/upright prompts, readable URL snapshot explanation and intentional phone action hierarchy. The full empty Paste prompt fits. Initial text-based method locators did not switch modes; those repeated Agent frames were discarded, and actual `data-mode` controls were used to inspect URL and Paste successfully. No agent started, URL fetched, or form submitted.
- **Settings:** 320/both from top through Storage, then scrolled Sign out/disabled Delete account explanation; 768/both top through Sign out. Facts align at desktop and stack on phone, comparable values share weight, headings/icons are consistent, and the destructive region has appropriate restrained emphasis.
- **Viewer:** 390/both settled More menu and About/posture popover. Opaque surfaces, readable labels and consistent icon rhythm remain clear above the user-authored light tool body. The light tool itself is excluded. Also inspected the Filter sheet at 390/both, with clear collection/tag groups and centered full-width Done; dismissed without selecting filters.

## Source chrome and retained paint

Existing Run Log source was inspected without editing it. **Populated Find/Replace at all four widths/both schemes**, query `meta`, replacement `link`: readable flat controls, field filling to fixed Close, complete Previous, whole option units (2+1 at 320), and coherent replacement actions. The source remains visible above the panel; no replacement invoked. Closing Find and opening **Go to line at 1280/both** shows matching field/Go/Close treatment.

Navigated to existing CSS `27:8` and opened **Control+Space completion at 1280/390/320, both schemes**. Labels are centered/readable, selection is clear and the surface fits the app. At 320 the actual tooltip is **x50.5546875–312**, preserving the **8px right inset**, with height242px. No completion accepted. The partially visible final row signals the scrollable list; it does not clip a fixed action.

At **320/both**, navigated back to the opening meta row and selected four characters with Shift+Right. The actual native selection was **`eta `**, rather than the full word `meta`; this is a valid partial active-row selection, not a claimed whole-word probe. It visibly leads the next-row echo. Actual selection fills are **rgb(219,231,251)** / **rgb(39,54,74)**, active-line alpha **.25**, echo accent alpha **.08**. Syntax remains differentiated and the source/gutter readable. I did not repeat the exhaustive grammar/contrast matrix or calculate every composited token ratio this round. Existing source remained **28,568 characters / All changes saved** after navigation and inspection.

## Minor opportunities and limits

The generous240px rail at768 is optional density work; it does not harm these readable cards or active labels. Quiet active-band/echo tints are appropriate secondary cues. Completion's utilitarian property square marker and small card-kebab optical differences could be refined, but neither obscures hierarchy or disrupts composition. These are trivial opportunities, not a request for arbitrary novelty or a reason to withhold9.

Used only my own T3 tab **tab_18_0d23e38c-d413-4e76-b0eb-59687a206343**, opened after `preview_status` with `reuseExistingTab:false, open:false`. Exact tab ID was supplied on every later call. Explicit `preview_set_appearance` plus live `matchMedia('(prefers-color-scheme:dark)').matches` established **false/true at1280/768/390/320**. No root-class simulation or other tab use.

Consulted the required briefs, historical reports through19 (including no-score quota/deferral records), specified change notes through20 and nine-validation.md. Large historical reads were bounded, with smaller follow-up reads. Did not read round20 peer output, current parent scores/summary/resume, ticket or aggregate checkpoints. Supplied tests and previous ratings do not determine these scores.

Primary proof is fresh live images and measured geometry/interaction. No supplementary saved capture was substituted for live inspection, and no capture file was written. Coverage is broad but sampled, **not every route/theme/width/state permutation**. In particular: arbitrary long values/custom colors/many grants, enlarged text, invalid/dirty/post-save states, completion detail panes, special-character and arbitrary bracket/fold variants remain unverified. Paste was inspected empty; no new mixed-language Paste specimen was entered in this round. Expanded Share disclosure was inspected desktop, not every phone permutation. No physical phone, software keyboard, alternate engine, screen-reader or hosted-identity certification. Resizing retains desktop Chromium/fine pointer. User-authored widget/tool designs and credential-dependent execution are excluded.

No builds, suites, commits, app edits, capture writes, persisted fixture changes or subdelegation. No valid saves, creates, deletes, grants, link replacements or data-mode changes. No source draft was entered; search/replacement text was only local native-panel state and dismissed. Dialogs closed unchanged. **Only this assigned report was written.**
