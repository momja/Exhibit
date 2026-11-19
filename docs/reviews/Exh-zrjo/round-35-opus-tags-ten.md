# Round 35: independent Opus review of tag editing (target 10)

Reviewer: Claude Opus 5.5, a fresh delegated task run after the provider quota reset. It is not a continuation of the failed Opus34 attempt.

**Read before reviewing:** `tags-ten-brief.md`, both round-32 reports, both round-33 reports, `round-34-sol-tags-ten.md`, `round-34-opus-tags-ten-unavailable.md` (which has no scores), the round-33/34/35 change notes, `tags-ten-runtime-identity.json`, `tags-ten-playground.json` and `tags-ten-validation.md`.

**Not read:** any current round-35 peer report, status or launch file, `round-35-opus-tags-ten-deferred.md`, any score/summary/resume aggregate, the ticket, and any parent checkpoint.

**Scope:** only the editor's **Tags** section and its **Edit tag** sheet. There was no subdelegation.

## Candidate and scope proof

- The application candidate is `235a5c9ecddd7494a20eaef32e55c0320994adad`, with assets `b1f015b9d4bf`.
- At entry, HEAD was `99697da8`; at cleanup it was `8506d136`. Both are later evidence-only commits.
- The scope check passed at entry and at cleanup:
  - `git diff --exit-code 235a5c9e -- internal web e2e cmd scripts Makefile go.mod go.sum` returned 0 (empty).
  - `git status --porcelain` over the same paths printed nothing.
  - No paths were excluded.
- Both live instances served only `/assets/v/b1f015b9d4bf/…`. I checked the DOM asset URLs on MAIN and on the sample, and the sample's loaded resources.
- I read the SHA-256 of `bin/server`, `/proc/290478/exe` (MAIN) and `/proc/290564/exe` (sample) myself. All three are `3e9cf847e05ddf4c9ed01fd8ca15d2151c6c6069db0373246ed47cb97cd6f1bb`, which matches `tags-ten-runtime-identity.json`.
- The binary has no VCS stamp. Provenance therefore rests on four things:
  - the empty scoped comparison,
  - the live asset version,
  - the matching executables,
  - the supplied build identity.

  I did not extract a commit from the binary or recompute the source fingerprint.
- The tests are context only. The supplied validation reports the full Go short run as PASS, exit 0 in 443.14s, along with the Node, native and Playwright passes. I ran no build or test, and no test result contributes to a score.

## Independent scores (0–10, no rounding up)

| # | Category | Light | Dark |
|---|----------|------:|-----:|
| 1 | Typography | 10 | 10 |
| 2 | Composition and spacing | 9.5 | 9.5 |
| 3 | Color harmony | 10 | 10 |
| 4 | Contrast and emphasis | 10 | 10 |
| 5 | Component visual consistency | 10 | 10 |
| 6 | Responsive visual polish | 10 | 10 |
| 7 | Tag workflow clarity and feedback | 10 | 10 |

Thirteen of the fourteen values are 10. The single deduction in each scheme is one concrete, small composition issue: the desktop "Add existing tag" row (below). I am not inheriting any prior, peer or test score. This is one reviewer's opinion, not a joint pass.

## Actual viewed coverage

For every pair I did the following, all in my own tab:
- called `preview_resize` and `preview_set_appearance`;
- evaluated `innerWidth`/`innerHeight` and **both** `matchMedia('(prefers-color-scheme: light)')` and `matchMedia('(prefers-color-scheme: dark)')`;
- returned each listed render with `includeImage:true` and viewed it after it settled.

No hidden frames, unviewed resizes, parent images or tests fill any cell.

### MAIN (default profile, my tab `tab_2x_0d23e38c-…`)

Run Log, `http://t3code:18280/artifacts/461183aa-…/edit`.

