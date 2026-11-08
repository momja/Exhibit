# Round 19 — independent Sol visual review

Reviewer: **gpt-6.1-sol**, 2026-11-04, approximately 16:42–16:52 UTC. Frozen application **a65d7c6bb4cb1277c5c44c51de4db6293ada2b0c**, served assets **b1ef638450cc**, base **44def51afa43019771fd258bc61b28e3ebf4613b**. HEAD during review was **8e11abd97c4853368dbf22b9b01bd1ae92f81102**; read-only diffs of `internal/` and `web/` against the frozen application were empty. Asset version was reconfirmed at exit. Preview: http://t3code:18280.

**Verdict: FAIL against the all-twelve-at-least-9 target.** Share's scrolling/paint failure is genuinely repaired. Most inspected surfaces now earn excellent ratings, but desktop Versions has lost its aligned timestamp column. Composition is **8.5 in each scheme**. This is a localized finish issue, substantially smaller than the round-18 broken sheet, but it falls short of carefully finished tabular composition. This report establishes neither a joint passing round nor council completion. Incoming joint target-9 streak is zero; no missing peer result is approval.

## Twelve independent scores

The weakest material inspected surface governs each category. Matching theme ratings follow live inspection in both actual schemes.

| Fixed category | Light | Dark | Fresh basis |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Shared field captions, upright prompts, comfortable helpers and page/section hierarchy now agree. Share's essential warning and short rules are scannable; native editor labels are readable. |
| Composition and spacing | **8.5** | **8.5** | Sheets now contain their content correctly and narrow footers are deliberate. However, the two desktop Versions timestamps start 108.45px apart because each row sizes its action column independently. For 9, share an intrinsic action-column width across the list, preserving the single-row actions and phone layout. |
| Color harmony | **9** | **9** | Slate neutrals, restrained blue emphasis, muted presets/default tiles and integrated source surfaces are excellent together. No palette replacement or ornament is needed. |
| Contrast and emphasis | **9** | **9** | Save leads quiet Delete; selected methods and sharing cards are unmistakable; Duplicate option titles lead helpers. Actual source selection remains visible over the active row and stronger than its echo. Share's radios now stay with their complete choices. |
| Component visual consistency | **9** | **9** | Tag, Duplicate and Share use a coherent modal family; native Find/Go/ completion now share the app's readable flat controls and surfaces. Full-width action labels are centered. The timestamp issue is counted under composition. |
| Responsive visual polish | **9** | **9** | Actual 320/390 sheets have intentional full-width action stacks, readable choice measures and fixed headers/footers with reachable lower content. The 768 rail/cards remain comfortable; completion has a real 8px edge inset at 320. Desktop column alignment is scored under composition rather than deducted again here. |

## Remaining treatment necessary for 9: align desktop history timestamps

**Observed live:** Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → Versions, **1280×800, light and dark**, `.version-row`, `.version-when`, `.version-actions`.

The previous View/Restore wrap is fixed: the buttons sit side by side in a **186.35px × 36px** group. But Current occupies only **77.91px**, and each row now allocates its columns independently. The date consequently moves by more than a full button width between adjacent rows, despite identical date strings. The screenshot reads as staggered metadata rather than a shared history table.

| Live desktop geometry, same in both schemes | Current row | Older row |
| --- | ---: | ---: |
| Row left/right | 234 / 1046 | 234 / 1046 |
| Version label starts | 282 | 282 |
| Timestamp starts | **824.35** | **715.91** |
| Timestamp width | 135.74 | 135.74 |
| Action track starts | 968.09 | 859.65 |
| Action track width | **77.91** | **186.35** |
| Computed grid tracks | 40 / 534.35 / 135.74 / 77.91 | 40 / 425.91 / 135.74 / 186.35 |

This is rendered geometry supporting a visible symptom, not a speculative source defect. I did not alter the DOM/CSS to demonstrate a fix. Earlier claims of retained aligned dates are not fresh proof for this candidate.

