# Round 34 — independent Sol tag UI review

The complete live core matrix and the isolated Add/long-label matrix are viewed. The round-33 visual objections are resolved in the current renders. **One source-derived keyboard validity inconsistency remains: workflow clarity and feedback scores 9.5 in each appearance.** I did not exercise the server-error/Enter path on either frozen fixture. This is an independent assessment, not a joint pass or completion claim.

I read `tags-ten-brief.md`, both round-32 reports, both prior round-33 reports, both round-33/34 change notes, and the supplied runtime identity, playground identity and validation files. I did not read a current round-34 peer report/status/launch, score/summary/resume aggregate, ticket, or parent checkpoint. No subdelegation occurred. Scope is only Tags and Edit tag; unrelated editor content contributes no score.

## Candidate and provenance

- Frozen application candidate: `62328d495baa4dc3c049ffa818b0679980938221`; live assets: `ef44b49026ee`.
- Entry HEAD was that candidate. Cleanup HEAD was `2e0940c2f39843c59ec8b56fb0164dc1b5e65722`. At both entry and cleanup, the **entire** comparison and porcelain scope was empty, without exclusions: `git diff 62328d495baa4dc3c049ffa818b0679980938221 -- internal web e2e cmd scripts Makefile go.mod go.sum` and `git status --porcelain -- internal web e2e cmd scripts Makefile go.mod go.sum`. Cleanup also used `git diff --exit-code` and returned zero. The later HEAD therefore does not change this scoped application candidate.
- MAIN: [Run Log](http://t3code:18280/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit), default profile. My exact tab was `tab_2q_0d23e38c-d413-4e76-b0eb-59687a206343`.
- Isolated sample: [Tag editing example](http://t3code:18580/artifacts/252b5594-bfb3-4df5-bb9e-3d91579a5be8/edit), **incognito**, never default. My exact tab was `tab_2s_0d23e38c-d413-4e76-b0eb-59687a206343`. Only this profile needed login. Every preview operation after opening targeted its exact owned tab ID.
- DOM asset URLs and successful loaded resources on both instances used `ef44b49026ee`, including tags/color-picker modules and gallery styles.
- Independently read hashes of `bin/server`, `/proc/283639/exe` (MAIN), and `/proc/284106/exe` (sample) matched `37a01fee1647cfbc6657e45ab0cf03d513ba48b0b4b7daf3544a8b82abf1ae22`; both process executable paths resolve to this worktree's `bin/server`. Process hashes matched again at cleanup.
- The supplied identity records source fingerprint `240e924dd429ff3bc85da24b2778024d879ed12e1e130a657bc663972bfdac2f` and **no binary VCS stamp**. Commit provenance rests on supplied build/source identity, the empty strict comparison, live assets and matching executables. I did not extract a commit from binary metadata or independently recompute the fingerprint.

## Independent scores

| Category | Light | Dark |
| --- | ---: | ---: |
| Typography | 10 | 10 |
| Composition and spacing | 10 | 10 |
| Color harmony | 10 | 10 |
| Contrast and emphasis | 10 | 10 |
| Component visual consistency | 10 | 10 |
| Responsive visual polish | 10 | 10 |
| Tag workflow clarity and feedback | 9.5 | 9.5 |

These are unrounded opinions of the inspected scope. I found no actionable visual finishing issue in the viewed panel, palette, custom/invalid fields, sheet, footer, confirmation, or long-label/Add sample. The workflow deduction is the concrete source finding below; it is not a claim that I reproduced a persisted failure.

## Actual viewed coverage

Each pair used real `preview_resize` and `preview_set_appearance`, followed by actual `innerWidth`/`innerHeight` and **both** color-scheme `matchMedia` queries. Every panel/sheet listed below was returned with `includeImage:true` and personally viewed. No hidden frames, unviewed resizes, parent screenshots, tests or metadata-only checks fill a cell.

| MAIN CSS viewport | Appearance | matchMedia light / dark | Tags controls viewed | Edit tag sheet viewed |
| --- | --- | --- | --- | --- |
| 1280 × 800 | Light | true / false | Two overlapping exposures, including complete Create button | Health preset, entire body/footer |
| 1280 × 800 | Dark | false / true | All controls in one exposure | Reading custom, entire body/footer |
| 768 × 900 | Light | true / false | Complete panel in one exposure | Reading custom, entire body/footer |
| 768 × 900 | Dark | false / true | Complete panel in one exposure | Health preset, entire body/footer |
| 390 × 844 | Light | true / false | Two overlapping scroll exposures | Health preset, entire body/footer |
| 390 × 844 | Dark | false / true | Two overlapping scroll exposures | Reading custom, entire body/footer |
| 320 × 740 | Light | true / false | Two overlapping scroll exposures | Reading custom, entire body/footer |
| 320 × 740 | Dark | false / true | Two overlapping scroll exposures | Health preset, entire body/footer |

That completes the **16 core states**. Ordinary page scrolling exposes the lower creation controls; it is not clipping. Measured document horizontal overflow was zero at 768, 390 and 320 light, and in all four isolated sample pairs. Incidental restored-pencil focus and pointer hover fills were distinguished from resting states and were not scored as defects. An initial unsuccessful locator attempt did not open a sheet; its panel image was not counted as sheet coverage. The subsequent actual open Health sheet was viewed and verified `open:true`.

| Isolated sample CSS viewport | Appearance | matchMedia light / dark | Actually viewed |
| --- | --- | --- | --- |
| 1280 × 800 | Light | true / false | Long attached label, placeholder/disabled Add, selected Utilities/enabled Add, lower creation controls |
| 1280 × 800 | Dark | false / true | Same, in overlapping top/lower images |
| 320 × 740 | Light | true / false | Same, in overlapping top/lower images |
| 320 × 740 | Dark | false / true | Same, in overlapping top/lower images |

Utilities was selected and the placeholder restored at **each** pair, without pressing Add. The long label “Neighborhood walks and favorite routes” wraps to two lines, retains its dot, and leaves Edit/Remove aligned and unobstructed at both widths. The row separator ties the label and utilities together. Placeholder disables Add; Utilities enables it; restoring the placeholder disables it again. There were no `/api/tags` resource entries during these local selections.

## Remaining blocker to 10

**P2 — Make Edit's keyboard submit honor the same known-rejected-name validity as its disabled Save button and Create. Source-derived, not live-reproduced.**

The current source has a complete, concrete path:

1. An Edit PATCH returning 409 calls `nameError('tag-edit-name', …)` (`web/gallery/tags.js:256`), leaving the entered name marked invalid. `syncSave()` disables Save tag while that attribute is true (`:202–206`).
2. Enter in either Edit Name or Edit hex calls `saveTag()` regardless of the button's disabled state (`:316–318`).
3. `saveTag()` checks pending state, then calls `readDetails('edit')` (`:277–281`). For any nonblank name, `readDetails()` clears the name error/invalid attribute **before** it returns details (`:80–96`, especially `:87`). With a valid color and the same dirty duplicate name, the code issues another PATCH. Shared modal keyboard handling only wraps Tab and does not block this Enter route.
4. Create already prevents this exact retry with a pre-validation guard for a nonblank `aria-invalid=true` name (`:165–168`). Edit lacks that guard.

Thus an unchanged rejected Edit name is unavailable through the visible Save button but resubmittable through Enter, clearing the existing field feedback and entering Saving again before another rejection. This falls short of the round-34 response's claimed common validity boundary. It does not imply a successful invalid write or data loss.

**Concrete fix:** apply the same rejected-name guard before `readDetails('edit')`, or centralize submit eligibility so the button and Enter handler share it. Keep the rejection visible and focus Name until its value is edited. A parent disposable-fixture regression should cover Edit 409 → unchanged Name/hex Enter → no additional PATCH, then correction → normal eligibility. I ran no test and pressed no Enter in these fields.

## Fresh reassessment of earlier objections

- **Invalid-color border — resolved visually and computationally.** In the Light 1280 sheet, partial `#37` computed `rgb(180, 35, 24)`; in the Dark 320 sheet it computed `rgb(242, 114, 106)`, matching danger. The create field in Dark 320 had the same danger border. Name and hex now have consistent invalid treatment.
- **Hint/error, stale caption/sample — resolved.** The invalid error replaces the hint, its top is **6px** below hex, the selection caption is hidden, all presets are unchecked, and the native sample is masked with a neutral empty shell. The previous Custom caption/color no longer certifies an incomplete value. Viewed Light desktop and Dark phone sheet errors, plus the complete Dark phone create error/button exposure.
- **Validation geometry — resolved in the specimens.** Light 1280 dialog was `(400,54.75,480,690.5)` before and after simultaneous blank Name/partial hex. Dark 320 dialog was `(0,34,320,706)` before and after the same errors. No error-induced recentering occurred.
- **Create/Save button validity — resolved for local blank/hex states.** A valid local create name enabled Create; partial hex disabled it despite the nonblank name. Restoring blank name/default hex left it disabled. A keyboard-selected valid Red draft enabled Save; blank Name or incomplete hex disabled it; restoring stored values returned clean/disabled Save and cleared field errors. The server-rejected Edit keyboard exception remains above.
- **Row/action grouping — resolved.** Quiet separators make each desktop row legible as a unit. Phone actions remain aligned; compact pills do not stretch around utilities. The isolated long label also wraps without collisions.
- **Palette composition and original check contour — resolved.** Subtle tile outlines give the 4×2 grid a coherent structure. Full circular samples clear the inset badge in creation and the sheet, including the narrow 320 cells. No corner fringe or original badge/sample collision recurred.
- **Custom versus focus — resolved.** Reading has a Custom caption, no false checked preset, and no custom-row focus outline. Keyboard focus remains a separate visible ring.
- **Scope, unavailable Add, mobile Save — resolved within live scope.** MAIN accurately says all existing tags are attached; there is no impossible Add. The panel explains immediate add/remove persistence. The sheet explains library-wide changes and labels its explicit action Save tag. Add errors are source-located immediately after the Add row; I did not induce one.

## Keyboard, local copies and supplementary views

- Name received initial focus. At Light 320, Tab reached the custom palette's single Gray tab stop, then ArrowRight selected Red. Live DOM showed exactly one checked radio and one radio tab stop, matching Red caption and hex/native `#b86562`. The focused selected tile image was viewed; the ring and inset check remained distinct.
- Successive Tab presses traversed native picker → hex → Delete tag… → Cancel → enabled Save tag → Close, wrapping inside the sheet and matching the visible order. Escape restored focus to the originating Reading pencil.
- The safe Delete tag… entry and confirmation were viewed at Light 320. It named stored Reading, stated library-wide removal and artifact retention, and initially focused Keep tag. Keep returned to editing with the Red working copy intact and focus on Delete tag…. I restored `#8a5db0` before dismissing. Final Delete was never activated.
- Supplementary Dark **320 × 640** Health sheet: viewed initial and scrolled-bottom images; body measured 485px client / 563px content. Header/footer stayed available and scrolling revealed Delete entry.
- Supplementary Light **640 × 360** Reading sheet: viewed initial and bottom images; body measured 172px client / 542px content. Body scrolling reached lower controls while header/footer remained available. Initial autofocus scrolled the body toward Name; these supplementary images do not replace core coverage or claim every intermediate landscape body position was viewed.
- Blank Name, partial hex, temporary create name and preset drafts were local input changes only, restored/cancelled. Before the cleanup reload, MAIN resource timing showed **zero `/api/tags` requests** across these specimens.

## Limits and cleanup

I did not personally exercise valid Save/Create/Add/Remove/Delete, pending/success/server-error persistence, 409 rejection, partial-create/attach recovery, or htmx refresh after mutation. The identified Enter inconsistency is a source-path finding. The parent's disposable outcome screenshots/tests are supplementary evidence described in the supplied notes; I did not view their images or inherit their outcomes as my live observations. I did not open the native color dialog. No Safari, physical-phone or screen-reader certification follows from Chromium resizing and sampled keyboard checks.

The supplied validation says full standard Go short is still pending; I did not run it or any build/test. Passing checks do not award these scores or waive that pending validation.

Final MAIN reload and a viewed clean Tags panel confirmed Health `#377a64`, Reading `#8a5db0`, Test `#8470ac`, Utilities `#23559e`; blank create Name; default hex/native `#6b7280`; Create disabled; no visible errors or open dialog; header Saved with artifact Save disabled. Isolated sample cleanup confirmed its original three attached names/colors, Utilities still available, placeholder selected, Add disabled, blank create Name/default color, and no errors/dialog. I reset each owned tab's appearance to system and successfully closed **only** my two tabs. Strict scoped diff/porcelain remained empty. No application/fixture/test/build/commit changes were made. This report is the only file I wrote; screenshots were viewed in tool history without saving additional files.