| Viewport | Scheme | matchMedia light / dark | Tags panel viewed | Edit tag sheet viewed |
|---|---|---|---|---|
| 1280×800 | Light | true / false | All tag controls; the top 2/3 of Create is visible at the bottom edge | Reading (custom), entire body and footer |
| 1280×800 | Dark | false / true | Complete panel in one view | Health (preset), entire body and footer |
| 768×900 | Light | true / false | Complete panel in one view | Health (preset), entire body and footer |
| 768×900 | Dark | false / true | Complete panel in one view | Reading (custom), plus a blank-Name / partial-hex probe |
| 390×844 | Light | true / false | Top view, plus an overlapping scrolled view of the lower controls | Reading (custom), entire bottom sheet; also a keyboard-selected Red working copy with Save tag enabled |
| 390×844 | Dark | false / true | Top view showing the label of the lower Create button; the lower controls match the scrolled light view | Health (preset), entire sheet; also the Delete tag… → Keep tag confirmation |
| 320×740 | Light | true / false | Top view, plus a scrolled view with a create-form partial-hex probe | Reading (custom), entire sheet |
| 320×740 | Dark | false / true | Top view, plus an overlapping scrolled view of the lower controls | Health (preset), entire sheet |

That covers all **16 core states**.
- The document's horizontal overflow measured 0 at 768, 390 and 320 in both schemes.
- At 390 and 320 the sheet body measured 563/563, so nothing inside the sheet scrolls.
- At 1280 and 768, the dialog rect measured `(400,54.75,480,690.5)` and `(144,104.75,480,690.5)`.

### Isolated sample (incognito profile, my tab `tab_2y_0d23e38c-…`)

`http://t3code:18580/artifacts/252b5594-…/edit`. I logged in only in this profile.

| Viewport | Scheme | matchMedia light / dark | Viewed |
|---|---|---|---|
| 1280×800 | Light | true / false | Long attached label; placeholder state with Add disabled; Utilities selected with Add enabled; create controls |
| 1280×800 | Dark | false / true | Same, with Utilities selected and Add enabled |
| 320×740 | Light | true / false | Long label, add row with Utilities selected and Add enabled, create name and palette |
| 320×740 | Dark | false / true | Same |

- The Tags `<details>` was collapsed on load, so I opened it locally.
- I selected Utilities and then restored the placeholder **without pressing Add**. The placeholder state disabled Add again, which I verified.
- The sample made **zero `/api/tags` requests** during all of this.
- The sample's lower create controls are the same partial as MAIN's, which I scrolled and viewed at 320 in both schemes. I did not separately scroll them in the sample.

### Discarded artifacts (not scored)

- Pointer-hover fills on pencils after clicks: 768 dark (Health) and 1280 dark (Reading).
- Restored-focus rings on the originating pencil after Escape.

## Blocker to 10

**P3 (composition): the desktop "Add existing tag" row overhangs the shared tag column.**

Measured in the sample:
- At 1280 (and 768, which uses the same 320px column), the select is 320px wide at x=234–554, the full width of the column used by the tag list, its action column and the create form.
- The Add button then sits *outside* that column, at x=562–635, 81px past the right edge every other tag control aligns to.
- At 320 the same row fits inside the column, with the select shrunk to 187px and Add ending at the column edge (x=294). That shows the intended grid.

The result on desktop is a single control poking past the edge that the rows' pencil and × buttons and the palette all share. It is small, but it is the one non-aligned element in an otherwise strict column.

**Concrete fix:** give `.tag-add-row` (or its equivalent) the same max-width as the list and form, 320px, and let the select flex, exactly as it already does under 480px. Alternatively, keep the select at a fixed width and end the button at the column edge.

This is the only actionable finishing issue I found. It only appears when an existing tag is available, so it is not visible on Run Log, where all tags are attached.

## Fresh reassessment of earlier objections

**Original defects**
- **Check badge against the outer corner:** fixed. The 16px check sits inset inside the 8px-radius tile at every width and in both schemes. There is no corner fringe, and the badge clears the circular sample, including in the narrow 61–64px cells at 320.
- **Stretched capsule:** fixed. The capsules are compact and fit their content. The long name wraps to two lines inside a rounded capsule that stays clear of the aligned pencil and × column. Quiet separators tie each row together.
- **Ambiguous mobile Save:** fixed. The sheet's primary action reads **Save tag**, and the bottom sheet's Cancel/Save tag footer is fully visible at 390 and 320.

**Palette and custom-versus-focus**
- Reading shows a "Custom" caption chip, no checked preset, and no false focus outline.
- A keyboard focus ring (390 light, Red) renders outside the selected border and badge and is clearly distinct from the selected state.
- In dark mode the Utilities `#23559e` dot keeps a separating rim on the pill.

