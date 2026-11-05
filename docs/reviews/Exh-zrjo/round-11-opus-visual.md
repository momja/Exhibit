# Round 11: Opus visual review

Reviewer: **claude-opus-5-5**, working independently. I did not read Sol's round-11 report or any aggregate score, summary or resume file.
Candidate: application `7e96d783` on `origin/main` `44def51a`. The served asset version was `d2086263fb6b`.
Method: I used my own T3 tab, `tab_m_0d23e38c-d413-4e76-b0eb-59687a206343`, opened with `reuseExistingTab:false`.
I set the scheme with `preview_set_appearance` and checked `matchMedia('(prefers-color-scheme: dark)').matches`
(false in light, true in dark) at 1280, 390 and 320 wherever I took a measurement. I did not use class-based emulation.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition and spacing | 8 | 8 |
| Color harmony | 8 | 8 |
| Contrast and emphasis | **7.5** | **7.5** |
| Component visual consistency | **7.5** | **7.5** |
| Responsive visual polish | 8 | 8 |

**Verdict: FAIL for Opus.** Two categories are below 8 in each theme. The round-10 selection fix works.
The failure comes from a different selection defect on the same component, which I observed live below.

## Material finding: the selection is invisible on the line that holds the cursor (observed live, both schemes, both editors)

- **Routes:** `/new` → Paste HTML → `HTML source`, and `/artifacts/461183aa-…/edit` → Artifact source.
  Seen at 1280 and 320 in light and dark. The cause is width-independent.
- **Reproduction:**
  1. Double-click a word in the source, for example `function` in Paste or `meta` in the existing Run Log source.
  2. `getSelection()` returns the word, and CodeMirror draws `.cm-selectionBackground` at the word's rect.
  3. **Nothing is visible on screen.** Saved light Paste capture: `/home/maxomdal/.t3/userdata/browser-artifacts/browser-screenshot-t3code-mw1xyoim-94f331f5.png`.
     Line 4 shows only the pale active-line band, and "function" looks unselected.
  4. With Control+A, every line is highlighted except the last one, which holds the selection head.
     Capture: `…-mw1xyey8-ba3ac554.png`. In the 320 dark Paste view, line 4 is likewise grey rather than selection blue.
- **Painted stack at the selection's centre** (`document.elementsFromPoint`), top to bottom:
  - light: `.cm-line.cm-activeLine` **`rgb(240,243,248)` opaque** → `.cm-content` → `.cm-selectionBackground` `rgb(219,231,251)`
  - dark: `.cm-activeLine` **`rgb(35,43,54)` opaque** → … → `.cm-selectionBackground` `rgb(39,54,74)`

  The selection layer sits behind the content (`z-index:-2`), so an opaque active-line fill covers it.
  CodeMirror's native `::selection` is transparent, so nothing else shows the selection.
- **Cause (source):** `web/editor/editor.js:37` sets `.cm-activeLine` to `var(--color-code-active)`.
  The tokens in `tokens.css:65` (`#f0f3f8`) and `:200` (`#232b36`) are opaque. The rebased baseline `a65a7cfe` had no
  activeLine override, so it used CodeMirror's translucent stock fills (`#cceeff44` and One Dark `#6699ff0b`), which
  let the selection show through. **This branch introduced the regression.**
- **Why it is material:** the source editor is a named in-scope component. Selecting a word or part of one line is the
  most common selection, and in that case there is **no visible feedback at all**. In multi-line selections, the
  head line looks unselected. This is not a marginal contrast ratio: the emphasis is absent.
- **Why tests and captures miss it:** the round-11 check uses Control+A and samples token contrast "against the field,
  active line and selection" as separate backgrounds. It never asks which layer is on top where text is both selected
  and on the active line. The six supplied selected-source captures select from the top of long documents, so the head
  line is below the fold. That explains the gap; I did not rerun the tests.
- **Suggested fix, not prescribed:** make the active line translucent (an rgba version of each token, or `color-mix(… transparent)`).
  Alternatively, drop the active-line fill while the selection is non-empty, or paint the selection above the line
  background. Then double-click a word on the cursor line in both schemes and check the composited colour under the
  word. The contrast floors below should still hold.

## Prior fix verified: focused selection fill (observed live)

These measurements use the computed `.cm-selectionBackground` after a real focus and Control+A, against every distinct
`.cm-content span/.cm-line` colour, on lines that are not the active line.

