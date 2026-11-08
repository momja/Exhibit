# Round 10 — independent Sol visual review

Reviewer: **gpt-6.1-sol**. Reviewed 2026-11-04.

Frozen candidate: `1c86da10284c20832f121e2b854b8d1d93e40706`. HEAD and merge-base with `origin/main`, `44def51afa43019771fd258bc61b28e3ebf4613b`, independently confirmed. Running app: `http://t3code:18280`; observed asset version `53abe758a922`.

Read the [fixed visual brief](visual-brief.md), both models' round 5–9 reports, the round 6/7/8/10 change notes and [visual validation](visual-validation.md). Did **not** read the other model's round-10 report or scores. Supplied tests and previous scores do not determine these ratings.

**Verdict: FAIL for Sol. Contrast and emphasis is 7.5 in each theme. The other ten scores reach 8.** Light syntax highlighting is genuinely restored, but focused text selection still uses stock CodeMirror backgrounds that reduce several syntax colors below 4.5:1. This is an observed remaining legibility issue, not a preference for a different palette. Round 10 cannot establish a joint pass; the qualifying joint streak remains zero.

## Twelve independent scores

Each category considers its weakest materially inspected surface. An 8 means polished/coherent without a substantial visual issue, not excellent or exceptional design.

| Fixed category | Light | Dark | Fresh visual basis |
| --- | ---: | ---: | --- |
| Typography | 8 | 8 | Clear page/section/label hierarchy, consistent system controls and readable ordinary source text. The selected-text contrast defect is scored under Contrast and emphasis. |
| Composition and spacing | 8 | 8 | Aligned desktop card title areas and Settings facts; orderly editor sections/history; full-width phone sharing choices and a correctly separated replacement helper. |
| Color harmony | 8 | 8 | Coherent neutral shell, blue emphasis and restrained default tiles. Restored light syntax has distinct, compatible colors; dark source remains integrated with recessed fields. |
| Contrast and emphasis | **7.5** | **7.5** | Focused code selection renders on stock backgrounds rather than the app selection token. Several small syntax/comment colors are below 4.5:1; details below. Other inspected headings, controls, selected options and attribution remain clear. |
| Component visual consistency | 8 | 8 | Full syntax styles are now present in both themes. Fields, buttons, menus and dialogs share compatible typography/surfaces; phone URL/Copy and tag Add/field pairs align. The selection defect limits contrast rather than requiring a second deduction for the same issue. |
| Responsive visual polish | 8 | 8 | Sampled 1280/390/320 layouts retain readable measures, aligned tabs, integrated Search clear, reflowed history and stacked sharing. No material responsive composition regression found; the code-selection issue occurs across widths. |

## Material finding — focused source selection bypasses the intended background

**Priority: P2. Observed live in both actual schemes.**

**Routes and selectors:**

- `/new` → Paste → `role=textbox[name="HTML source"]`; `.cm-editor.cm-focused .cm-selectionBackground` and `.cm-content span`. Reproduced and visually inspected at **1280×800, 390×844 and 320×740 in light and dark**.
- `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332/edit` → Artifact source; same selection/token selectors. Independently reproduced on the existing source at **1280×800, both schemes**, without editing it.

**Reproduction:** In Paste, enter an unsaved HTML specimen containing a doctype, HTML attributes/strings, CSS comment/properties/numbers, JavaScript comment/function/string/escape/regexp. Keep the source textbox focused and press **Control+A**. Switch actual light → dark → light. The syntax colors and buffer survive, but the selected text visibly loses contrast on the selection fill. The specimen was cleared back to the original empty value afterward; nothing was submitted successfully or saved.

Measured the actual rendered `.cm-selectionBackground`, not a hypothetical surface resolved from a token:

| Scheme | Intended root selection token | Actual focused selection fill | Examples of rendered foreground / contrast on actual fill |
| --- | --- | --- | --- |
| Light | `#dbe7fb` | **`#d7d4f0`** (`rgb(215,212,240)`) | doctype/comments `#5e6877`: **3.91:1**; HTML tags `#06734a`: **4.09:1**; escape/regexp `#b53600`: **4.17:1** |
| Dark | `#27364a` | **`#3e4451`** (`rgb(62,68,81)`) | attributes/CSS properties/JS names `#ed8088`: **3.74:1**; keywords/units `#cf87e5`: **3.84:1**; CSS color values `#d19a66`: **3.96:1**; operators/escape/regexp `#56b6c2`: **4.12:1**; functions `#61afef`: **4.13:1**; comments/meta `#a4aebd`: **4.36:1** |

These ratios use live computed token colors and the opaque, live computed selection background, with the usual sRGB relative-luminance calculation. The code measured 13 CSS px with this browser's fine pointer. At 1280, 390 and 320 the Paste measurements agreed; the existing editor also returned the same failing tag/meta/CSS colors on the same fills at 1280. This is a common editing state affecting multiple token families, rather than one marginal token or an unobserved accessibility assertion.

**Cause, supported by live generated CSS and source inspection:** `web/editor/editor.js:39` uses the focused selector `&.cm-focused .cm-selectionBackground`. Its generated rule has three class selectors. Both CodeMirror's light focused-selection rule and One Dark's rule use the more specific hierarchy:

```css
.theme.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground
```

That hierarchy has five class selectors and wins over the app override despite `Prec.highest`. Live generated CSS contains the stock `#d7d4f0` light and `#3E4451` One Dark declarations, as well as the less-specific app token declaration; computed backgrounds confirm which wins. The exact generated theme class names are runtime details.

**Why the supplied E2E pass misses it:** inspected `e2e/specs/ui-appearance.spec.ts:59`. The test creates a temporary span with `color:var(--color-code-selection)` and uses that resolved token as its selection background. It never creates a focused code selection or reads `.cm-selectionBackground`. Thus its reported floor applies to the intended token, while the rendered selection uses another color. This explanation is source evidence; the failing focused presentation and ratios above were independently observed first. I did not rerun or alter the test.

**Suggested correction:** match the stock focused-selection hierarchy in the app theme so the intended token actually wins. Then select real source text and calculate contrast against the computed selection-layer background through actual theme changes. Preserve the complete light/One Dark highlighting rules. Recheck both Paste and the existing source editor; do not reduce the contrast floor or treat the root token's value as proof of the rendered fill.

## Round-9 syntax regression — genuinely fixed, separately from selection

Observed live on existing Run Log source, both schemes at 1280, with additional 390 source inspection. Light now has **seven distinct span colors**, including green HTML tags, red strings and differentiated CSS values. Dark has **eight** across the inspected existing-source spans. This is a real restoration, not merely a claim that flattened source is readable. The earlier Sol characterization of missing light highlighting as “subdued” was incorrect; the failed joint round is not carried forward.

The separate unsaved mixed-language specimen produced **eight light / nine dark** syntax colors, including JavaScript function, string/escape and regexp treatments. Switching schemes retained the exact specimen text. From those live colors, the minimum sampled contrast was **5.64:1 on the light field / 5.07:1 on its active line**, and **6.48:1 on the dark field / 5.47:1 on its active line**. The remaining failure is specifically the focused selection background. These samples are not an exhaustive audit of every grammar token.

## Other fresh observations and retained fixes