**Coherent treatment:** let all desktop version rows share a trailing track sized to the widest intrinsic action group, and a common date track. A parent grid/subgrid or equivalent shared intrinsic sizing can retain natural label sizing without returning to the inadequate hard-coded 176px track. Place Current consistently within the shared trailing cell. Keep View/Restore on one line when room permits; preserve deliberate phone reflow and readable enlarged labels. Verify the rendered date x positions across both rows, in addition to checking that the buttons fit. No new feature, decorative styling, smaller font or tighter copy is required.

## Round-18 Share blocker — genuinely resolved

**Observed live:** same Run Log viewer → `#share-open` → `#share-panel`, at **1280×800, 768×800, 390×844 and 320×740, both actual schemes**. Opened unchanged, inspected top, issued real container wheel scrolls, inspected lower settings and measured the body/footer.

| Width | Body clientHeight / scrollHeight | Actual scrollTop after downward wheel | Footer top / bottom |
| --- | --- | ---: | --- |
| 1280 | 612 / 919 | **306.5** | 706 / 783 |
| 768 | 612 / 919 | **306.5** | 706 / 783 |
| 390 | 681 / 1070 | **389** | 763 / 844 |
| 320 | 577 / 1133 | **556** | 659 / 740 |

Both schemes returned the same layout measurements. The body is now a regular block. People, Revoke, both complete saved-data choices, the public-link rule and the switch disclosure are reachable. The native radios remain inside their cards and disappear with them beneath the fixed body boundary; no detached circles paint over Done. Footer hit probes returned the footer region, not an offscreen radio. Header and footer remain visually separate and stationary across these scrolls.

Expanded “What happens when I switch?” at **768 dark and 390 both**. Scrolling reaches the final public-link paragraph with Done still available. This expanded coverage is sampled, not a claim that every disclosure permutation was exercised at all widths. No sharing value, checkbox, radio, grant, link or data mode was changed. The longer sheet now feels like an intentional settings sheet rather than clipped content. The four-line warning on phones remains proportionate and necessary; I do not require another copy rewrite.

## Other prior-repair verification

All below are fresh live renders unless a limit is stated.

- **Tag, 320 both:** Health opens as a bottom sheet with complete Name/Color fields, muted preset choices, selected Health color and coherent hex/native-color row. Save, Cancel and outlined Delete form a centered full-width stack. The old accidental Save wrap and dominant coral Delete are gone. Dismissed unchanged. Expanded creation fields have the short “Choose a tag…” prompt and secondary Add beside it; Create and add tag retains task emphasis.
- **Duplicate, 320 both:** menu is opaque with even icon/label rows. The sheet has readable Title, 18px choice controls, darker option titles above quieter helpers, app-family text selection and an intentional primary/Cancel stack. Dismissed without submission.
- **History, 320 both:** version/title/date/current/actions remain clearly grouped, Current has an accent badge, explanatory text is comfortably spaced, and Return to Library/Delete labels are centered full width. No history action invoked. Desktop action wrap is repaired, with the separate date-alignment finding above.
- **Add, 320 both:** Agent, URL and Paste selected treatments persist after focus moves. One visible page title, Library return, common captions and upright prompts agree. Empty Paste prompt fits in full. URL snapshot choice and its explanation are readable; no URL entered or fetched. A short temporary HTML/CSS/JS Paste draft showed distinct light/dark syntax; Find with `visitor`, Replace and whole-unit options/complete Previous are readable in both schemes. The source retains its 240px search-open viewport. No replacement/creation invoked. Draft cleared; hidden `#body` length **0** confirmed before leaving.
- **Native Go/completion:** Go used in Paste at 320 and existing source at 1280; no edits through navigation. Existing CSS completion at `27:8` opened successfully with **Control+Space**, then was inspected at **1280 dark, 390 both and 320 both**. Rows are centered and readable, selected tint is clear and the 320 popup spans **x50.55–312**, giving the promised **8px right inset**. No completion accepted. An earlier literal-space shortcut attempt in Paste produced no popup and was not accepted as completion evidence.
- **Actual source selection, 390 both:** double-clicked existing `meta` on its active row. Native selection returned `meta`; real fills are **rgb(219,231,251)** / **rgb(39,54,74)**; active paint retains **.25 alpha**, echoes **.08 accent alpha**. Screenshots show the selected word leading the next-row echo. Complete syntax remains differentiated. Existing source stayed **28,568 characters / All changes saved**. This is a focused retained-paint check, not a repeat of the exhaustive grammar/paint matrix.
- **Library/spaces/Search:** 1280 light Library; 768 both Library/outgoing/results `run`/incoming results `packing`; 768 dark active incoming space; 390 both incoming and Filters; 320 dark Library/card menu; 320 both blank/miss Search. Active rail labels fit, counts remain separate, phone icons align above full wrapping tab labels, incoming owner chip is readable, and empty-state helper/Clear have deliberate rhythm. Search is visibly active in the rail. Two 232px cards remain comfortable beside the generous 240px rail at 768.
- **Settings:** 320 both top through Storage and scrolled Sign out/disabled Delete explanation; 1280 dark Profile/Administration/Storage/Sign out. Facts have consistent weights and aligned desktop columns; phone grouping, card headings/icons and restrained destructive region are coherent.
- **Viewer, 390 both:** About/posture popover and settled More menu have opaque surfaces, clear icon/text rhythm and readable hierarchy over the user-authored light body. An initial opening-animation frame was discarded; settled More was rechecked in both schemes. The light Run Log body is outside scoring.