| Editor | Scheme | Widths | Actual fill | Lowest selected-text contrast | Lowest field contrast |
|---|---|---|---|---|---|
| Existing Run Log | light | 1280 / 390 / 320 | `rgb(219,231,251)` | 4.52 (`#5e6877` doctype/meta), then 4.73 (`#06734a`) | 5.64 |
| Existing Run Log | dark | 1280 / 390 / 320 | `rgb(39,54,74)` | 4.70 (`#ed8088`), then 4.82 (`#cf87e5`) | 6.48 |
| Paste specimen (HTML/CSS/JS) | light | 1280 | `rgb(219,231,251)` | 4.52 | 5.64 |
| Paste specimen | dark | 320 | `rgb(39,54,74)` | 4.70 | 6.48 |

The stock `#d7d4f0` and `#3e4451` fills are gone. Highlighting stays complete: 8 colours in light and 9 in dark on
the existing source, and 8 and 10 on the specimen. Switching light → dark → light kept the selection and buffer.
4.52 clears 4.5:1 only narrowly, but it clears. The round-9 light syntax regression stays fixed.

## Other fresh surfaces (observed)

- **Library, 1280 light:** even card grid, reserved title areas, quiet borders, restrained default tile.
  The card menu at 390 light is well layered, and its icon/label rhythm matches the shell. Dismissed with Escape.
- **Library search miss, 1280 dark:** the empty state is centred and calm. The accent icon chip and outlined
  "Clear search and filters" give the right emphasis, and the Search clear sits inside the field.
- **Global Search `q=packing`, 320 dark:** the integrated 44px clear, readable owner chip and full-width result card are good.
  The fixed tabs' labels wrap symmetrically, and the active tab is tinted clearly.
- **Add, 320 dark (Agent / Paste) and 1280 light (Paste):** the segmented method control, card form and stacked 44px
  footer are coherent. The italic Title placeholder next to the regular Description placeholder remains.
- **Editor, 1280 light and dark, and 320 dark:** section summaries and dividers read clearly. The recessed editor surface is
  integrated. At 320 the footer shows full-width "Return to Library" above a compact 104px "Delete". That is deliberate
  de-emphasis of the destructive action, though slightly unbalanced. Horizontal overflow measured 0.
- **Share, 390 dark:** dialog overflow is 0. URL and Copy are both 44. The helper sits on its own line below Replace link.
  People has a complete placeholder and comma helper. The data choices are stacked full-width cards.

## Reassessed minor observations (not material)

- **Share access explainer:** it is now 6 lines at 390, with connected-accounts copy. It is dense but has a readable
  measure. Refinement only.
- **Replace link / Revoke at 32px** against 44px URL, Copy and Done: still true (measured). Revoke's right edge is at 336
  while Copy ends at 351, a 15px inset. These are small alignment and density refinements.
- **Bold `admin` vs regular Storage values; italic vs normal placeholders:** small inconsistencies with no hierarchy harm.
  I did not freshly reopen Settings this round, so the Settings item rests on prior evidence.
- **Empty source prompt clipped at 320 and the Edit tag footer wrapping Save at 320:** not freshly reopened. I carry them
  as minor, from prior reports and the supplied captures. They do not affect the scores.

## Coverage and limits

- Live: both source editors at 1280, 390 and 320 in both schemes for the selection fill, plus Paste at 1280 light and 320 dark.
  The active-line defect was confirmed at 1280 light and dark, and at 320 dark, by painted-stack inspection and screenshot.
  I also inspected Library, the card menu, the library miss, global Search incoming, Add, and Share at 390 dark.
- Not freshly reviewed: Settings, Versions, the Tags and Edit-tag modal, Duplicate, `/shared` and `/shared-by-me`,
  viewer popovers, desktop Share, and 390 light Share. For those I relied on round 8–10 evidence and the round-11 captures,
  without rescoring them upward.
- Resizing keeps a desktop user agent and fine pointer. I tested no real touch devices, software keyboards or other engines.
- The cause and baseline comparison come from reading source. The defect itself was observed live.
- Data safety: nothing was saved, created, deleted, granted, replaced or mode-changed. The Paste specimen was
  cleared (select all, then Backspace). The existing source was only selected, and its status stayed "All changes saved".
  The Share dialog and card menu were dismissed with Escape. I made no source edits, builds, test runs, commits or
  capture overwrites. The only file I wrote is this report.
- This report does not declare council completion. The qualifying joint streak stays at zero on my side.
