# Round 5 — independent Sol visual audit

Baseline: `a65a7cfe`, rebased on `origin/main` `44def51a`. Reviewed the running build at `http://t3code:18280`. This is a fresh visual baseline under the six-category rubric; previous round scores were not used. This round does **not** pass, and is not a completion review.

## Scores

Scores reflect the weakest materially reviewed app-owned surface, rather than an average across pages. An 8 means polished/coherent with no substantial visual issue; it does not mean no possible aesthetic refinement.

| Fixed category | Light | Dark | Limiting evidence |
| --- | ---: | ---: | --- |
| Typography (scale/weight/hierarchy) | 8.0 | 8.0 | Page headings, card titles, form labels and secondary metadata have a coherent hierarchy. No substantial type issue identified independently of the contrast issue below. |
| Composition & spacing | 7.5 | 7.5 | Share saved-data choices retain two very narrow columns at 320px. |
| Color harmony | 8.0 | 8.0 | Neutral surfaces and blue accent are coherent; default monogram tile and attribution adapt to both modes. |
| Contrast & emphasis | 7.5 | 7.0 | Source editor placeholder in both modes; additional subdued syntax/gutter text in dark mode. |
| Component visual consistency (buttons/forms/cards/menus/dialogs/editor) | 8.0 | 8.0 | Shared control shapes, accent treatment, cards and dialogs are coherent. The editor has its own code palette, but no incorrect light native-control surface appeared in dark mode. |
| Responsive visual polish | 7.5 | 7.5 | The share comparison layout needs a narrow-screen treatment, not merely smaller padding. |

The same finding can limit composition and responsive polish; those are not two separate implementation requests. Only three of six categories pass in both modes. There is no qualifying consecutive-pass streak from this report.

## Prioritized findings

### 1. P2 — subdued source-editor text is too faint, especially in dark mode

**Observed:** `/new` → Paste HTML, `.cm-placeholder`; `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → Artifact source, `.cm-gutters` and the subdued `<!doctype html>` syntax span. Seen on desktop and narrow layouts. The neighboring title/description controls have stronger, clearer secondary text; the mounted source editor takes a visibly weaker gray from its independent defaults.

Computed browser evidence:

- Dark editor surface: `rgb(40,44,52)` (`#282c34`). Placeholder: `#888888`, approximately **3.95:1** on the base surface and lower on the tinted active line. Gutters and the doctype syntax: `#7d8799`, approximately **3.86:1** on the base surface.
- Light Paste HTML placeholder: `#888888` on white, approximately **3.54:1**, again weaker on the tinted active line.
- Editor type measured 12 CSS px with this desktop browser's fine pointer, so these are small-text contrasts. This is not a claim that every syntax color fails: the principal code colors and ordinary source text were substantially clearer.

**Practical fix:** Add deliberate light/dark CodeMirror theme overrides for placeholder and gutter text, and a highlight-style override for subdued comment/doctype syntax. Choose colors against the editor's actual base and active-line backgrounds. Reuse the app's secondary-text intent, or add editor-specific tokens that reach at least 4.5:1 for these small characters. Preserve the working live media-query theme switch. The source location is `web/editor/editor.js` (`themeFor` currently selects stock One Dark or the default light theme); the surrounding sizes/borders are in `web/gallery/new.css` and `edit.css`.

**Classification:** Concrete visual-legibility issue supported by measured colors, not a preference for a different syntax palette.

### 2. P2 — mobile share comparison cards become narrow text towers

**Observed:** Run Log viewer → Share Settings → Saved Data for People with Access, `#share-panel .state-choices` / `.state-choice > span`, both light and dark. At 320 CSS px the dialog is 288px wide, each choice is **116px wide × 277px high**. Headings wrap to three lines and the explanatory text breaks into many short lines below the large illustrations. The illustrations and borders dominate a cramped comparison. At 390px the same two-column treatment remains visibly tight, although less severe. Desktop presentation is balanced.

**Practical fix:** Introduce a narrow breakpoint that stacks the two choices or makes each a full-width row with a compact illustration beside the title and description. Keep the selected treatment and readable type; do not solve this by shrinking the descriptions. `web/gallery/detail.css` keeps `.state-choices` at `grid-template-columns:1fr 1fr`; its existing 480px rule only reduces glyph width and padding.

**Classification:** Concrete responsive composition weakness, not an overflow or data-mode behavior defect. Both options remain readable and operable, but the narrow composition is below the stated polished threshold.

## Aesthetic observations, not additional blockers

- The dark app palette itself is not visibly broken. Ground `#16181c`, panel `#202328`, blue links/actions, default tiles and owner attribution form a coherent system. I found no evidence for replacing the whole palette.
- Form/control borders are intentionally much stronger than panel dividers. `/new` and expanded editor Tags therefore have a fairly outlined, utilitarian appearance. A designer could reduce secondary boundary noise with a distinct resting-control token, but that is a preference unless text/control recognition remains clear. Do not indiscriminately weaken the current contrast to make screenshots softer.
- The green default monogram tile is more chromatic than the neutral shell, but it remains legible and is an intentional artifact identifier. I do not count a preference for blue/gray tiles as a defect.

## Observation and source limits

- Used my own T3 tab `tab_9_0d23e38c-d413-4e76-b0eb-59687a206343`, opened with `reuseExistingTab:false`, with that tab ID on every subsequent browser call.
- Explicit `preview_set_appearance` calls selected light and dark. Browser evaluation confirmed `matchMedia('(prefers-color-scheme:dark)').matches === true` at 1280, 390 and 320 CSS px; explicit light returned false. This was actual media emulation, not a root-class substitution.
- Inspected at 1280×800, 390×844 and 320×740. Coverage was focused rather than every surface at every size: desktop gallery/card menu/duplicate dialog, Search owned results (`q=run`) and incoming result (`q=packing`, including `council-guest` attribution), all three creation methods, settings, editor title/tags/source/versions, and viewer controls/share. Phone/narrow coverage included gallery in both modes, filter miss (`tag=Health&tag=Reading`) in both modes, incoming Search in both modes, settings in both modes, editor tags/source/versions in both modes, all creation methods, share comparison in both modes, and viewer info/menu controls. Some desktop views and some creation-method/viewport/theme combinations were sampled in only one mode; this is not an exhaustive screenshot matrix.
- Inspected rendered screenshots and computed styles, then checked relevant rules in `tokens.css`, `components.css`, `new.css`, `edit.css`, `settings.css`, `detail.css` and `web/editor/editor.js`. Source inspection supports the findings; it does not replace visual observation.
- This browser resize retains the desktop user agent/fine pointer. Actual coarse-pointer font sizing, on-screen keyboards, alternate browser engines, long arbitrary fixture values, and post-save states were not tested. `tokens.css` already increases code/control fonts for coarse pointers, so I am not reporting the observed desktop 12px editor font as an actual-phone defect.
- The Run Log artifact body remains light under dark preference; that is user-authored content outside this audit's app scope. Its inputs/chart/layout were excluded from scoring. App-owned toolbars, dialogs, default monogram tiles, card metadata and source-editor presentation were included.
- No artifact, tag, share grant, account or saved data was changed. Duplicate was cancelled; Share was closed without changing controls. No save/create/delete/grant or provider-credential requirement was exercised. No production code edits, commits or delegation were performed; only this report was written.