- **Share, 390/320, both themes:** inspected top, People, choices and bottom live. The replacement helper is **6px below** the button in all four combinations. URL and Copy are both **44px**. Choices are **312px wide at 390 / 242px at 320**, stacked with readable copy and clear selected treatment. Dialog and document horizontal overflow measured zero. People placeholder and comma helper are complete; the disclosure is sentence case. No sharing control was changed.
- **Editor Tags, 390, both themes:** creation fields span the section, and Add/Create controls visually align with the 44px fields. Opened Edit tag Health in both themes and dismissed it unchanged. Modal fields, color controls and footer remain orderly.
- **Versions/footer, 320, both themes:** clear version/label/date/badge/action grouping; readable explanatory text; full-width return action and 44px footer controls. No View/Restore/Delete action invoked.
- **Library, 1280/390, both themes:** coherent headings/cards, reserved desktop title areas, quiet dark outlines and restrained default tiles. Opened the Run Log card menu at 1280 dark; its icon/text spacing and layering agree with the shell. Opened Duplicate at 390, inspected both themes, and cancelled it.
- **Search:** `q=run` at 1280 both themes; blank, incoming `q=packing` and miss `q=not-in-this-library` at 320 both themes. Integrated clear, readable owner chip, aligned fixed tab icons and well-proportioned empty states hold up. Measured zero document overflow on those 320 Search states.
- **Shared spaces, 390, both themes:** `/shared` and `/shared-by-me` retain coherent card/title/attribution and fixed-tab presentation.
- **Settings:** 1280 both themes at Profile/Administration/Storage; 390 both themes through Storage/Sign out/Delete account. Facts align/stack comfortably, headings share icons, destructive entry is restrained and its disabled explanation is readable.
- **Add:** agent at 1280/390 both themes; URL at 390 both themes; Paste mixed-language source at all three widths in both themes, plus blank invalid-Paste at 320 both themes. Method selection, fields and phone footer composition remain coherent. Empty Paste was the only submission, producing “Paste an artifact first.” No URL was entered or fetched and no agent was started.
- **Viewer chrome, 390, both themes:** toolbar, posture popover and More menu inspected. Labels/icons/actions remain legible and appropriately separated. The light Run Log body under dark preference is user-authored and excluded from scoring.

### Remaining refinements, not additional material findings

- Phone Share's long access explanation remains dense, especially at 320. It has a readable measure and consistent section spacing; shortening it would improve lightness without being required to resolve a broken composition.
- Replace link/Revoke remain compact relative to URL/Copy/Done. Revoke is slightly inset from the section edge. They remain readable and separated; I still treat these as density/alignment refinements rather than an independent under-8 surface.
- Settings' bold `admin` versus regular storage values, and italic input versus normal textarea placeholders on Add, remain small inconsistencies with no substantial hierarchy or legibility harm.
- The blank source placeholder clips horizontally at 320 within the scrollable editor, while its visible HTML source label identifies the field. Minor finish opportunity, not document overflow or a material composition defect.

## Method, evidence and limits

- Used only my own T3 preview tab, **`tab_k_0d23e38c-d413-4e76-b0eb-59687a206343`**, opened with `reuseExistingTab:false`; passed its ID on every subsequent preview call. No other reviewer's tab was operated.
- Explicit `preview_set_appearance` light/dark; live `matchMedia('(prefers-color-scheme: dark)').matches` returned **false/true at each of 1280, 390 and 320 CSS px**, including the key source-selection checks. No root-class emulation.
- Scores use fresh live screenshots and computed styles/geometry. Historical reports and change/validation notes are context. The existing round-10 capture set was not substituted for these live observations; no capture was overwritten and no screenshot file was written.
- Source inspection was limited to explaining the already observed selection defect and the corresponding test gap. It was not used to declare unobserved UI fixed. No build, lint or test suite was run, including the parent's Go run.
- Sampled coverage, not every route/state/theme/viewport combination. Desktop URL/Share expanded embed/disclosure content, collection/filter overlays and misses, enlarged text, editor dirty-state emphasis, arbitrary long values and post-save states were not freshly reviewed. Actual coarse-pointer devices, software keyboards and alternate engines were not tested; resizing retains a desktop user agent/pointer.
- App-owned shell/cards/default tiles/editor/overlays are in scope. User-authored widget/artifact bodies, provider-dependent agent execution and experimental connectors are excluded.
- No valid saves, creates, deletes, grants, access-link replacements or data-mode changes. The temporary Paste specimen was restored to empty; existing source was selected only, never edited. No app edits, builds, commits, persisted fixture changes, external URL fetches or subdelegation. **Only this assigned report was written.**
