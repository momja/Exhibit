# Round 36: independent Opus review of tag editing (target 10)

Reviewer: Claude Opus 5.5, a fresh delegated task. There was no subdelegation.

**Read:** `tags-ten-brief.md` (the original), both round-35 reports (Sol and Opus), the cumulative round-36 header, `tags-ten-round-36-changes.md`, `tags-ten-runtime-identity.json`, `tags-ten-r36-scope-change.json`, and the one-file `web/gallery/edit.css` delta between `f58de10b~1` and `f58de10b`.

**Not read:** any current round-36 peer report, status or launch file; any parent scores, summary or resume aggregate; the ticket; or any timer prompt.

**Scope:** only the editor's **Tags** section and its **Edit tag** sheet.

## Candidate and scope proof

- The application is `f58de10bf88acff268b9339e0c7dae406a93c664` with assets `4beedb277545`.
- At entry, HEAD was `f58de10b`. At cleanup it was `d62235a0`, a later commit, but the scope check below shows that the application bytes did not change.
- The scope check passed at entry and at cleanup:
  - `git diff --exit-code f58de10b -- internal web e2e cmd scripts Makefile go.mod go.sum` returned 0 (empty).
  - `git status --porcelain` over the same paths printed nothing.
  - No paths were excluded.
- I read the SHA-256 of `bin/server`, `/proc/305222/exe` (MAIN, port 18280) and `/proc/305238/exe` (sample, port 18580) myself, at entry and again at cleanup. All three were `7be17fc2…5d7458` both times, which matches `tags-ten-runtime-identity.json`.
- Both live pages loaded only `/assets/v/4beedb277545/…`. I checked the DOM on MAIN and the sample's network log.
- There is no VCS stamp in the binary. Provenance therefore rests on four things:
  - the empty scoped comparison,
  - the live asset version,
  - the matching executables,
  - the supplied build record.
- I ran no build or test. Test results contribute nothing to these scores.

## Independent scores (0–10, unrounded)

| # | Category | Light | Dark |
|---|---|---:|---:|
| 1 | Typography | 10 | 10 |
| 2 | Composition and spacing | 10 | 10 |
| 3 | Color harmony | 10 | 10 |
| 4 | Contrast and emphasis | 10 | 10 |
| 5 | Component visual consistency | 10 | 10 |
| 6 | Responsive visual polish | 10 | 10 |
| 7 | Tag workflow clarity and feedback | 10 | 10 |

All 14 values are 10. **Blockers to 10: none found in the scope I inspected.**

My only round-35 deduction is fixed in the live result:
- At 1280×800, the "Add existing tag" row now spans x=234–554. The select is at 234–473 and Add at 481–554.
- So Add ends exactly on the 554px right edge that the tag list, the pencil and × column, the palette and the create form all share.
- At 320×740, the row spans 26–294 and Add ends at 294, the same edge as the list and the full-width Create button.

This was the same in both schemes. This is one reviewer's opinion, not a joint pass. It is also only the first of the two consecutive rounds required.

## Actual viewed matrix

For every pair I did the following, all in my own tab:
- called `preview_resize` and `preview_set_appearance`;
- evaluated `innerWidth`/`innerHeight` and **both** `prefers-color-scheme` `matchMedia` queries;
- returned each listed render with `includeImage:true` and viewed it.

No unviewed resize, image borrowed from the other scheme, parent image or test fills any cell.

**MAIN**, default profile, tab `tab_31_…` (Run Log, tags Health, Reading, Test, Utilities):

| Viewport | Scheme | mM light / dark | Tags panel viewed (lower controls through Create fully exposed) | Edit tag sheet viewed |
|---|---|---|---|---|
| 1280×800 | Light | true / false | Whole panel in one view; Create at y=697–741, fully visible | Reading (custom), whole body and footer |
| 1280×800 | Dark | false / true | Whole panel in one view, Create fully visible | Health (Green preset), whole body and footer |
| 768×900 | Light | true / false | Whole panel in one view, Create fully visible | Health, whole body and footer |
| 768×900 | Dark | false / true | Whole panel in one view, Create fully visible | Reading (custom), whole body and footer |
| 390×844 | Light | true / false | Top view (Create clipped), then a scrolled lower view with Create at 784–828 | Reading, whole bottom sheet (body 563/563); also a local Red working copy with Save tag enabled, then Cancel |
| 390×844 | Dark | false / true | Top view, then a scrolled lower view with Create at 784–828 | Health, whole bottom sheet |
| 320×740 | Light | true / false | Top view, a lower view with a partial-hex `#37` probe, then a clean restored lower view with Create at 680–724 | Reading, whole sheet; also Delete tag… → Keep tag confirmation |
| 320×740 | Dark | false / true | Top view, then a scrolled lower view with Create at 680–724 | Test (Violet preset), whole sheet |

