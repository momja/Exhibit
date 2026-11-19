# Round 9: Opus visual review

Reviewer: **claude-opus-5-5**, working independently. I did not read Sol's round-9 report.
Candidate: `4d725ba252d637e64c6c3c648bffe0d83fc27bd1`, on top of `origin/main` `44def51a`, with no app edits.
Method: I used my own T3 preview tab, emulated the theme with `preview_set_appearance`, and confirmed
`matchMedia('(prefers-color-scheme: dark)').matches` on every route listed below. I edited nothing and saved nothing.
I opened the share dialog and the card menu, then dismissed both.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition and spacing | 8 | 8 |
| Color harmony | 8 | 8 |
| Contrast and emphasis | **7.5** | 8 |
| Component visual consistency | **7** | 8 |
| Responsive visual polish | 8 | 8 |

**Verdict: FAIL for Opus.** Two light scores are below 8. The round-8 pass does not carry forward.

## Material finding: light source editor has lost all syntax highlighting (observed, and confirmed in source)

- **Where:** `/artifacts/461183aa-…/edit`, Artifact source, **light** theme, 1280. The editor is the same at every width.
- **Observed:** in light, I counted the colours of every `.cm-content span`. Only one span has a token colour,
  `rgb(94,104,119)` on the doctype. Tags, attribute names, strings, CSS properties and values all render in
  plain ink. In **dark**, the same buffer uses 8 distinct One Dark token colours across about 150 spans.
- **Baseline:** `docs/screenshots/Exh-zrjo/rebased-before/editor-1280-light.png` shows proper light highlighting:
  green tags, attribute names, red strings, and coloured CSS values. **This is a regression this branch introduced.**
- **Cause (from source, `web/editor/editor.js`, commit 7eaf8354):** `themeFor()` now always adds
  `Prec.highest(syntaxHighlighting(HighlightStyle.define([...comment/meta only...])))`. In CodeMirror, a
  non-fallback `syntaxHighlighting` turns off the fallback `defaultHighlightStyle` that `basicSetup` registers.
  Light mode keeps only the comment/meta rule. Dark is unaffected because `oneDark` brings its own non-fallback
  highlighter.
- **Why it counts:** the source editor is a named in-scope component. Its two themes no longer match, and the
  light editor is a flat grey-black wall where the baseline was legible and structured. Round 8 called this
  a "subdued light syntax distinction", which understates it: the highlighting is **gone**, not subdued. I
  score light Component consistency 7 and light Contrast/emphasis 7.5 for this reason.
- **Suggested fix:** apply `subduedSyntax` with `{fallback: true}` alongside `defaultHighlightStyle`, or add
  `syntaxHighlighting(defaultHighlightStyle)` to the light branch beneath the subdued override. Then check the
  token colours for ≥4.5:1 contrast on `--color-field` and the active line.

## Prior fixes re-verified (observed)

| Item | Status | Evidence |
|---|---|---|
| Replace-link helper | Holds | 390 light: Replace link spans x 39–163, y 355–387, and "The current link stops working." sits on its own line beneath it. |
| Copy/URL field height | Holds | 390 light: Copy is 44px, and the URL ends in an ellipsis. Dialog `scrollWidth−clientWidth = 0`. |
| Stacked full-width data choices | Holds | 390 light: the selected "Separate data per person" card is full width with an accent border. |
| Integrated Search clear, 320 dark | Holds | Clear is 44×44 inside the 296px field, page overflow is 0, and the tab bar is fixed with the active tab clearly tinted. |

## Fresh surfaces (observed)

- **/settings, 1280 dark:** sections are consistent cards with icon headings. The Delete card has a quiet danger
  border, and the disabled Delete button has its explanation beside it. Minor: the "admin" value is bold while
  the Storage values are regular.
- **/new, 1280 dark:** the three method cards are even, and the selected card has an accent border and a tinted icon
  chip. The form and actions are aligned. Minor: the Title placeholder is italic but the Description placeholder
  is not (computed `::placeholder` fontStyle italic vs normal). The share-sheet People placeholder is italic too.
- **Library card menu, 1280 dark:** the menu is well layered over the tile and its items have clear focus.
  The trigger is 32×36 inside a 32×32 wrapper, which is a subpixel matter only.
- **Search, 320 dark:** the type scale is good, the cards are even and the tab labels wrap cleanly. This is
  good work.
- **Share sheet, 390 light:** the minor round-8 items are unchanged. Revoke ends at x=336 while Copy and the
  section end at 351, a 15px inset. Replace link and Revoke are 32px against 44px fields. The six-line
  link-access explainer still dominates the top. All three are refinements, not defects.

## Limits

- I could not freshly open the Versions list, or check 390/320 light editor, Duplicate, `/search?q=packing`
  or the miss state in this time box. For those I relied on round-8 evidence.
- I did not open the dark share sheet this round.
- The editor cause above is inferred from source. The symptom itself is observed in computed styles and matches the baseline screenshot comparison.
- Passing tests were not treated as visual proof.
