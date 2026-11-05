# Round 37: independent Opus review of tag editing (target 10)

Reviewer: Claude Opus 5.5, a fresh delegated task. There was no subdelegation.

**Read:** the original `tags-ten-brief.md`, both round-36 reports (Sol and Opus), the cumulative round-37 brief, `tags-ten-round-36-changes.md` and `tags-ten-runtime-identity.json`.

**Not read:** any current round-37 peer report, status or launch file; any parent scores, summary or resume aggregate; the ticket; or any timer prompt.

**Scope:** only the editor's **Tags** section and its **Edit tag** sheet. The prior round-36 scores were treated as context, not as a starting point.

## Candidate and scope proof

- The application is `f58de10bf88acff268b9339e0c7dae406a93c664`, with assets `4beedb277545`.
- HEAD was `b5c0e54d` at entry and `227f1335` at cleanup. Both are later evidence-only commits.
- `git diff --exit-code f58de10b -- internal web e2e cmd scripts Makefile go.mod go.sum` returned 0 at entry and at cleanup.
- Porcelain over the same paths was empty at both points. No paths were excluded.
- I read the SHA-256 of `bin/server`, `/proc/305222/exe` (MAIN, port 18280) and `/proc/305238/exe` (sample, port 18580) myself, at 22:02 and at 22:06 UTC.
  - All three were `7be17fc2…5d7458` both times.
  - That matches `tags-ten-runtime-identity.json`.
- Both live pages loaded only `/assets/v/4beedb277545/…`. I checked this in the DOM on both, and in the sample's network log.
- The binary has no VCS stamp. Provenance therefore rests on four things:
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

All 14 values are 10. **Blockers to 10: none found in the inspected scope.**

This is one reviewer's opinion and is not a joint result. Whether the consecutive-round requirement is met is for the parent to judge against both round-37 reports.

## Actual viewed matrix

For every pair I did the following, all in my own tab:
- called `preview_resize` (freeform) and `preview_set_appearance`;
- evaluated `innerWidth`/`innerHeight`, **both** `prefers-color-scheme` `matchMedia` queries, and horizontal overflow;
- returned every listed view with `includeImage:true` and looked at it.

No image from another scheme or instance, no parent capture and no test fills any cell.

### MAIN

Default profile, tab `tab_35_0d23e38c…`, the Run Log artifact. Its tags are Health, Reading, Test and Utilities, all attached.

| Viewport | Scheme | mM light / dark | Tags panel viewed (lower controls through Create) | Edit tag sheet viewed |
|---|---|---|---|---|
| 1280×800 | Light | true / false | One view of list → Create (Create 706–750, fully exposed) | Reading (custom), full body and footer, dialog (400,55)–(880,745) |
| 1280×800 | Dark | false / true | Top view (Create clipped by 5px), then a lower view with Create at 744–788 | Health (Green preset), full body and footer |
| 768×900 | Light | true / false | Whole panel in one view, Create 761–805 | Reading (custom), full |
| 768×900 | Dark | false / true | Whole panel in one view, Create 761–805 | Test (Violet preset), full |
| 390×844 | Light | true / false | Top view, then a lower view with Create at 788–832 | Health, full bottom sheet (0,138)–(390,844); body 563/563 |
| 390×844 | Dark | false / true | Top view, then a lower view with Create at 788–832 | Reading with a local **Red** working copy: Save tag enabled; then Cancel |
| 320×740 | Light | true / false | Top view, then a lower view with Create at 684–728 | Reading: partial-hex `#37` probe → restored → Delete tag… → Keep tag → clean sheet |
| 320×740 | Dark | false / true | Top view, then a lower view with Create at 684–728 | Utilities (custom `#23559e`), full sheet (0,34)–(320,740) |

That covers all 16 core states. As the brief required, the lower controls for 1280 light and 390 dark were viewed in their own scheme. Horizontal overflow was 0 in every pair.

### Isolated sample

Incognito profile, tab `tab_37_0d23e38c…`, logged in only in that profile. The artifact is Tag editing example, with Health, "Neighborhood walks and favorite routes" and Reading attached; Utilities is available.

