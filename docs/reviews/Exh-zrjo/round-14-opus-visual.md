# Round 14: Opus visual review

Reviewer: **claude-opus-5-5**, working independently.

- **Not read:** Sol's round-14 report, `visual-scores.json`, `visual-summary.md`, `visual-resume.md` and the ticket.
- **Read as prior context:** `visual-brief.md` and both round-13 reports. Their scores were not inherited.
- **Candidate:** application `30bdaa5d`, with HEAD `501bb5b6` (evidence only), on `origin/main` `44def51a`.
- **Served asset version:** `356ca1fa22d1`, confirmed from the served script URLs.

**Method**

- I used only my own T3 tab, `tab_t_0d23e38c-d413-4e76-b0eb-59687a206343`, opened with `reuseExistingTab:false`. I passed its id on every call.
- I set the scheme explicitly with `preview_set_appearance`. Before each measurement, `matchMedia('(prefers-color-scheme: dark)').matches` read `false` in light and `true` in dark. I did not use root-class simulation.
- I tested at 1280×800, 390×844 and 320×740.
- Every finding below comes from a live screenshot together with computed geometry or paint. Layered paint was read from the actual `elementsFromPoint` stack.
- I deliberately put weight on the surfaces I skipped in round 13: card menu, Duplicate, Versions, Tags, Edit tag, desktop Share, viewer menu and popover, shared spaces, Add Agent and Paste, go-to-line in dark, and completion at 390.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition and spacing | **7.5** | **7.5** |
| Color harmony | 8 | 8 |
| Contrast and emphasis | 8 | 8 |
| Component visual consistency | 8 | 8 |
| Responsive visual polish | 8 | 8 |

**Verdict: FAIL for Opus.** Composition and spacing scores 7.5 in both schemes.

- The cause is a defect in the main navigation on an in-scope page, described below. Neither council member has reported it before.
- It is **not a regression from the council work**. It is upstream and already present in the rebased baseline.
- Still, the rubric scores the interface rather than the diff. Measured by the standard I used in round 6, it is a substantial finish defect: there, an orphaned helper line cost 7.5.
- This report does not continue the qualifying streak, and it makes no council-completion claim.

## Material finding: the active "Shared with me" item wraps in the desktop rail

**Observed live** at `/shared`, 1280, in both light and dark. The selector is `.gallery-spaces a[aria-current=page]`.

**What the screenshot shows**

- In the active, current-page state, the label reads "Shared with" on one line and "me" alone on a second line.
- The inactive item on `/` and `/shared-by-me` is a single line.

**Measured cause**

- The active weight is 600. At that weight, "Shared with me" at 14px needs **124.1px**, but its flex item gets **118.6px**.
  - The space is used by a 183px link, 8px padding, a 20px icon, 10px gaps and the count.
- So the active row grows from 44px to **53px**.
- "Shared by me" moves from y=172 on `/` to **y=181** on `/shared`. Switching spaces therefore shifts the rail by 9px.

**Scope**

- The rail has a fixed width at every width of 768px and above, so this happens on every non-phone visit to Shared with me.
- The phone tabs are a different layout, where two-line labels are intended.

**Provenance**

- It is already present in `docs/screenshots/Exh-zrjo/rebased-before/shared-1280-light.png`.
- It is also in the round-13 capture `visual-round-13/shared-1280-dark.png`. All earlier reviews, mine included, missed it.

**Why it counts as material**

- It is the primary navigation, at the primary desktop breakpoint.
- It leaves a one-word orphan.
- It moves content under the pointer when you change destination.

**Possible fixes:**
- Let the label's flex item take its intrinsic width (`flex:1 1 auto; min-width:0` with `white-space:nowrap` on the label).
- Or reserve the bold advance width so weight changes don't reflow, for example with a hidden bold duplicate or `text-shadow` emphasis.
- The fix is small, and the rail elsewhere is clean.

## Previous fixes re-verified (live)

**Repeated-word echoes stay quieter than the selection**