**Invalid states**
- 768 dark sheet, blank Name with `#8a5d` (observed live):
  - Name and hex both get `aria-invalid=true` and the danger border `rgb(242,114,106)`.
  - The errors replace the hints directly under each field.
  - The caption chip hides, and the native sample shows the neutral minus glyph.
  - Save tag stays disabled.
  - The dialog rect is identical before and after, so there is no recentering.
  - No requests were sent.
- 320 light create form with `#12`: red border `rgb(180,35,24)`, the six-digit error, the neutral glyph, and Create disabled.

**Create and Save validity**
- Clean Save tag is disabled. A keyboard-selected Red working copy enables it: opacity goes from 0.55 to 1, and the fill becomes solid `#23559e` with white text.
- In dark mode, the disabled primary composites to roughly 3.3:1 label contrast versus about 7.5:1 enabled. That is a clearly perceptible difference, and disabled controls are WCAG-exempt.

**Round-34 keyboard/409 finding**
- This is resolved in source: `web/gallery/tags.js:280` applies the same known-rejected-name guard before `readDetails('edit')` as Create does at `:168`.
- The Enter handler (`:318`) routes to that guard, and an unchanged copy returns early (`:282`).
- I did not press Enter in any Name or hex field, and I did not reproduce a 409. Live persistence and rejection outcomes are the parent's disposable-fixture evidence, which I did not view or certify.

**Footer and confirmation**
- I viewed Delete tag… → "Delete tag?" at 390 dark. It shows "Delete **Health** from your library? It will be removed from every artifact. Your artifacts will be kept.", with focus on **Keep tag** and a destructive coral fill on Delete tag.
- Keep tag returned to the Edit view with focus on Delete tag… and the name intact.
- I never pressed the final Delete.

**Scope copy**
- The panel says "Adding or removing a tag saves immediately."
- The sheet says "Changes apply to this tag on every artifact in your library."
- Run Log correctly shows "All existing tags are already on this artifact." and no impossible Add.

## Keyboard and working-copy observations (live)

**Opening and dismissing the sheet**
- The Edit sheet opens with focus on Name.
- Escape closes it and restores focus to the originating pencil ("Edit tag Reading" or "Edit tag Health"), which I observed at 1280, 768, 390 and 320.
- Reopening shows the stored values, so working copies are discarded.

**Palette keyboard behavior**
- At 390 light on Reading (custom), Tab from Name reached the radio group's single tab stop, and ArrowRight then selected Red.
- After that one press, exactly one radio was checked and one was a tab stop.
- The hex and native pickers both read `#b86562`, the caption read "Red", and Save tag was enabled.

**Restoring the probes**
- The blank-Name/partial-hex probe was restored by typing `Reading` and `#8a5db0`. That returned the Custom caption, native `#8a5db0`, no alerts, and Save tag disabled.
- The create-form `#12` probe was restored to `#6b7280`, which reselected the Gray radio and cleared `aria-invalid`.

## Supplementary vs live limits

- **Not exercised live:** valid Save, Create, Add, Remove or Delete; pending, success, 409 or 503 states; the htmx refresh after a mutation; attach-error placement; the native color dialog.
- **Not rendered:** the supplementary 320×640 and 640×360 sheets.
- **Parent images:** I did not view the parent's outcome screenshots and do not certify them.
- These scores are an opinion of the Chromium renders I inspected. They are not a Safari, physical-phone or screen-reader certification, and not a joint completion claim.

## Restoration and cleanup

- **Writes:** none. I made no valid Save, Create, Add, Remove or Delete, and never pressed the final Delete. I made no Enter presses in any Name or hex field, and no app, build, test, fixture or commit changes. Resource timing showed **0** `/api/tags` requests in both tabs.
- **MAIN final state:** after a fresh reload, MAIN lists Health `#377a64`, Reading `#8a5db0`, Test `#8470ac` and Utilities `#23559e`. The create Name is blank, the hex is `#6b7280`, Create is disabled, no dialog is open, and the status reads "Saved".
- **Sample final state:** the placeholder is selected and Add is disabled. Health, Neighborhood walks and favorite routes, and Reading are still attached. The create Name is blank, the hex is `#6b7280`, and no dialog is open.
- **Tabs:** I reset the appearance of both of my tabs to `system` and closed **only** my two tabs. The scoped diff and porcelain remained empty at cleanup.
- **Files:** this report is the only file I wrote.