| Viewport | Scheme | mM light / dark | Viewed |
|---|---|---|---|
| 1280×800 | Light | true / false | Long label wraps to two lines; Utilities selected with Add enabled (481–554); placeholder restored with Add disabled; lower view through Create (744–788) |
| 1280×800 | Dark | false / true | Same; Add 481–554; lower view through Create |
| 320×740 | Light | true / false | Same; select 26–213, Add 221–294; lower view through the full-width Create (684–728) |
| 320×740 | Dark | false / true | Same; Add 221–294; lower view through Create |

At every sample pair:
- I selected Utilities, confirmed Add was enabled, and restored "Choose a tag…" **without pressing Add**.
- The Add row ended on the shared column edge: 554 on desktop, 294 on phone.
- Horizontal overflow was 0.

## Prior issues, freshly assessed

- **Check against the tile corner (original):** fixed. The 16px check sits inset inside the rounded tile with no corner fringe at 74, 61, 102, 81 and 64px tile widths in both schemes. It clears the sample dot, including the 61px tiles at 320.
- **Stretched pill (original):** fixed.
  - Capsules fit their text.
  - The long name wraps inside its capsule, which ends 8px before the 32px (desktop) or 44px (phone) pencil and × slots.
  - Row dividers tie each label to its own actions.
- **Ambiguous mobile Save (original):** fixed. The bottom-sheet footer reads Cancel and **Save tag**. Save tag is muted until a valid change is made; with the local Red working copy it turned solid.
- **Add overhang (R35):** fixed in the live sample at both widths and in both schemes.
- **Scope and immediacy wording:** present and correct.
  - The panel says "Adding or removing a tag saves immediately."
  - The sheet says "Changes apply to this tag on every artifact in your library."
  - MAIN has every tag attached, so it shows "All existing tags are already on this artifact." and no impossible Add.
- **Custom versus preset versus focus:** kept distinct.
  - Reading and Utilities show the Custom chip with no checked preset.
  - Presets show their named chip and a check badge.
  - The focus ring is visually separate from the selection mark.
- **Invalid hex (`#37`) at 320 light:**
  - "Enter a six-digit color, like #377a64." replaced the hint in the same slot.
  - The field had a red border, the sample showed a neutral minus, the chip was hidden, and Save tag stayed disabled.
  - Restoring `#8a5db0` cleared `aria-invalid` and put Save tag back to its disabled, clean state.
- **Delete confirmation:**
  - It is a same-sheet confirmation naming **Reading**, saying "removed from every artifact" and "Your artifacts will be kept".
  - Keep tag is focused first.
  - Keep returned to Edit tag with the name and hex intact and focus on Delete tag….
  - **Final Delete was never pressed.**
- **Enter-after-409 guard (R34):** assessed by source and parent evidence only. I pressed Enter nowhere, per the brief.

## Limits

- I exercised no valid Save, Create, Add, Remove or Delete.
- I saw no live pending, success, 409 or 503 outcome, and no global rename.
- These were not covered: the native colour dialog, the optional 320×640 and 640×360 sheets, Safari, physical phones and screen readers.
- Escape and Cancel dismissal were observed. Cancel returned focus to the pencil that opened the sheet.
- Pointer-hover fills left on a pencil after an earlier click were discarded as interaction artifacts. I used script `.click()` on later opens to avoid them.

## Restoration and cleanup

- **MAIN:**
  - The Red working copy was cancelled.
  - The `#37` probe was restored to `#8a5db0`.
  - The Delete confirmation was dismissed with Keep, and the sheet then cancelled.
  - Final API state: Health `#377a64`, Reading `#8a5db0`, Test `#8470ac`, Utilities `#23559e`.
  - The create form is blank with `#6b7280`, no dialog is open, and the header reads "Saved".
  - The tab made **0** tag API requests.
- **Sample:**
  - The select was set back to its placeholder at every pair.
  - Final API state: Health `#377a64`, "Neighborhood walks and favorite routes" `#527cb0`, Reading `#8a5db0`.
  - The tab made **0** tag API requests.
- Both tabs were reset to the system appearance. **Only my two tabs** (`tab_35_…`, `tab_37_…`) were closed, by 22:06 UTC on 2026-11-05.
- At cleanup the scoped diff and porcelain were empty and the process hashes were unchanged.
- This report is the only file I wrote.
