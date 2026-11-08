# Round 12: Opus visual review

Reviewer: **claude-opus-5-5**, working independently. I did not read Sol's round-12 report, any round-12 scores,
`visual-scores.json`, `visual-summary.md`, `visual-resume.md` or the ticket.
Candidate: application `b1b43d96`, served asset version `ec71042deca4`, on `origin/main` `44def51a`.
Method: I used my own T3 tab, `tab_p_0d23e38c-d413-4e76-b0eb-59687a206343`, opened with `reuseExistingTab:false`.
I set the scheme with `preview_set_appearance` and checked `matchMedia('(prefers-color-scheme: dark)').matches`
(false in light, true in dark) each time I measured. I tested at 1280, 390 and 320. I did not use class-based emulation.
Painted results come from composing the computed backgrounds at the probe point, front to back: field, then
`.cm-selectionBackground`, then `.cm-selectionMatch`, then the `.cm-line` background. I checked each one against a screenshot.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition and spacing | 8 | 8 |
| Color harmony | 8 | 8 |
| Contrast and emphasis | **7.5** | 8 |
| Component visual consistency | **7.5** | **7.5** |
| Responsive visual polish | 8 | 8 |

**Verdict: FAIL for Opus.** Three of the twelve scores are below 8. The round-11 active-line defect is fixed, and I
verified it live. The failures come from two other CodeMirror surfaces that still use library defaults rather than
app tokens. Neither surface appears in the supplied captures or the E2E checks. I found both through ordinary
editing gestures: double-clicking a repeated identifier, and Ctrl/Cmd+F.

## Prior fix verified: the selection shows through the active line (observed live)

| Editor / gesture | Scheme | Width | Painted stack at the word | Composite | Text contrast |
|---|---|---|---|---|---|
| Paste specimen, double-click comment word `visitor` on the active line 4 | light | 1280 | field `#fff` → selection `rgb(219,231,251)` → active `rgba(240,243,248,.25)` | `rgb(224,234,250)` vs unselected active `rgb(251,252,253)` | 4.66 (`#5e6877` comment) |
| Same selection, kept after switching scheme | dark | 1280 | field `rgb(26,29,33)` → `rgb(39,54,74)` → `rgba(35,43,54,.25)` | `rgb(38,51,69)` vs unselected `rgb(28,33,38)` | 5.69 |
| Same specimen after switching dark → light, double-click comment `greet` | light | 320 | as in light above | `rgb(224,234,250)` | 4.66 |
| Same specimen, Control+A, head row 4 sampled at 4 x-positions | light | 320 | selection under translucent active line on every sample | `rgb(224,234,250)` | 4.66 to 13.51 |
| Existing Run Log, double-click `--surface` on line 10 | light | 390 | same | `rgb(224,234,250)` | 13.51 |
| Existing Run Log, Control+A, scrolled to the end | light | 390 | screenshot: lines 720–740 selection-blue through `</html>`; head on empty line 741 with the faint active band | — | — |

- The specimen was four lines of mixed HTML, CSS and JS with no trailing newline.
- `getSelection()` returned the expected word each time. The selection and the typed draft survived light → dark → light.
- Horizontal overflow was 0 at 320 on `/new`. The selection on the cursor line is visible in both schemes, so the round-11 finding is resolved.
- **Side effect (source and live, aesthetic):** with the active line at 25% opacity, light composes to `rgb(251,252,253)` on white.
  The current-line band is almost invisible in light (about 1.02:1), and somewhat stronger in dark.
  I accept this as a quiet choice and do not count it as a defect.

## Material finding 1: the stock lime selection-match outshines the actual selection (light, observed live)

- **Routes:** `/new` Paste, at 1280 and 320, and `/artifacts/461183aa-…/edit`, at 390.
- **Reproduction:** double-click an identifier that appears more than once.
  - In the specimen, the comment word `visitor` also appears inside a string.
  - `greet` appears as a variable and in the comment.
  - In Run Log, `resizeTimer` appears four more times on lines 726–732.
- **Painted (measured):**
  - The other occurrences get `.cm-selectionMatch` `rgba(153,255,119,.5)`, which composites to saturated lime `rgb(204,255,187)`.
  - The selected word gets pale blue `rgb(224,234,250)`.
  - On screen, the echoes are clearly louder than the selection. At 320 the lime `greet` reads as the selected word and the actually selected comment `greet` reads as a hint.
  - Text contrast stays high (14.49 on lime), so this is a problem of emphasis and harmony, not of legibility.
- **Cause (source):** `web/editor/editor.js`'s `appEditorTheme` sets tokens for the field, gutter, active line, selection and cursor, but not for `.cm-selectionMatch`.
  So light keeps basicSetup's stock `#99ff7780`, the only lime anywhere in the app palette.
- **Dark is fine:** One Dark supplies `rgba(170,254,102,.1)`, which composites to a quiet olive. It sits at a similar weight to the selection `rgb(38,51,69)` and does not compete with it.
- **Why material:** this is the same component and the same gesture class as round 11. Selecting a variable name is the commonest double-click in code.
  The rubric's contrast-and-emphasis category asks whether the most important thing reads strongest, and here the inverse happens in light.
- **Suggested fix, not prescribed:** add a `--color-code-match` token per scheme to `appEditorTheme`.
  Make it a subtle accent-family tint, or an outline, that stays weaker than `--color-code-selection`.

## Material finding 2: the source editor's find/replace panel is unstyled CodeMirror chrome (both themes, observed live)

