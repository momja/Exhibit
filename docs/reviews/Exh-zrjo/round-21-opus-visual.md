# Round 21 — independent Opus visual review (target 9)

Reviewer: **claude-opus-5-5**, 2026-11-04, about 20:26–20:32 UTC (live session).

**Application under review**

- Frozen application **74544552a816a3ac3a6dd0c04f8d0713fe223cce**. Served assets **a48067e4e6f5**, confirmed from live versioned asset URLs on every page I loaded.
- Base: origin/main **44def51a**.
- HEAD at exit was **5a3e4005** (evidence commits only). A read-only `git diff 74544552..HEAD -- internal web` was empty.

**What I read**

- `visual-brief.md` and `nine-brief.md`.
- R20 Opus and R20 Sol in full, and R19 Sol's findings. Earlier rounds 5–18 and the R18/R19 Opus records are known through the brief's summary of findings and responses.
- Change notes 19, 20 and 21, and `nine-validation.md`.

**What I did not read**

- `round-21-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md`
- the ticket and any parent aggregate

These scores are fresh and were not shaped by any peer rating. Tests did not decide any score.

**Verdict: FAIL against the all-twelve-at-least-9 target, narrowly.**

- Every R20 Opus finding I re-checked is repaired, and the repairs are good. The posture receipt is now a finished component.
- Eleven of twelve categories score 9.
- **Responsive visual polish scores 8.5 in both schemes** because of one newly found short-phone state: **Share opens part-way scrolled at 320×640.**

This single-model report establishes no joint pass and makes no council-completion claim.

## Twelve scores

| Category | Light | Dark | Weakest material surface |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | The receipts now use the 14/21 phone helper token and sentence-case 600 headers. Form captions, helpers and section heads share one scale. "Create new tag" (14/600) now leads its 13px captions. |
| Composition and spacing | **9** | **9** | History columns are shared, and the colour row sits on the field edges. Filters uses the sheet shell. The Paste phone footer is helper, then primary, then Cancel. The card receipt flips up and stays inside the viewport. |
| Color harmony | **9** | **9** | A coherent slate/blue system in both schemes. The native colour well now has the slate field frame. Presets are muted. |
| Contrast and emphasis | **9** | **9** | Primaries lead, Delete is a quiet outline, and selected method, preset and choice states are unmistakable. Find's filled Next leads the outline siblings. |
| Component visual consistency | **9** | **9** | Shared sheet header/body/footer across Duplicate, Edit tag, Share and Filters. One receipt idiom for card and viewer. 18px checkboxes with description text. Trivial residue only (see Minor). |
| Responsive visual polish | **8.5** | **8.5** | **Share at 320×640 opens with its body already scrolled 351px**, so the first frame shows a cut-off "Replace link" under the header. The other 1280/768/390/320 compositions are deliberate. |

## Material finding

### Share opens mid-sheet on short phones (new; observed live, 320×640 light)

**Route:** `/artifacts/461183aa…` → pill Share, opened with a real pointer click.

**What happens**

- The sheet focuses the People field ("Usernames or emails", `:focus-visible` true).
- That focus scrolls `.modal-body` to `scrollTop` **351** (clientHeight 477, scrollHeight 1124).

**What the user sees first**

- The header, then the bottom half of the Replace link button. The connected-accounts notice, "Link access", the checkbox, its explanation and the link field are all above the fold.
- The focused input is in the middle of the sheet, with a heavy focus ring.
- On a real phone, focusing a text field on open would also raise the on-screen keyboard over the sheet. That part is inferred: I used a single engine, not a physical device.

**At 390×844 (dark)** the same focus lands without scrolling (`scrollTop` 0), so the composition is correct there. The defect only appears where the People field falls below the fold, which includes 320-wide and other short phone viewports.

This is the one place where a sheet's first impression looks unfinished. Every other sheet opens at its top: Edit tag focuses Name at the top, Duplicate focuses Title at the top, Filters focuses Close.

**Treatment to earn 9**

- On open, keep the body at its top. Options:
  - focus the dialog heading or the Close button;
  - focus the first control (the link checkbox);
  - call `focus({preventScroll: true})` on People only when it is already in view.
- Do not autofocus a text field on coarse pointers. A typing affordance should wait for a tap.

No new content or ornament is needed.

## Minor opportunities (trivial; none holds a category below 9 on its own)

- **Share → Embed tile helper** ("Embeddable tile showing this t…") is still `P.text-sm.muted` in `rgb(71,85,105)`. The other Share helpers are `.form-help` in `rgb(93,104,120)`, both at 14/21. This is the last two-tone helper, and it sits inside a collapsed disclosure.
- **Editor Tags panel at 1280.** "Add" is 44px tall but "Create and add tag" in the same panel is 36px. At 320 both are 44px. Use one height in the desktop panel.
- **Share's saved-data copy repeats itself.** "Public links always show your saved data and remain read-only" appears as a helper and again at the end of the "What happens when I switch?" disclosure.
- **Card footer control sizes differ.** The posture trigger is a 21×32 target next to the 32×32 kebab.
- **Dark Share radios.** The unselected dark radio renders as a filled grey disc rather than a ring. It is native and readable.
- **Two label idioms.** Uppercase eyebrows are used for read-only facts (Settings terms, rail "Collections/Tags", Filters "Owners"), sentence-case captions for form fields. The split is consistent and defensible.

