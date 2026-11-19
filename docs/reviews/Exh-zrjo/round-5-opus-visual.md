# Exh-zrjo — Round 5 visual audit (Opus, independent)

Build: rebased `a65a7cfe` on `origin/main` `44def51a`, app at `http://t3code:18280`.
Rubric: the six fixed visual categories, scored separately for light and dark.
Each score reflects the weakest material surface, not an average. Rounds 1–4
are historical context only and do not count toward these scores.

## Method

- Opened my own preview tab, emulated `prefers-color-scheme` with
  `preview_set_appearance`, and confirmed
  `matchMedia('(prefers-color-scheme: dark)').matches === true` at 1280 and
  390. Switched to light explicitly to compare.
- Used the `t3code` host, not `127.0.0.1`. On loopback, `frame-ancestors`
  blocks the widget frames, so cards fall back to monogram tiles that the
  real setup would not show.
- Covered at 1280: gallery (`/`), `/search?q=run`, `/new` (agent and paste
  modes), `/settings`, `/artifacts/RunLog/edit` (Tags and Versions opened, plus
  source), the viewer pill and posture popover, the share dialog (closed
  without changes) and the card action menu. Covered dark at 390 (gallery) and
  320 (`/search?q=pack`, cross-library result with owner attribution).
- Read computed styles with `preview_evaluate`. Source:
  `web/gallery/tokens.css`, `components.css` and `web/editor/editor.js`.
- Nothing was saved, created, deleted or granted.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 7 | 7 |
| Composition & spacing | 7 | 7 |
| Color harmony | 8 | 6 |
| Contrast & emphasis | 7 | 6 |
| Component visual consistency | 7 | 6 |
| Responsive visual polish | 7 | 7 |

No category passes in both modes. Dark is weakest on three categories: color,
emphasis and component consistency. That fits the user's complaint that some
dark design choices look bad.

## Prioritized findings

### D1 (dark, objective, high): control borders are the loudest element on the page

- **Evidence:** dark `--color-border` is `#748198`, a bright blue-gray. It
  goes on every input, select, `.btn-sec`, the header search field, the
  Accounts and Settings buttons, Cancel and Return to Library, and the
  CodeMirror frame. Card borders use `--color-card-border` `#363b42`, and
  dividers use `#2f333a`. So at 1280 on `/`, the header's outlined
  search/Accounts/Settings are the highest-contrast edges on screen, louder
  than the cards and the content. On `/new` and `/edit`, every field reads as
  a bright outline inside a dark panel. At 390, the header is three heavily
  outlined squares (Filter, Settings, plus the blue +).
- **Cause:** the value probably exists to clear 3:1 non-text contrast. But
  3:1 against `#202328` only needs about `#5d6573`.
- **Fix:** drop dark `--color-border` to roughly `#5a6270`–`#626a78`, which
  still gives ≥3:1 on panel. Optionally give inputs a recessed fill
  (`--color-subtle` `#1a1d21`, or a new `--color-field` token) so fields read
  as wells and are not defined by outline alone. Header secondary buttons
  could lose the border on dark and rely on a hover fill.

### D2 (dark, objective, high): the source editor uses a foreign palette

- **Evidence:** in dark, `/new` (Paste HTML) and `/edit` (Artifact source)
  show `.cm-editor` with background `rgb(40,44,52)` and text
  `rgb(171,178,191)`. That is stock One Dark, from
  `web/editor/editor.js:28` (`themeFor = dark ? oneDark : []`). App panels
  and inputs are `#202328` with ink `#e8eaed`. The editor reads as a lighter
  slate box with dimmer text inside a panel of a different hue, framed by the
  bright D1 border, inside a hairline `details` box. That is three nested
  edges.
- **Fix:** keep One Dark's syntax highlight. Layer an `EditorView.theme(...,
  {dark:true})` that sets `&` / `.cm-gutters` background to the field token,
  content color to `--color-ink`, gutter color to `--color-muted`, and the
  active line to `--color-hover-fill` at low alpha. CSS variables work inside
  `EditorView.theme`.

### D3 (dark, objective/aesthetic, high): default tiles are muddy and over-saturated

- **Evidence:** dark tile tokens are `--tile-face-top:30% 23%` and
  `--tile-face-bottom:32% 17%`, with a mono fill of `30% 32% / .55`. Under
  fixture hues the default tiles come out as forest green ("Unit Converter",
  "Shared packing list"), brown/rust and olive. Seen on `/` at 1280 and 390,
  and on `/search?q=pack` at 320. Next to app-neutral widget tiles they are
  the most saturated surfaces in the library, and they look like a different
  product. In light the same tokens give soft pastels that sit well.
- **Fix:** lower dark saturation to about 12–16% and lightness to 18–20%, or
  use a neutral `--color-panel` face with only the monogram chip tinted
  (saturation about 25%, lightness about 28%), with ink at about 75%
  lightness. This is partly aesthetic, but the mismatch with the light
  treatment is objective.

### T1 (both, objective, high): form controls render in Arial

- **Evidence:** computed `font-family` is `Arial` on `<input>`, `<select>`
  and `<button class="btn">` on `/edit` and `/new`, while `body` is
  `system-ui, sans-serif`. `<a class="btn">` inherits system-ui. So on `/new`,
  Cancel (an `a`) and Start building (a `button`) use different typefaces in
  the same footer. "Choose an existing tag…", the share-panel buttons
  (Copy, Replace link, Revoke), View/Restore and Delete are all Arial. No
  global `font:inherit` reset exists. The only matches are on
  `.action-menu`, `.route` and `.banner-action`.
- **Fix:** add `button,input,select,textarea{font:inherit}` (or
  `font-family:inherit`) in `components.css`. Then re-check field heights,
  because metrics change.

### C1 (both, objective, medium): control heights and secondary-button colors vary

- **Heights:** inputs are 34px, `.btn-sm` 32px, `.btn` 36px, `a.btn-sec`
  36.9px (the border adds to `min-height`) and header controls 40px. On `/new`
  the Cancel/Start building pair is visibly off by 1px. On `/edit` Tags, the
  Add button (32) sits beside the select (33.5).
- **Text color:** `.btn-sec` text is `--color-ink-soft`, but the header
  Accounts/View artifact links render in accent blue. So "Accounts" is blue in
  the header and gray on `/settings`.
- **Fix:** pick one control height per density (36 default, 32 small) and
  apply it to inputs too, with `box-sizing:border-box`. Make header secondary
  actions use the same `.btn-sec` color, or make all secondary actions accent.

### C2 (dark, objective/aesthetic, medium): the danger fill is too heavy

- **Evidence:** `/edit` footer Delete and `/settings` Delete account use a
  solid `#f2726a` salmon fill with near-black text. The Delete account card
  also has a full danger-colored border. In dark this is the brightest fill
  on the page after the primary button. Routine destructive affordances read
  as alarms.