That covers all **16 core states**.
- The document's horizontal overflow measured 0 in every pair.
- The dialog rect measured `(400,55,480,691)` at 1280 and `(144,105,480,691)` at 768.
- The sheet measured `(0,138,390,706)` at 390 and `(0,34,320,706)` at 320. The body was 563/563 in both, with header 68 and footer 72.

**Isolated sample**, incognito profile, tab `tab_33_…`, logged in only in this profile (Tag editing example: Health, "Neighborhood walks and favorite routes", Reading; Utilities available):

| Viewport | Scheme | mM light / dark | Viewed |
|---|---|---|---|
| 1280×800 | Light | true / false | Long label wraps to two lines; Utilities selected with Add enabled (481–554); placeholder restored with Add disabled; lower view through Create (740–784) |
| 1280×800 | Dark | false / true | Same; Add 481–554; lower view through Create |
| 320×740 | Light | true / false | Same; Add 221–294; lower view through the full-width Create (680–724) |
| 320×740 | Dark | false / true | Same; Add 221–294; lower view through Create |

At every pair I selected Utilities, checked that Add was enabled, and restored "Choose a tag…" **without pressing Add**. The sample made **0** `/api/tags` or `…/tags` requests.

**Discarded, not scored:** pointer-hover fills left on a pencil or × after clicks (1280 dark, 768 dark, 390 light), and the focus ring returned to a pencil after Escape.

## Earlier issues, freshly assessed

- **Check badge against the corner (original defect):** fixed. The 16px check sits inset inside the 8px-radius tile at every width and in both schemes. It clears the 22px sample, including in the 61–64px cells at 320. There is no corner fringe.
- **Stretched pill (original defect):** fixed.
  - The capsules fit their content.
  - The long name wraps inside its capsule, which ends 8px before the 32px desktop or 44px phone pencil and × column.
  - Quiet row dividers tie each label to its own actions.
- **Ambiguous mobile Save (original defect):** fixed. The bottom sheet's footer reads Cancel and **Save tag**. Save tag is disabled until something changes; it filled solid blue once the Red working copy was a valid change.
- **R32–R35 workflow items:**
  - The panel says "Adding or removing a tag saves immediately."
  - The sheet says "Changes apply to this tag on every artifact in your library."
  - With every tag attached, MAIN replaces the add row with "All existing tags are already on this artifact." The sample shows a labelled select whose Add stays disabled until a tag is chosen.
- **Invalid custom colour:** the create form's partial `#37` replaced the hint with "Enter a six-digit color, like #377a64." in the same slot. The field border turned red, the caption hid, the sample showed a neutral minus, and Create stayed disabled. Typing `#6b7280` restored Gray, the hint and the caption.
- **Custom versus focus:** Reading shows the Custom caption chip, no checked preset and no false outline. The selected preset's check badge is separate from the focus ring.
- **Delete confirmation:** it names Reading, says "removed from every artifact" and "Your artifacts will be kept", and focuses Keep tag first. Keep returned to Edit tag with the name intact and focus on Delete tag…. **Final Delete was never pressed.**
- **R34 Enter-after-409 guard:** checked by source reading only, not live. I did not press Enter in any field, per the brief.

## Limits

I did not exercise any valid Save, Create, Add, Remove or Delete. Nor did I see live pending, success, 409 or 503 outcomes, or a global rename. Those are parent-only evidence and certify nothing here.

Also not covered:
- the native colour dialog;
- the optional 320×640 and 640×360 sheets;
- Safari, physical phones and screen readers.

## Restoration and cleanup

Everything I changed was local and restored without saving:
- The Red working copy was cancelled.
- The create form's hex was put back to `#6b7280`, with Gray selected and Create disabled.
- The Delete confirmation was dismissed with Keep.
- The sample's select was set back to its placeholder at every pair.

Final MAIN state:
- Tags: Health `#377a64`, Reading `#8a5db0`, Test `#8470ac`, Utilities `#23559e`.
- No dialog open; header "Saved".
- **Zero** tag API requests.

Both tabs' appearance was reset to system. **Only my two tabs** (`tab_31_…`, `tab_33_…`) were closed, at about 21:56 UTC on 2026-11-05. At cleanup the scoped diff and porcelain were empty, and the process hashes still matched.
