# Round 33: independent Opus review of tag editing (target 10)

Reviewer: Claude Opus 5.5, an independent delegated task. Before reviewing I read both round-32 reports (Opus and Sol) in full, `tags-ten-round-33-changes.md` and `tags-ten-validation.md`. I did not read the current same-round peer's report or status, any tags-ten scores, summary or resume aggregate, or the ticket.

Scope: only the artifact editor's **Tags** section and its **Edit tag** sheet.

## Candidate and scope proof

- Worktree HEAD is `72be0ccc60f8b8820ad0715c5113f5a6f0cb44aa`.
- `git diff --stat 72be0ccc… -- internal web e2e cmd scripts Makefile go.mod go.sum` was **empty**, with no exclusions.
- `git status --porcelain` over the same paths was also empty.
- The live page loaded only asset version **`bc262e992d4d`** (`/assets/v/bc262e992d4d/…`).
- The listener is PID 275988, which resolves to this worktree's `bin/server`. `sha256(bin/server)` and `sha256(/proc/275988/exe)` are both `b2cfda01…5c2c`, matching `tags-ten-runtime-identity.json`.
- The binary has no VCS stamp. Provenance therefore rests on the clean scoped source, the matching assets and the matching executable hash. No revision was extracted from the binary.
- Instance: MAIN `http://t3code:18280`, Run Log `/artifacts/461183aa-…/edit`.
- I opened my own tab with `preview_open(profileId:default, reuseExistingTab:false, open:false)` and used its exact id, `tab_2l_0d23e38c-…`, on every call.

## Scores (0–10, no rounding up)

| # | Category | Light | Dark |
|---|----------|------:|-----:|
| 1 | Typography | 10 | 10 |
| 2 | Composition and spacing | 9.5 | 9.5 |
| 3 | Color harmony | 10 | 10 |
| 4 | Contrast and emphasis | 9.5 | 9.5 |
| 5 | Component visual consistency | 9.5 | 9.5 |
| 6 | Responsive visual polish | 10 | 10 |
| 7 | Tag workflow clarity and feedback | 9.5 | 9.5 |

This is a large step up from my round-32 scores of 7.5–9.5. It is **not yet 10 in every category**. What remains is small and specific, mostly in the invalid-hex state and in desktop row rhythm.

## Rendered coverage

For each pair I called `preview_set_appearance`, then checked `innerWidth`/`innerHeight` and `matchMedia('(prefers-color-scheme: dark|light)')` in the same tab. Every render below was returned as an image and viewed after it settled.

| Viewport | Light: panel / sheet | Dark: panel / sheet | matchMedia dark (light / dark) |
|----------|----------------------|---------------------|-------------------------------|
| 1280×800 | ✔ full panel / ✔ Reading (custom) | ✔ full panel / ✔ Health (preset) | false / true |
| 768×900 | ✔ full panel / ✔ Health (preset) | ✔ full panel / ✔ Reading (custom) | false / true |
| 390×844 | ✔ full panel / ✔ Reading, complete sheet and footer | ✔ full panel / ✔ Health, complete sheet and footer | false / true |
| 320×740 | ✔ panel in 2 overlapping scroll views / ✔ Health, complete sheet and footer | ✔ panel in 2 overlapping scroll views / ✔ Reading, complete sheet and footer | false / true |

That is all 16 core views.

- At 320 the 829 px panel cannot fit under the sticky header in one view, so the overlapping scroll views are ordinary scrolling, not a defect.
- At 390 and 320 the sheet body measured `scrollHeight == clientHeight` (534/534), so nothing inside the sheet scrolls.

**Supplementary renders (viewed live, not counted as core):**

- 1280 light: a keyboard-selected Red preset with its focus ring.
- 1280 light: blank Name plus partial hex `#8a5d` validation.
- 1280 light: the Delete tag… confirmation.
- 320 light: partial hex `#12` in the create form.

**Discarded:** residual pointer-hover fills on one utility button at 768 light (Reading pencil), 390 light (Health ×) and 390 dark (Reading pencil). One automation scroll jump at 320 was also discarded: my selector click scrolled an off-screen pencil into view. None of these is scored.

## Round-32 blockers: status (judged on the new UI)

| Round-32 blocker (both reports) | Status |
|---|---|
| Library-wide or immediate scope never stated | **Resolved.** The sheet says "Changes apply to this tag on every artifact in your library." The panel says "Adding or removing a tag saves immediately." The button now reads **Save tag**, which separates it from the page header's Save on phones. |
| Badge or check covering the preset dot | **Resolved.** At every width, the 16 px check at inset 3 clears the 22 px sample inside the 56 px cell, and no contour fringe appears. |
| Enabled, impossible Add with an empty select | **Resolved.** The live panel shows "All existing tags are already on this artifact." and has no select or Add. |
| Add/attach error far from its control | **Resolved by source:** `#tag-add-error` now follows the add row inside the panel body. Not exercised live, because Run Log shows no Add row. |
| Custom-color state looked like keyboard focus | **Resolved.** The outline is gone. A "Custom" caption chip replaces it and reads as state, not focus. |
| Name and hex validation | **Mostly resolved.** Name: the error sits directly under the field, the field gets `aria-invalid` and a red border, focus stays on it, and the error has `role=alert`. Hex: the error is announced and Save tag is disabled. The residual issues are listed below. |
| Ragged tag rows at 390 and 320 | **Resolved.** There is one tag per row, with a fixed right-aligned action column (pencil and × at x=272/316 at 390 and x=206/250 at 320 on every row). |
| Swatch names and dark-dot contrast | **Resolved.** Accessible names read "Green (#377A64)" and so on. In dark mode, Utilities `#23559e` now has a ring that separates it from the pill. |
| Footer DOM order | **Resolved.** The DOM order Cancel → Save tag matches the visual order at every width. |
| Native `confirm()` for library-wide Delete | **Resolved.** Delete tag… switches the same sheet to "Delete tag?" with "Delete **Reading** from your library? It will be removed from every artifact. Your artifacts will be kept." Focus lands on **Keep tag**. Keep tag returns to the edit view with focus on Delete tag… and the working copy intact. |

