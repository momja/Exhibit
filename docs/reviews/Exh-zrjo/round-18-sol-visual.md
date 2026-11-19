# Round 18 — independent Sol visual review

Reviewer: **gpt-6.1-sol**, 2026-11-04. Frozen application **234da736cec712dc1d1f50e4f8ae0871e4ed4bff**, base **44def51afa43019771fd258bc61b28e3ebf4613b**. Read-only Git checks returned those exact HEAD/base values and no application difference under `internal/` or `web/`. Live asset version **8309e86eb320**, checked at entry and exit. Preview: http://t3code:18280.

**Verdict: FAIL for the 9 target.** The shared grammar repairs substantially improve the interface, but the new Share modal clips its lower content and its intended body does not scroll in the inspected Chromium session. Saved-data choices disappear below the footer while native radio circles can paint over it. This is a material rendered defect, not a request for decoration or another palette. No qualifying joint round or council completion is established by this report.

## Twelve independent scores

Scores consider the weakest material inspected surface, without averaging a broken modal away. Matching theme scores reflect reproduction in both actual schemes.

| Fixed category | Light | Dark | Evidence and treatment needed for 9 |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Field captions, upright prompts, option titles/helpers and page headings now have a coherent hierarchy. Share's visible warning/rules are readable and separated by meaningful headings. Clipped lower content is scored as layout, rather than a separate type-style defect. |
| Composition and spacing | **7** | **7** | Share's lower body collides with/disappears under its fixed footer; desktop also shows isolated radio circles in the footer. For 9, provide a genuinely scrolling body with all choices, labels and disclosures contained above an opaque, separate footer. Other inspected dialog and history layouts are substantially better. |
| Color harmony | **9** | **9** | Slate neutrals, blue emphasis, quiet dark fields, restrained default tiles and muted tag presets form an excellent coherent system. No replacement palette is needed. |
| Contrast and emphasis | **8** | **8** | Tag Save now clearly leads quiet Delete, selected Add is persistent, and Duplicate options lead their helpers. Share nevertheless leaves selected/unselected radio indicators visually detached from the hidden choices. For 9, restore the whole choice's visible label, description and selected treatment together within the scroll region. Do not strengthen the bare circles. |
| Component visual consistency | **7** | **7** | Tag and Duplicate render as deliberate shared sheets, but Share's fieldset-backed body fails the shared modal contract and exposes native controls across the footer. For 9, make the same header/body/footer containment work with Share's actual DOM, retaining native semantics and pending/dismiss behavior. |
| Responsive visual polish | **7** | **7** | At 320px even the lower People region is cut off; at 390px Saved data is below the body's bottom edge. This is materially worse than an incidental wrap. For 9, scrolling must reveal the entire lower sheet at 320/390 and desktop, with header/footer fixed and no controls bleeding over either. |

This is one dominant defect with consequences for several independently defined categories, not multiple unrelated redesign requests. The repaired Library/forms do not compensate for it.

## Material finding: Share's intended scroll region is not scrolling

**Observed live:** Run Log `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332` → Share (`#share-open` → `#share-panel`), **320×740, 390×844 and 1280×800, light and dark**.

Reproduction on the frozen application:

1. Open Share without changing a setting. Done remains visible at the bottom.
2. Try to scroll `#share-panel .modal-body` downward. I used the T3 container `preview_scroll` command at 320, direct `scrollTop = 9999` with a settling wait, and PageDown from the initially focused People input at 390.
3. The body remains at `scrollTop: 0`. The lower settings are outside the visible body and the dialog clips them. At desktop, two bare radio circles paint in the Done footer with their labels/cards hidden. At 390 a radio circle also appears near the bottom edge beneath the footer.

Measured live geometry (same values in both themes):

| Viewport | Body clientHeight / scrollHeight | Body scrollTop after requested downward scroll | Body bottom | Important clipping |
| --- | --- | ---: | ---: | --- |
| 320×740 | **577 / 1133** | **0** | **659** | Revoke's rectangle is y747.5–791.5, beyond the body and viewport. Done is still visible. |
| 390×844 | **681 / 1070** | **0** | **763** | Saved data heading starts at **y774.5**. Choice cards occupy **y807.5–925** and **y935–1031.5**, beyond the body. Done is y784–828. |
| 1280×800 | **612 / 919** | **0** | **706** | Saved data begins at **y692.5**, cut by the footer. Done is y723–767; unlabelled radio circles show beside it. |

At 320 the actual `preview_scroll` container call also left both body and dialog scroll positions at zero. PageDown at 390 left both at zero. These are browser observations, not acceptance of an `overflow:auto` declaration. No choice, checkbox, grant, copy, revoke or link-replacement action was invoked.

**Source support, not a proven complete root cause:** the live body is `FIELDSET.modal-fields.modal-body`, with computed `display:block; overflow:auto`. The dialog is a flex column with `overflow:hidden`. `web/gallery/components.css` around 513–520 assigns the shared flex/scroll treatment directly to `.modal-body`. Fieldsets have special browser layout behavior; the fieldset implementation is the first place to investigate. I did not alter CSS/DOM to test a repair, so I do not claim a root-cause fix is verified.

**Concrete coherent treatment:** use a regular block as the actual constrained scroll container, with the semantic fieldset inside it, or otherwise correct Share's fieldset layout so the real browser scrolls its contents. Keep the fixed header/footer, existing native semantics and pending state. Ensure native radio paint is clipped to the body with its card. Verify rendered bottom content and actual scroll movement, not just horizontal overflow, the presence of `overflow-y:auto`, or a visible Done button. Reopen the unchanged sheet and inspect top, People, both full saved-data choices, the public-link rule and the switch disclosure in both themes at all three sizes.

## Prior repair verification

