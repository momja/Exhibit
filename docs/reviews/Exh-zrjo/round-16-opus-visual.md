# Round 16: Opus visual review (fresh, post-reset)

Reviewer: **claude-opus-5-5**, working independently. This replaces the earlier
no-score provider failure recorded in `round-16-opus-unavailable.md`.

- **Not read:** `round-16-sol-visual.md`, `visual-scores.json`, `visual-summary.md`,
  `visual-resume.md` and the ticket.
- **Read as prior context only:** the round-15 Opus report. Its scores were not inherited.
- **Application:** `01109602`. `git diff 0110960247e5 HEAD -- internal web` is empty.
- **Served asset version:** `150ec936001f`, confirmed from the live script URLs.

**Method**

- I used only my own T3 tab, `tab_y_0d23e38c-…`, opened with `reuseExistingTab:false`. I passed its id on every call and closed it afterwards.
- I set the scheme explicitly with `preview_set_appearance`. Each measurement read `matchMedia('(prefers-color-scheme: dark)').matches` alongside the data, giving `false` in light and `true` in dark. I did not use root-class simulation.
- I tested at 1280×800, 768×1024, 390×844 and 320×740.
- Geometry comes from live `getBoundingClientRect` and `getClientRects().length`.
- Paint order comes from `elementsFromPoint`.
- I switched spaces with real clicks on the rail and tab links (htmx swaps).
- I saved, created and deleted nothing. Find and replace text was typed into the panel and then closed. Share and Duplicate were opened and dismissed with Escape.
- Afterwards the library still listed the same 5 artifacts, the editor showed "All changes saved", and the Paste draft was empty (I switched back to Agent).

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition and spacing | 8 | 8 |
| Color harmony | 8 | 8 |
| Contrast and emphasis | 8 | 8 |
| Component visual consistency | 8 | 8 |
| Responsive visual polish | 8 | 8 |

**Verdict: PASS for Opus.** All twelve scores are 8 or higher.

This is a single-model result, and it makes no claim that the council is complete. Whether round 16 is a second consecutive joint pass depends on Sol's independent report, which I have not read.

I found no new material defect. The new observations below are minor.

## Key focus: space navigation (live)

**Desktop rail.** The rail is 240px wide and each link box is 215px at x=12. All three rows stay **44px high at y=76/124/172** in every state below.

| Viewport | Scheme | Active space | Active label | Count |
|---|---|---|---|---|
| 1280 | light | Library → Shared with me → Shared by me (clicks) | One line. "Shared with me" at weight 600 spans x50–174 | x211–219, about 37px clear |
| 1280 | dark | Shared by me → Shared with me | One line | Separate |
| 768 | dark | Library → Shared by me → Shared with me (scripted clicks) | One line | Separate |

- Horizontal overflow (`scrollWidth − innerWidth`) was 0 in every state.
- In dark the active pill is `rgb(28,41,64)` with `rgb(126,170,240)` text on a `rgb(22,24,28)` body.
- In light it is a pale blue pill with accent text.

**768 composition**

- `main` is x240, 528px wide, with two 232px card columns. Library cards are 196px high; incoming cards are 221px because of the owner row.
- At 768 the header holds Search at 340px, then Accounts, Add artifact and Settings, with no collision.
- In the 768 dark screenshot the rail (31% of the width) reads as calm. The cards are comfortable, and titles such as "Marathon Training Plan" wrap naturally.

**Phone tabs, 320 light**

- Library, Search, Shared with me and Shared by me are each 70×62px, at x=8/86/164/242, y=674.
- The active tab gets a fill of `rgb(238,244,255)` and a bold accent label. I cycled /shared → /shared-by-me → / with real link clicks, and the tab geometry never changed. Overflow was 0.
- The two-line labels "Shared with / me" and "Shared by / me" are balanced.
- At 390 on `/search?q=not-in-this-library`, the Search tab is active. One observation: the Shared with me tab showed a grey hover fill left over from a pointer position. That is desktop-pointer residue, not a touch issue.

## Earlier source repairs re-verified (live, Run Log `/edit`)

**Selected word on the active row, 1280 light**

- I double-clicked `meta` on line 4, so `getSelection()` returned `"meta"`.
- The paint stack at the word, front to back:
  1. text
  2. `.cm-activeLine` at `rgba(240,243,248,0.25)`
  3. `.cm-content`
  4. `.cm-selectionBackground` at `rgb(219,231,251)`, 31px wide over the word
- The line-5 echo is `rgba(35,85,158,0.08)` with no outline.
- The selected word clearly leads. The secondary echo is very faint; that is the accepted tradeoff, still minor.

**Find and replace, 1280**

- The query `meta` came from the selection; I set the replacement to `link` and pressed Enter.
- The current match on line 5 has the tint plus the accent outline.
- Controls are 32px: field 548px, then Next as the filled primary, Previous, All and a 32px close. The option labels are 32px rows, and Replace and Replace all are aligned.

**Live scheme switch to dark**

- The query and the replacement both survived.
- The full One Dark–style palette renders: tags red, attributes orange, strings green, doctype muted.
- The panel uses the dark field surfaces, and Next becomes a light-blue fill.

**Select All, dark**

- After Ctrl+A, the paint stack at the non-empty head row (`<!doctype html>`) is:
  1. text `rgb(164,174,189)`
  2. `.cm-line`
  3. `.cm-content`
  4. `.cm-selectionBackground` `rgb(39,54,74)`
- So the selection is visibly painted under the first row, with legible text over it.
- I collapsed the selection with ArrowLeft.