- **Route:** `/artifacts/461183aa-…/edit` → Artifact source → Ctrl+F. Seen at 390 light and 1280 dark.
- **Observed (dark, measured):**
  - The find/replace inputs and the `next` / `previous` / `all` / `replace` / `replace all` buttons are **16px tall with 9.1px text**.
  - The buttons use a `linear-gradient(rgb(57,57,57), rgb(17,17,17))` fill, a 1px radius and an `rgb(136,136,136)` border.
  - The checkboxes are native 13px with lowercase labels, and the close control is an 11×15 `×`.
  - For comparison, the app button on the same page is 40px tall, 14px text, radius 6px, with a `rgb(62,70,82)` border on a token surface.
- **Observed (light, screenshot):** the same layout uses light-grey gradient buttons and stock yellow `.cm-searchMatch` fills, with an orange current match.
  At 390 the panel's tiny controls sit directly above 44px app controls.
- **Why material:** the source editor is a named in-scope shared component, and earlier rounds counted "foreign-looking CodeMirror surfaces" as material.
  Find is a standard editor action. When used, this panel is the most off-system piece of UI in the app: its type size, control height, casing, radius and fills all disagree with the shared components.
  This round's selection repair did not cause it; it was simply never reviewed. It was not captured, and I did not open it in earlier rounds either.
- **Suggested fix, not prescribed:** theme `.cm-panels`, `.cm-panel.cm-search`, `.cm-textfield`, `.cm-button` and `.cm-searchMatch` / `-selected` from app tokens.
  Use the field font size, control heights, radius, border and focus ring, and sentence case. Alternatively, drop the search keymap if find is not intended.
- **Scoring:** I count this once, under component consistency in both themes. I did not also lower typography, though 9.1px text is a typography flaw too.

## Other fresh surfaces (observed, supporting the 8s)

- **`/new`, 1280 light and dark:**
  - The three method cards, the selected Paste state, the label/field rhythm and the recessed source field are coherent.
  - The syntax colours are complete in both schemes. The light syntax colours are CodeMirror's default palette with three darkened hues; they are vivid but legible.
- **`/new`, 320 light:**
  - The segmented Agent/Paste/URL control is used.
  - The 44px full-width "Add to library" sits above the helper and then Cancel. Overflow was 0.
- **Editor, 390 light and 1280 dark:**
  - Section summaries, dividers and the recessed source field match.
  - The header uses icon squares at 390 and labelled buttons at 1280. The status read "All changes saved" throughout.
- **Library, 1280 dark:** the rail, the active space tint and the counts are balanced. Cards keep an even grid with restrained tiles, and the sharing glyph sits beside the tag dot.
- **Settings, 320 light:** the cards, headings and icons are consistent. Labelled values are readable. `admin` is bold while Storage values are regular, as before.
- **Share, 320 light:**
  - Overflow is not visible. URL and Copy share one 44px line.
  - "Replace link" (32px) sits above its own helper line. Embed tile and the People section are separated by a rule.
  - The access explainer runs to 7 lines at this width, which is dense.
- **Global Search `q=packing`, 390 dark:**
  - The 44px field has an integrated clear. The full-width card carries the `council-guest` owner chip.
  - The fixed bottom tabs wrap symmetrically with a clearly tinted active Search tab. Overflow was 0.

## Reassessed minor observations (not material)

- **Share access explainer:** 7 lines at 320. It is dense but readable. It is a refinement.
- **Replace link and Revoke at 32px**, against 44px fields, Copy and Done: still true. The 15px Revoke inset is carried from round 11; I did not re-measure it.
- **Settings at 320:** "Sign out" and the disabled "Delete account" are 36px while "Accounts" is 44px. Bold `admin` against regular Storage values: minor.
- **Italic "Untitled" Title placeholder** against the regular placeholders elsewhere: unchanged and minor.
- **Empty source prompt clipped at 320, and Edit-tag Save wrapping at 320:** not reopened this round. I carry them as minor.
- **Light active-line band is nearly invisible** after the alpha change. This is an aesthetic choice. A faint gutter tint or a slightly stronger alpha would keep orientation without covering the selection.

## Coverage and limits

- **Live:**
  - Paste at 1280 light and dark, and 320 light, including a scheme switch that kept the selection.
  - The existing source at 390 light (word, Control+A with the head row visible, repeated identifier, find panel) and 1280 dark (find panel, matches).
  - Library 1280 dark, Settings 320 light, Share 320 light, and global Search 390 dark.
- **Not freshly reviewed:**
  - I did not take a separate painted-stack word selection in the existing source at 1280 or 320, or in dark at 390. The cause is a token, independent of width and editor; I saw dark composite behaviour on Paste.
  - I did not take a painted-stack measurement of the light find panel. Its sizing comes from the same CodeMirror base theme as dark; light was observed by screenshot only.
  - I also did not re-check Versions, the Tags and Edit-tag modals, Duplicate, `/shared`, `/shared-by-me`, viewer popovers, desktop Share or the card menu. For those I relied on round 8–11 evidence and the round-12 captures, without scoring them upward.
- **Environment limits:** resizing keeps a desktop user agent and fine pointer. I used no real touch devices, software keyboards or other engines.
- **Source inference:** the causes come from reading `web/editor/editor.js` and `web/gallery/tokens.css`. The symptoms were observed live.
  `grep` finds no `selectionMatch` or `cm-search` coverage in `e2e/` (apart from node_modules). No round-12 capture shows the find panel.
- **Data safety:**
  - I saved, created, deleted, granted, replaced or mode-changed nothing.
  - I cleared the Paste specimen (Control+A, then Backspace) before leaving `/new`. No leave prompt appeared.
  - I only selected and searched the existing source. Find was closed with Escape, and the status stayed "All changes saved".
  - I dismissed the Share dialog with Escape.
  - I made no builds, test runs, commits, app edits or capture overwrites. The only file I wrote is this report.
- This report does not declare council completion. From my side the joint qualifying streak stays at zero.