- *Run Log source, 1280 dark.* I double-clicked `meta` on line 4, which was the active line.
  - `getSelection()` returned `meta`, with focus on `.cm-content`.
  - The selection paints `rgb(39,54,74)` under the active line `rgba(35,43,54,.25)`.
  - The line-5 echo is `rgba(126,170,240,.08)` with outline `none`, over the field `rgb(26,29,33)`.
  - In the screenshot the selection is the clearly stronger cue.
- *Paste specimen, 320 light.* The specimen was six mixed HTML/CSS/JS lines with no trailing newline. I double-clicked `app` on active line 5.
  - The paint stack is: text, then `.cm-activeLine` at `rgba(240,243,248,.25)`, then `.cm-selectionBackground` at `rgb(219,231,251)`.
  - All four echoes are `rgba(35,85,158,.08)`. In the screenshot, `app` is visibly selected and the echoes are faint grey-blue. There is no lime anywhere.

**Select All with a non-empty head row**

- I pressed Ctrl+A at 320 light, then switched live to dark.
- The selection survived: 107 characters.
- The head row `</script>` is the active line. It keeps a 70px `rgb(39,54,74)` selection rectangle, and the rows above it are continuous.
- Syntax stays fully differentiated in both schemes.

**Find and replace (Run Log source)**

- *1280 dark:* query `meta`, replacement `link`, after Next.
  - The current match has the selection fill plus an accent outline. The other match is faint.
  - The buttons are flat, Next is the primary, and the 32px row grid is intact.
- *390 light, after a live switch:* the query and replacement both survived.
  - Every control is 44px.
  - The field runs 37–246, with close at 309–353.
  - Next, Previous and All are three 101px buttons. The three option labels sit on one row.
  - The replace field is 316px wide. Replace and Replace all are 155px each.
  - The source scroller keeps 240px. Document overflow is 0.

**Go to line, 1280 dark (not covered in round 13)**

- The "Go to line:" label, a 72×32 field, a 32px Go button and a bordered 32px close all use the same 14px system type as the find panel.
- I entered 40. The active gutter moved to 40, and the page stayed "All changes saved".
- Bracket matching on `<head>` shows the quiet accent outline.

**Completion, 390 dark (not covered in round 13)**

- I typed `<` on a new line of the Paste specimen.
- The panel is `rgb(32,35,40)`, rows are 44px, and the selected row is `rgb(28,41,64)`.
- The popup spans x 71–324 inside the 390 viewport. Overflow is 0.

## Other surfaces revisited (live)

- **Library, 1280 dark:** the rail, cards, sharing glyph and default tiles are coherent.
  - Run Log's card menu has Edit and Duplicate, with even icon and label rhythm, on a panel with a hairline border.
- **Duplicate, 1280 dark:** the hierarchy is clear: title field, two checkbox options with muted help, and Cancel beside a filled Duplicate. I cancelled with Escape.
- **Editor, 320 dark:**
  - Versions has a clear v-number, title, timestamp and badge grid, with 44px View and Restore.
  - Tags shows the chip, the select with Add, and the create form.
  - Edit tag reproduces the known footer (below). I dismissed it.
- **Edit tag, 1280 light:** Delete on the left, Cancel and Save on the right. This is a conventional split and reads well.
- **Share, 1280 light, scrolled through:**
  - The URL and Copy row is aligned, and the Replace link helper is inline.
  - People has its full placeholder and comma helper.
  - The selected data-mode card is clearly tinted, and Done is bottom-right.
  - I changed nothing.
- **Viewer, 1280 dark:** the More menu and the posture popover use dark chrome over the light Run Log body. They are readable, with consistent menu rhythm. The body itself is user content.
- **`/shared` 1280 light and `/shared-by-me` 390 dark:** cards, owner attribution and the fixed tabs are coherent, apart from the rail wrap above.
- **Search `q=not-in-this-library`, 320 dark:** the field has an integrated clear. The empty state has a tinted icon tile, a bold heading, a muted hint and an outlined Clear search. Tabs are evenly wrapped.
- **Add, 320 light and dark, and 390 dark:** Agent and Paste. The method switch, the uppercase labels with muted "optional", and a full-width primary with Cancel are all consistent.
- **Settings, 1280 light:** the card headings and icons are consistent, and the uppercase fact labels align with their values.

## Minor observations (not material on their own)