- **Fix:** use an outlined or tinted danger style (`--color-danger-tint`
  fill, `--color-danger-ink` text) for the resting state. Keep the solid fill
  for the confirmation step.

### E1 (dark, objective, medium): placeholders look like entered values

- **Evidence:** dark `--color-placeholder` is `#a0a6af`, against value ink
  `#e8eaed` and the same field background. "Session timer" and "Untitled" on
  `/new` read like prefilled content.
- **Fix:** about `#7d8490`, which still gives ≥4.5:1 on `#202328`, or
  italic/lighter weight.

### E2 (both, medium): edit sections look disabled

- **Evidence:** on `/edit`, `details > summary` is 13px, weight 400, in
  `--color-muted`. Section titles (Tags, Stored state, Artifact source,
  Versions) are the faintest text in the panel, quieter than the field labels
  above them. Each section is also a separate hairline box inside the panel,
  which adds boundary noise.
- **Fix:** make summaries 14px, weight 600, in `--color-ink-soft`. Drop the
  per-section box in favor of a top divider between sections, or remove the
  outer panel border.

### K1 (both, objective, medium): composition misalignments

- **Versions** (`/edit`, opened): the timestamp column moves between rows
  (x≈846 on v2 "current" versus x≈718 on v1 with View/Restore). Fix with a
  fixed-width trailing action cell, or a grid.
- **`/settings`:** the definition column starts at a different x in each card
  (344 in Profile, 355 in Storage). Section headings have icons on some cards
  (Storage, Delete account) but not others (Profile, Administration, Sign
  out). Fix with a shared `grid-template-columns` label width and a
  consistent heading-icon decision.
- **Gallery card meta:** long titles ("Marathon Training Plan") wrap to two
  lines at 1280, so that card's meta row sits higher than its neighbors' and
  the title collides with the top of the row. Consider clamping to one line
  with an ellipsis on desktop, or reserving two lines for every card.

### K2 (copy/typography, both, low): share dialog uses Title Case

The share dialog headings use Title Case ("Share Settings", "Link Access",
"Saved Data for People with Access", "Separate Data per Person"). Every other
surface uses sentence case ("Your account", "Add existing tag"). The dialog
title is 16px/600, only one step above the 14px body. Switch to sentence case
and consider an 18px dialog title.

### R1 (phone, medium)

- **390/320 tabs:** "Shared with me" and "Shared by me" wrap to two lines
  while "Library" and "Search" stay on one. The icons are therefore at
  different heights (y 638 versus 646 at 320), and the tab bar has no shared
  baseline. Use shorter labels ("Shared", "By me"), or top-align icons with a
  fixed two-line label box.
- **320 `/search`:** the clear (×) button sits outside the field, which
  shrinks to 248px, while at 1280 it sits inside. It looks detached. Keep it
  inside the field at all widths.
- **320 `/search` owner chip:** the "C" avatar is a dark disc on a dark card
  and is barely visible. Give it a tinted fill.
- **390 header (dark):** made worse by D1. Three bordered 44px squares with
  wide empty space between them.

## Objective versus preference

- **Objective:** T1, C1, D2, K1, E1, the R1 tab/clear layout, and the D1
  contrast hierarchy (outlines outrank content).
- **Mostly aesthetic:** D3 tile saturation (the light/dark mismatch is
  objective), C2 danger weight, K2 casing.

## Not material, or out of scope

- The artifact and widget content is user-authored: the Run Log page stays
  light in dark mode, and the mortgage widget repeats the card title. The
  viewer pill and posture popover read well in dark. The card menu is
  acceptable.
- Light mode is broadly coherent. Its main deficits are T1, C1, K1 and E2.

## Not tested

- Agent chat and preview page, URL ingest mode, the duplicate dialog itself
  (only the menu was opened), the admin users page, filter sheet at 390, the
  `/new` and `/edit` phone layouts, hover and focus-visible rings in detail.
- Phone widths were checked in dark only.
