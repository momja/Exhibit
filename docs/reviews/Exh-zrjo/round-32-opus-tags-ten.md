# Round 32: independent Opus review of tag editing (target 10)

Reviewer: Claude Opus 5.5, an independent delegated task. I did not read the current peer's report, status or scores, or any tags-ten aggregate.
Scope: only the artifact editor's **Tags** section and its **Edit tag** sheet.

## Candidate and scope proof

- Worktree HEAD `4e7509a3` is an evidence-only commit on top of the frozen candidate.
- `git diff --stat c1156ef15c7f5caa282b1068214492bce42018e1 HEAD -- internal web e2e cmd scripts Makefile go.mod go.sum` was **empty**, with no exclusions.
- The live page loaded asset version **`aa7a00afd7a2`** (`/assets/v/aa7a00afd7a2/...`). This matches the brief.
- The live instance was MAIN `http://t3code:18280`, Run Log `/artifacts/461183aa-…/edit`. The attached tags were Health `#377a64` (preset), Reading `#8a5db0` (custom), Test `#8470ac` and Utilities `#23559e`.
- I used my own tab, opened with `preview_open(profileId:default, reuseExistingTab:false, open:false)`. Every call used that tab's exact id. I touched no other tab.

## Scores (0–10, no rounding up)

| # | Category | Light | Dark |
|---|----------|------:|-----:|
| 1 | Typography | 9 | 9 |
| 2 | Composition and spacing | 8.5 | 8.5 |
| 3 | Color harmony | 9.5 | 9 |
| 4 | Contrast and emphasis | 8.5 | 8.5 |
| 5 | Component visual consistency | 8.5 | 8.5 |
| 6 | Responsive visual polish | 8.5 | 8.5 |
| 7 | Tag workflow clarity and feedback | 7.5 | 7.5 |

The candidate is **not a 10**. The visual base is good. The remaining issues are mostly about feedback and clarity, plus two layout finishing problems.

## Rendered coverage (all actually viewed, settled)

For each pair I called `preview_set_appearance` and then checked `matchMedia('(prefers-color-scheme: dark)')` in the same tab.

| Viewport | Light: panel / sheet | Dark: panel / sheet | matchMedia |
|----------|----------------------|---------------------|------------|
| 1280×800 | ✔ / ✔ Reading (custom) + preset working copy | ✔ / ✔ Reading, plus blank-name and partial-hex probes | false / true |
| 768×900 | ✔ / ✔ Health (preset) + keyboard focus on swatch | ✔ / ✔ Reading (custom) | false / true |
| 390×844 | ✔ / ✔ Reading (custom), stacked footer fully visible | ✔ / ✔ Health (preset) | false / true |
| 320×740 | ✔ / ✔ Reading, footer fully visible, body not scrolling (302/302) | ✔ / ✔ Reading | false / true |

That is all 16 required renders. I did not render the supplementary 320×640 or 640×360 sheets.

I discarded one hover artifact: a residual pointer hover fill on the Health pencil at 320 light. It is neither scored nor certified.

## Blockers to 10 (prioritized)

1. **"Add existing tag" error is in the wrong place, and the Add row can be a dead end.**
   - **Observed live (1280 dark):** pressing Add with nothing selected shows "Choose an existing tag to add." *under the Create and add tag button*. It reads as a creation error, about 300 px from the control it refers to. This is because `#tag-add-error` sits after the whole panel in `edit.tmpl`.
   - On Run Log the select has only its placeholder option: every tag is already attached. The user can still open an empty select and press Add.
   - **Fix:**
     - Render the add error directly under `.tag-add-row`, with `role="alert"`.
     - When `Available` is empty, show muted text such as "All your tags are on this artifact", or disable the select and Add with that explanation.
2. **The Edit tag sheet never says its changes are library-wide and immediate.**
   - The template has only the title "Edit tag", Name and Color. The library-wide scope is stated only inside the delete `confirm()`.
   - On phones the sheet's **Save** appears with the page header's "Saved / Save" still visible above the backdrop. This is the original ambiguous-Save concern, now in the sheet.
   - **Fix:**
     - Add one helper line under the title, for example "Changes apply to every artifact tagged *Reading*." A count would be even better.
     - Consider labeling the button "Save tag".
3. **The custom-color indication looks exactly like keyboard focus.**
   - `.color-custom-row.is-custom` draws a 1px accent outline at offset 3. Around the fields' own borders, that produces the same double-ring silhouette as the focused Name field.
   - **Observed live:** both rings showed at once at every pair with Reading, and the effect is most obvious at 768 dark. Typing in the hex field stacks a third ring.
   - **Fix:** make the selected-custom state read as selection, not focus. For example, use a check badge or "Custom" chip on the native swatch, matching the preset check. Or use an accent-tint fill without an outline.