**Carried from earlier rounds and reconfirmed**

- **Edit tag, 320 dark:** the footer is now freshly confirmed.
  - The solid coral Delete sits top-left and Cancel top-right, while Save wraps alone beneath Delete.
  - In dark, the coral Delete is at least as loud as the primary.
- **Paste empty prompt, 320 light:** this is now freshly confirmed.
  - "Paste HTML artifact source her…" is clipped, because the scroller's `scrollWidth` is 288 against a `clientWidth` of 266.
  - The placeholder is `white-space:pre`.
- **Completion rows at phone widths:** the label sits 8px from the top and 20px from the bottom of its 44px row, so it is visibly high.
- **Close-button styles:** the go-to-line close is bordered, while the find close is borderless.
- **Share:** the access explainer is dense, and Replace link and Revoke are 32px against the 44px controls. Revoke is inset about 15px from the section edge.
  - This is also visible at desktop: Revoke ends at x=882, while the section edge is at 897.
- **Settings:** `admin` is bold while the Storage values are regular.
- **Italic placeholders:** "Untitled", "Session timer" and the Search field are italic, while Description is not.
- **Find panel at 390:** the gap before Close is still there. In this run the three option labels fit on one row.
- **Faint tints:** the light secondary-match tint and the light active-line band are both very faint. They remain accepted, quiet choices.

**New this round, all minor**

- **Editor footer at phones:** "Return to Library" is a full-width (338px) flex link with `justify-content:normal`, so its label is left-aligned. Every other full-width button, such as Replace and Start building, centres its label.
- **Delete treatments differ:**
  - The editor footer Delete is an outlined danger button.
  - The Edit tag Delete is a solid danger fill.
- **Tags at 320:** the existing-tag select placeholder truncates to "Choose an existing ta". Two filled primaries (Add, Create and add tag) also compete within one section.
- **Add method switch in light:** the selected method (Agent or Paste) differs from the others only by a 1px accent border. In dark it gets an extra glow ring and reads more clearly.
- **Native text selection in Duplicate's title field, dark:** it uses the browser's saturated blue on a dark field. It is readable, but less integrated than the editor's selection.

## Coverage and limits

**Live coverage**

- **Library:** 1280 dark.
- **Card menu and Duplicate:** 1280 dark.
- **Editor at 1280 (both schemes):** selection and echoes, find and replace, go-to-line, and the Edit tag dialog.
- **Editor at 390 light:** find and replace.
- **Editor at 320 dark:** Versions, Tags and Edit tag.
- **Paste specimen:** 320 light and dark (selection, echoes, Select All and scheme switch), and completion at 390 dark.
- **Add:** Agent at 320 light.
- **Viewer:** Share at 1280 light, the More menu and posture popover at 1280 dark.
- **Shared spaces:** `/shared` at 1280 in both schemes (dark measured), `/shared-by-me` at 390 dark.
- **Search miss:** 320 dark.
- **Settings:** 1280 light.

**Not freshly covered**

- Share at phone widths.
- The URL method.
- Search results for `q=run` and `q=packing`.
- Filters and collection overlays.
- Library at 390 and 320.
- Settings at phone widths.
- Folding.
- The completion detail pane and special characters.
- The existing-source find panel at 320.

For these I rely on round 13 and do not score them upward.

**Captures**

- I consulted only `rebased-before/shared-1280-light.png` and `visual-round-13/shared-1280-dark.png`, to establish the provenance of the rail wrap.
- I wrote no captures. Temporary screenshots went to the browser's artifact directory.

**Environment limits**

- Resizing keeps a desktop user agent and a fine pointer, so fields stay at 14px rather than 16px.
- I used no real touch devices, software keyboards, other browser engines or screen readers.

**Data safety**

- I saved, created, deleted, granted, replaced or mode-changed nothing.
- Duplicate, Edit tag, Share and the menus were dismissed with Escape.
- In the Run Log source I only selected, searched, closed find and used go-to-line. The find panel was closed with Escape. It stayed "All changes saved".
- I cleared the Paste specimen: `#body` length 0, with the placeholder showing.
- I ran no builds or tests and made no commits or source edits. I wrote only this report.