## Minor opportunities, evidence and limits

- The 768 rail remains generous; shrinking it is optional. The card columns and active labels are comfortable. Do not sacrifice the repaired label widths to pursue density.
- Source active-band/echo tints remain deliberately faint. Actual selection leads, so greater saturation is optional, not a necessary 9-level change.
- Completion's subdued square property-type marker is utilitarian; a more descriptive marker could refine it, but readable centered labels and selected state already carry the task. I did not diagnose it as a missing font or demand an icon redesign.
- Small optical differences in card kebab/title anchors remain a possible refinement. Unlike the measured history column drift, these do not disrupt the information structure enough to hold another category below 9.

Used only my own tab **tab_17_0d23e38c-d413-4e76-b0eb-59687a206343**, opened after `preview_status` with `reuseExistingTab:false, open:false`. Every later browser call used that ID. Explicit `preview_set_appearance` plus live `matchMedia('(prefers-color-scheme: dark)').matches` established **false/true at 1280, 768, 390 and 320**; no root-class simulation. No other tab was read or operated.

Read the required briefs, both models' prior visual reports through round 18 (rounds 10/18 Opus availability records contain no scores), specified change notes through round 19 and `nine-validation.md` as historical/verification context. Did not read any round-19 peer report, nine scores/summary/resume, ticket or parent aggregate/checkpoint. Prior scores and tests did not determine these ratings. Some large historical reads were bounded and followed by smaller reads; no current capture manifest was treated as proof.

Primary evidence is live images, computed layout and real interaction. No supplementary saved PNGs or capture matrix was inspected, written or overwritten. No runtime source inspection was used to claim an unseen visual fix. Read-only Git established application identity; live DOM established the history-column behavior.

Coverage is broad but sampled: not every route/theme/size combination, desktop form/dialog variant, arbitrary long value, custom color, enlarged text, invalid/dirty/post-save state or applied filter result. Completion detail panes, special characters and arbitrary bracket/fold variants remain unverified; incidental bracket rendering is not exhaustive coverage. No physical phone, software keyboard, alternate engine, hosted identity or screen-reader certification. Viewport resizing retains desktop Chromium/fine pointer. User-authored tools/widgets and credential-dependent execution are excluded.

No builds, test suites, commits, application edits, persisted fixture changes, capture writes or subdelegation. No valid saves, creates, deletes, grants, link replacements or data-mode changes. Temporary Paste content cleared; dialogs/menus dismissed unchanged. **Only this assigned report was written.**