Original user defects:

- **Badge touching the corner:** fixed.
- **Badge mismatched with the swatch:** fixed.
- **Stretched capsule:** fixed. The capsules are compact, 13 px.
- **Ambiguous mobile Save:** fixed within this scope by the **Save tag** label.

## Remaining blockers to 10 (prioritized)

1. **Inside the Edit sheet, an invalid hex field gets no red border.** Live-measured:
   - In the sheet, `#tag-edit-color-hex[aria-invalid=true]` computed `border-color: rgb(135,146,162)`, the normal field border. The Name field directly above it turned red (`rgb(180,35,24)`).
   - In the create form, the same hex field does turn red.
   - **Cause:** `.modal-dialog input[type=text]` (components.css:446) has the same specificity as `.tag-color-fields input[aria-invalid=true]` (components.css:417) and comes later in the file, so it wins inside dialogs.
   - **Fix:** raise the invalid rule's specificity, or move it after the modal field rule.
   - This drives the consistency and emphasis deductions.
2. **The invalid-hex state sends mixed signals, in both the sheet and the create form.**
   - With `#8a5d` typed, the caption chip still reads "Custom", and the native swatch beside the field keeps the stale previous color (red in the sheet, gray in the create form).
   - The error sits *below* the persistent hint "Use the picker or a six-digit hex color.", so the field-to-error distance is 37 px, with two help lines saying nearly the same thing.
   - **Fix:**
     - Replace the hint with the error while the value is invalid, so the error sits right under the field.
     - Show a neutral or empty swatch, or no caption, for an incomplete value.
3. **The create form's validation does not match the sheet's.** In the create form, **Create and add tag** stays enabled while the hex is invalid (`disabled:false`, live), even though the sheet disables **Save tag** for the same state. I did not press it, because the source would submit on a valid path. Gate the create button the same way, or deliberately validate on click; either way, make the two forms consistent.
4. **The desktop row rhythm (1280 and 768) reads as detached.**
   - The list is 320 px wide with actions at the column's right edge. A short label like "Test" therefore sits about 200 px from its pencil and ×, and no row divider or hover band ties them together.
   - On phones the same layout reads well, because the column is the full width.
   - **Fix:** a subtle row hover or focus band, or hairline separators, would anchor each action pair to its label. So would a narrower desktop column.
5. **Minor polish:**
   - The centered dialog shifts upward by about 28 px when validation messages appear. Anchoring it to the top, or reserving the space, would steady it.
   - The unselected palette cells have no resting boundary, so at desktop the 4×2 grid of 56 px cells reads as sparse floating dots, 120 px tall. This is acceptable, and I note it only as the last composition fraction.

## Keyboard and working-copy observations (live)

- Edit opens with focus on Name.
  - Reading (custom) shows caption "Custom", no checked radio, and roving `tabindex` `0,-1,…`. Health shows the "Green" radio checked.
  - Save tag starts disabled.
- Tab from Name lands on the radio group; ArrowRight selected Red.
  - That one press set `aria-checked`, synced the hex and native picker to `#b86562`, changed the caption to "Red" and enabled Save tag.
  - The focus ring plus the selected cell rendered cleanly, with the badge clear of the dot.
- A blank Name shows "Name is required." directly under Name, with `aria-invalid` and a red border, and focus stays on Name.
- A partial hex sets `aria-invalid` and shows an alert, and Save tag becomes disabled again.
- Escape closed the sheet and returned `:focus-visible` to the exact originating pencil, "Edit tag Reading".
  - Reopening showed the stored values, so the working copy was discarded.
  - This held at 1280, 768, 390 and 320.
- In the create form, partial hex `#12` gave a red border and an alert. I then restored `#6b7280`, which brought back the Gray checked state and the "Gray" caption.
- I pressed no Enter in any name or hex field.

## Supplementary vs live limits

- **Not exercised live:** valid Save, create, add, remove and delete; pending, success, 409 and 503 states; htmx refresh after a mutation; and the add-row error placement. Run Log has every tag attached, so it shows no Add row.
- These outcomes are covered only by the parent's disposable-fixture E2E photos. I did not review those photos, and I certify nothing from them.
- I did not open the native color dialog. I did not render the supplementary 320×640 or 640×360 sheets.
- The full Go short suite is still pending, per `tags-ten-validation.md`.
- These scores are one reviewer's opinion of the inspected Chromium renders. They are not a universal browser, Safari or screen-reader certification, and not a joint completion claim.

## Restoration and cleanup

- **Writes:** I made no valid Save, create, add, remove or delete. I never pressed the final Delete tag button. I made no app, build, test, fixture or commit change.
- **Working copies:** the blank name, partial hex, Red preset and create-form `#12` were all cancelled or restored.
- **Reload and final check:** after a fresh reload, the server partial lists Health `#377a64`, Reading `#8a5db0`, Test `#8470ac` and Utilities `#23559e`. The modal is closed, the create name is empty, the create hex is `#6b7280`, and the header shows "Saved" with Save disabled.
- **Tab:** I reset appearance to `system` and closed only my own tab.
- **Files:** this report is the only file I wrote.