**Phone, 390 dark**

- The editor is 338×400 with no page overflow.
- The title field is 44px.
- The collapsible headings (Tags, Network & permissions, Stored state, Stored assets, Artifact source) share one bold weight, and their divider rhythm is consistent.

## Other surfaces revisited (live)

- **Library, 1280 light; 768 dark**
  - Four 236px columns at 1280; two 232px columns at 768.
  - The widget tiles and the restrained "UC" default tile sit well in both schemes. Dark default tiles are a muted olive-green and not loud.
- **Shared with me, 1280 light; 320 light**
  - The owner attribution row ("C council-guest") is aligned to the title's left edge.
- **Shared by me, 1280 dark**
  - The Run Log card shows the tag dot, link glyph and menu.
- **Search `?q=packing`, 1280 dark**
  - The contextual search field is 640px with an integrated 24px clear button, and the card carries the owner chip.
- **Search miss, 390 light**
  - The dashed empty state has a tinted icon tile, a bold headline, muted help text and an outlined Clear search button.
- **Viewer → Share, 390 light**
  - Link access checkbox, then a URL field with Copy, Replace link with "The current link stops working.", the Embed tile disclosure, and People with a focused field and comma helper.
  - The council-guest row has Revoke, and the selected data-mode card has an accent border and tint.
  - It is coherent and on palette.
- **Settings, 1280 dark**
  - Profile, Administration, Storage, Sign out and Delete account cards.
  - Delete account uses a red-tinted border and heading, and its disabled button carries an explanatory note. The danger emphasis is appropriate.
- **`/new`, 320 light (Agent) and 320 dark (Paste)**
  - The three equal 95×46 method buttons, the field card and the 44px full-width Start building and Cancel buttons are clean.
- **Card menu → Duplicate, 1280 light**
  - A modal over a dimmed page, with a focused title field, two checkbox options with helper text, and Cancel plus a primary Duplicate artifact button.

## Minor observations (not material, honestly reassessed)

**New this round**

- **Card footer kebab sits low.** The kebab glyph is centred about 8–9px below the first title line. In the 1280 and 768 cards the title starts at y=307 and the tag dot at 313, but the ⋮ glyph spans 316–334. It is subtle, and it is consistent on every card.
- **Duplicate modal uses different form idioms**
  - Its "Title" label is small sentence-case at regular weight. `/new` uses uppercase micro-labels ("TITLE optional"), and the editor uses "Artifact title".
  - Its checkboxes are native 13px, which is smaller than the Share checkbox.
  - Coherent within the modal, but it is the least token-aligned form I saw.
- **Search miss at 390.** The help text leaves an orphan "tag." on its own line. The Clear search button is 36px tall under a 44px field.

**Carried, still present (re-observed live this round)**

- **Share at 390**
  - The Link access explanation is 6 lines.
  - Replace link is visibly shorter than the URL field and Copy.
  - Revoke ends about 15px short of the section's right edge.
- **Full-width "Return to Library" at 390 is left-aligned** (`text-align:start`, 338×44). The 104px outlined Delete sits alone below it.
- **Light Agent selection on `/new`** is only a 1px accent border. Dark Paste gets an extra ring.
- **Placeholders are italic** in the Title fields ("Session timer", "Untitled") and the People field. The Description placeholder is upright.
- **Paste empty prompt** is clipped to "Paste HTML artifact source her…" at 320.
- **Settings:** the bold `admin` value against the regular Storage values.
- **Native text selection in inputs** (the find field, the Duplicate title) uses the browser's saturated blue, which is less integrated than the editor's selection paint.
- **The light secondary find echo and the 25% active band are very faint.** This is the accepted hierarchy tradeoff.
- **The 240px rail takes 31% of the width at 768.** A fluid track would be an opportunity, not a defect.

**Carried, not re-measured this round** (and not scored upward):

- Edit tag at 320: Save wraps beneath Delete and Cancel, and the solid Delete differs from the editor's outlined one.
- The existing-tag select truncation at 320.
- Completion rows at phone width.
- At 390 the find panel has a gap before Close and its option labels wrap.
- Go-to-line close versus find close.
- Folding, bracket and special-character variants.

None of these, alone or together, amounts to a substantial visual issue in any category. The weakest surfaces I saw are the Share panel at phone width and the Duplicate modal's form idioms. Both are dense or slightly off-pattern, but both are ordered, legible and on palette, so none of them is broken.

## Coverage and limits

**Live**

- Space switching at 1280 (light and dark), 768 (dark) and 320 (light), the last with real tab clicks.
- Library, Shared with me, Shared by me and Search (results and miss).
- Editor:
  - word selection and its paint stack
  - echo
  - find and replace, with retention across a light→dark switch
  - Select All on the head row
  - the phone layout at 390
- Viewer Share at 390 light.
- Settings at 1280 dark.
- `/new` Agent (320 light) and Paste (320 dark).
- Card menu → Duplicate at 1280 light.

**Not covered live this round**

- 768 in light. Round 15 measured 768 light directly. The 768 geometry I measured in dark is scheme-independent.
- 390 in dark for the gallery tabs.
- Native line navigation, completions and folding interactions.
- Edit tag dialog, history View and Restore, and tag creation.
- Admin users page.

**Limits**

- One Chromium engine, with emulated scheme and viewport. No physical phone or alternate engine.
- Arbitrary long data and every completion or bracket variant were not covered exhaustively.
- I wrote no captures. This report is the only file I wrote.