4. **Desktop and 768: the selection badge still covers the preset dot.**
   - Measured at 1280: the 16 px badge at inset 3/3 covers about 10×10 px of the 18 px dot's upper-right quadrant. At 768 the sheet's selected green dot is visibly bitten.
   - At ≤480, with 22 px dots in 44 px cells, the badge clears the dot. That is the reference look.
   - **Fix:** at desktop, either keep the 36 px cell but offset the dot down and left, or drop the corner badge for an inset ring around the dot plus a centered check. Do not just shrink the numbers.
5. **Validation feedback in the sheet.**
   - **Observed live (local probe, no request per source):** a blank name shows "Name is required." *below the color row*. The Name field gets no `aria-invalid` or invalid styling and does not receive focus. `#tag-edit-error` has no `role`/`aria-live`, unlike the create error's `role="alert"`.
   - A partial hex such as `#8a5d` shows no inline invalid state. The native swatch keeps the old color and the custom ring stays, so Save would send an invalid value to the server.
   - **Fix:**
     - Put the error beside the offending field, mark it invalid and move focus to it.
     - Give the sheet error `role="alert"`.
     - Flag a malformed hex inline, or block Save until it is `#rrggbb`.
6. **The 390 attached-tag list wraps raggedly.**
   - **Observed live (390 light and dark):** with 44 px utility slots, the four tags fall into rows of 1, 2 and 1. Every row has a different rhythm, and the 16 px icons sit loosely in wide slots.
   - At 320 the list is one per row, but the pencil and × columns zigzag with label length.
   - **Fix:** under 768, lay out one tag per row with the pill on the left and the actions right-aligned in a fixed column. That gives a scannable list.
7. **Minor items:**
   - Library-wide Delete confirms with a native `confirm()` stacked over the styled sheet. That is functional but out of grammar; a sheet-styled confirm step would match.
   - The footer's DOM order is Save, Cancel, Delete, but it displays as Delete … Cancel Save. Tab therefore runs right-to-left. This is shared modal grammar, so I note it and do not score it heavily.
   - Swatch accessible names are raw uppercase hex ("#377A64"). Color names such as "Green" would help screen-reader users.
   - In dark mode, dark tag colors such as Utilities `#23559e` have low contrast on the near-black pill. A 1px light separator ring on `.tag-dot` in dark mode would help.
   - Pill labels are 12 px muted ink, while the Edit/Remove glyphs are full-ink. The utilities outweigh the names slightly.

## Original defects: status

- **Badge flush against the outer corner:** fixed. The 8 px radius, 1px border and 3 px inset render cleanly, with no fringe at any pair.
- **Badge-over-dot mismatch:** remains at desktop and 768 (blocker 4). It does not occur at ≤480.
- **Stretched capsule with spread-out Edit/Remove:** fixed. The capsules are compact and the utilities sit 4 px away. A different arrangement issue remains at 390 (blocker 6).
- **Ambiguous mobile Save:** not fully resolved for the tag sheet (blocker 2).

## Keyboard and working-copy checks (live)

- Edit opens with focus on Name, and the working copy loads the stored values: Reading `#8a5db0` shows the custom ring with no preset pressed, and Health `#377a64` shows its preset pressed.
- Choosing a preset sets `aria-pressed`, syncs the hex, and clears the custom ring.
- Escape, Cancel and the header close button each close the sheet, discard the working copy (reopening showed `#8a5db0` again), and restore focus to the triggering pencil with `:focus-visible`.
- Tab order: Close → Name → 8 swatches → native picker → hex → Save → Cancel → Delete. The focus ring on a selected swatch is visible.
- **Not exercised live:** valid Save, create, add, remove, delete, pending/success/server-error states, htmx refresh after a mutation, and the native color dialog. I only read their source; the parent's disposable fixtures must cover them.

## Restoration and cleanup

- I made no valid Save, create, add, remove or delete, pressed no Enter in a creation field, and made no app, build, test or fixture change.
- Probe working copies (blank name, partial hex, preset choice) were cancelled.
- The page was reloaded once to clear the local add-error.
- Final check: the server partial still lists Health `#377a64`, Reading `#8a5db0`, Test `#8470ac` and Utilities `#23559e`. The header shows "Saved" with Save disabled, no dialog is open, and the create name is empty.
- I reset appearance to `system` and closed my own tab. This report is the only file I wrote.

This is one reviewer's opinion. It is not a joint completion claim or a universal browser/accessibility certification.