## Prior-repair verification (fresh, live)

**Posture receipt, card**

| Viewport | Placement | Document height |
| --- | --- | --- |
| 1280×800 light | 320×323 panel at x264/y191, flipped above the trigger | stayed **800** |
| 768×900 dark | flipped to y407–730 | **900** |

- Body text 14/21. Headers "Sandbox posture" and "Sharing" are 14/600 sentence case.
- Hairline section dividers; "Manage security settings →" has a 36px target.
- The Sharing paragraph runs to 4 lines at a comfortable measure. It looks finished in both schemes.

**Posture receipt, viewer (390 light)**

- The same idiom: header 14/21/600, body 14/21, Manage link 44px.
- The receipt's accent focus ring appears only after keyboard interaction. After a pointer-only open, `:focus-visible` was false and there was no outline.

**History columns (1280 dark)**

- Both dates start at x716. v2 shows the Current badge; v1 has View/Restore at 860–1046.
- The 108px stagger is gone.

**Tag colour row**

- 1280 light, editor: name field 234–554, well 234–278, hex 286–554.
- 320 dark, Edit tag: name 21–299, well 21–65, hex 73–299. The well frame is `rgb(100,112,131)` with a 6px radius; no pure grey remains.
- The sheet has a 4×2 muted preset grid with the selected Health cell marked, and full-width Save / Cancel / outlined Delete.

**Filters (390 dark, `/shared`)**

- Shared header with a 44px Close and a separator.
- The "Owners" list aligns at x21 with the title and a full-width Done.

**Paste footer (390 dark)**

- The note sits above the full-width primary and Cancel. The method segment shows the persistent selected tint, and the "Paste HTML here…" prompt is complete.

**Phone tabs (390 dark, 320 light)**

- Balanced "Shared / with me" and "Shared / by me", with no orphan words.

**Desktop kebab**

- A real 32px slot (y307–339), optically centred on the title's first line in all four 1280 cards.

**Duplicate (1280 dark)**

- Sheet shell, 18px checkboxes with titles leading their helpers, and the trailing Cancel / primary footer. Dismissed unchanged.

**Share scroll, paint and lower disclosure (390 dark)**

- Body 681/1061. Scrolled to the bottom with "What happens when I switch?" expanded.
- Both saved-data choice cards are complete, the radios are inside their cards, and Done stays fixed. No detached paint.

**Native Find/Replace (1280 dark, existing source)**

- The field flexes to Next, Previous and All. Close is a separate 36px square.
- The Replace row's buttons end flush with All. 18px option checkboxes.
- The source viewport stays at 257px above the panel. The source stayed "All changes saved".
- Closed with the panel's Close button.

**Library, Search, Settings**

- 1280 light: four 236px columns.
- 768 dark: two 232px cards beside the 240px rail, which is comfortable.
- Search miss at 320 light: integrated clear, balanced two-line empty state, a 44px Clear search, and the active Search tab.
- Settings at 320 dark: aligned fact groups, consistent icon headings, and a restrained destructive region with its disabled explanation (seen in the page text).

## Retained judgement calls, reassessed

- **240px rail at 768.** Comfortable; not a blocker.
- **Quiet source echoes and active band.** Not re-run as a paint matrix this round. Prior evidence plus today's Find view support the hierarchy.
- **Completion's square property marker.** Utilitarian; not re-opened this round.
- **User widget content** (for example the Reading List tile's clipped title) is excluded as user-authored.

## Coverage and limits

**Session and data handling**

- I used only my own tab, `tab_1b_0d23e38c-…`, opened with `reuseExistingTab:false, open:false`, and passed its exact id on every call. It is now closed.
- I was already signed in on `t3code:18280`.
- Scheme was set only with `preview_set_appearance`. `matchMedia('(prefers-color-scheme: dark)')` returned the expected false/true at 1280, 768, 390 and 320. I used no root-class simulation.
- Every dialog, menu and popover was dismissed unchanged.
- I typed only into the source Find field ("run"), and closed it without replacing anything.
- I made no saves, creates, deletes, grants, link replacements, data-mode changes, builds, tests, commits, capture writes or subdelegation. **I wrote only this file.**

**What was observed live, and what was not**

- Evidence is live screenshots plus computed geometry and style. I did not use saved captures, and none of my scores rely on them. I did not read source for scoring.
- Sampled rather than exhaustive:
  - Not re-checked this round: completion, Go-to-line, source selection paint, URL/Agent Add modes, the admin users page, the card menu at phone widths, outgoing `/shared-by-me`, and 768 light.
  - Not exercised: long or arbitrary data, enlarged text, invalid or dirty states.
- Single Chromium engine with emulated viewports. No physical phone (the keyboard effect in the material finding is inferred), alternate engine, hosted identity or accessibility certification.

## Scores (light / dark)

- Typography: 9 / 9
- Composition and spacing: 9 / 9
- Color harmony: 9 / 9
- Contrast and emphasis: 9 / 9
- Component visual consistency: 9 / 9
- Responsive visual polish: **8.5 / 8.5**