These are fresh rendered observations unless explicitly limited:

- **Edit tag, 320/both:** Health opens in a full-width bottom sheet with header Close, clear Name/Color labels, muted presets and an obvious selected Health swatch. Save, Cancel and quiet outlined Delete form an intentional centered full-width stack. The previous accidental Save wrap and loud coral Delete are gone. Dismissed without edits.
- **Duplicate, 320/both:** Title and helper hierarchy, 18px-scale checkboxes, app-family input selection, and full-width primary-then-Cancel footer look finished. The old left-aligned incidental wrap is gone. Dismissed unchanged. Card menu at 320 light has even icon/label rhythm and a settled opaque surface.
- **History, 1280/both:** View and Restore now sit together in one compact row. Timestamps remain aligned; Current has a readable accent badge. At 320 light, history groups version/title/date/status/actions clearly and Return to Library is centered full-width. No Restore or View action was invoked.
- **Tags, 320 dark:** the shorter “Choose a tag…” prompt fits. Add is secondary beside the field, reducing competition with Create and add tag. Creation fields and the new color picker occupy the available width.
- **Source Find, 390/320 both:** field fills to fixed Close; Previous stays one complete line; option units wrap whole at 320 rather than splitting their words. Source viewport remains **240px**. Flat readable controls, upright Replace prompt and matching Close treatment remain coherent. Query `meta` was entered, no replacement invoked, then Find dismissed.
- **Paste, 320/both:** complete “Paste HTML here…” prompt fits. A temporary mixed HTML/CSS/JS draft renders differentiated syntax in both schemes. Actual Select All remains visibly blue through the final nonempty `</script>` row across light/dark. Cleared afterward: hidden `#body` length **0**, placeholder restored. This was not a repeat of the exhaustive token/paint matrix.
- **Go to line, Paste, 320 dark:** label, field, Go and Close are complete and aligned. Navigated to `3:8` without accepting a completion or changing content through navigation. A Control+Space attempt did not produce a visible completion list; **completion centering/inset is not independently verified this round**.
- **Library/spaces, 768/both:** two readable card columns, complete active bold shared-space labels and separate counts; no orphaned “me” or displaced navigation. Search is visible in the rail. Title/tag/menu anchors are more coherent. The 240px rail remains generous but does not require redesign for 9.
- **Search:** desktop dark blank/results (`run`, `packing`)/miss and 320 both miss retain clear hierarchy and integrated clear. The shorter miss helper has no isolated “tag.”, and Clear is a full-sized action. Incoming owner attribution remains readable.
- **Add:** Agent at 320/both; URL and Paste at desktop dark; empty/populated Paste at 320/both. One page title and visible Library return, consistent captions/upright prompts, persistent selected-method tint, coherent phone action stack. No agent execution, URL fetching or submission.
- **Settings:** top/Profile/Administration/Storage at 320/both and lower Sign out/Delete explanation at 390 dark. Comparable values now share weight; headings and facts retain clear roles, and disabled destructive explanation remains readable.
- **Share's visible upper portion, all inspected sizes/both:** concise distinct warning, stronger section heads, full People prompt, paired URL/Copy, 44px Replace/Revoke and aligned grant edge are improvements. The helper remains below Replace on phones. These repairs do **not** establish successful lower-sheet composition because scrolling fails.
- **Viewer, 390 dark:** settled About/posture and More popovers have opaque app surfaces and readable icon/text hierarchy over the light user-authored body. That light body is outside scoring.

## Minor opportunities and coverage limits

- At 320, Find options occupy a 2+1 arrangement. This is deliberate whole-unit wrapping, with readable targets; I do not require smaller text or abbreviations to force one row.
- Share's warning still occupies four lines at 390, but it is concise, visually subordinate and necessary. Its wording is not a separate blocker. Repair containment before further copy changes.
- Light source hints/active band remain quiet. The observed selection leads; I do not require more saturated secondary paint.
- Muted tag swatches and selected state look harmonious. Arbitrary custom colors, long names, many grants and every enlarged-text scenario were not exercised.

Used only my own T3 tab **tab_16_0d23e38c-d413-4e76-b0eb-59687a206343**, opened after `preview_status` with `reuseExistingTab:false, open:false`. Every subsequent call supplied its exact ID. Explicit `preview_set_appearance` and live `matchMedia('(prefers-color-scheme: dark)').matches` returned false/true at **1280, 768, 390 and 320px**. No root-class simulation. No other tabs were read or operated.

Read the required briefs, prior visual reports for rounds 5–17 (round 10 Opus is a no-score availability record), response notes and `nine-validation.md` as context. Did not read the round-18 peer report, nine scores/summary/resume, ticket or parent checkpoints. Validation context described final runs as running when read; no test result or partial capture manifest was used as visual proof.

Primary evidence is current live images and measured DOM geometry/scroll behavior. No supplementary saved screenshots or current capture manifest were inspected, and no captures were written. Coverage is sampled: desktop dark Search states, both-theme 768 Library/incoming/outgoing, both-theme 320 Search miss/Agent/Settings/Tag/Duplicate/Paste, both-theme 1280 history and 390/320 Find, both-theme 1280/390/320 Share, and dark 390 viewer overlays. Not every surface was checked at every size/theme. Filters, desktop Duplicate, completion/detail, special characters, arbitrary bracket/fold variants, dirty/invalid/post-save states and hosted identity were not freshly verified. Resizing retains desktop Chromium/fine pointer; no physical phone, keyboard, alternate-engine or accessibility certification.

No builds, tests, commits, application edits, capture writes, persisted fixture changes or subdelegation. No valid saves, creates, deletes, grants, link replacements or data-mode changes. Temporary Paste draft cleared; dialogs dismissed unchanged. **Only this assigned report was written.**
