# Exh-zrjo — Round 7 visual audit (Opus, independent)

Build: frozen `a242183a` on `origin/main` `44def51a`, app at `http://t3code:18280`
(asset version `cdb36962aa17`). Rubric: the six categories in
[visual-brief.md](visual-brief.md), scored separately for light and dark from the
weakest material surface. Round 6 scores are not carried over.

## Method

- **Own tab**, opened with `reuseExistingTab=false`. Appearance was set with
  `preview_set_appearance` and checked with
  `matchMedia('(prefers-color-scheme: dark)').matches`: `true` in dark, `false` in light.
- **Observed live:**
  - The Run Log share sheet (`#share-panel`) at 390px in dark and light, top to
    bottom. It was opened with `showModal()` and closed without changes.
  - Run Log `/edit` at 390px in dark, with the Tags section open.
  - `/search?q=run` at 320px in dark.
- **Measured live:** element boxes and computed styles for the share rows, and the
  heights of the editor's Add buttons and the fields beside them.
- **Not re-inspected this round:** 1280 desktop pages, `/new`, `/settings`, the card
  menu and the dialogs. I relied on my round-6 live checks of these surfaces. The
  change notes say nothing outside the share sheet and the editor's Add/Create
  buttons changed, and the asset diff agrees (`detail.css`, `partials.tmpl`, and the
  editor's compact buttons). That is supplied and source evidence, not a fresh
  observation.
- Nothing was saved, created, deleted or shared.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition & spacing | 8 | 8 |
| Color harmony | 8 | 8 |
| Contrast & emphasis | 8 | 8 |
| Component visual consistency | 8 | 8 |
| Responsive visual polish | 7.5 | 7.5 |

## Round-6 findings: verification

| # | Finding | Status | Evidence (observed) |
|---|---|---|---|
| R1 | "working." left alone under Replace link | **NOT fixed** | At 390px in both themes, "The current link stops" still sits beside the button, and "working." wraps under it at the left edge. |
| R2 | Copy button shorter than the URL field | Fixed | Copy is 77×44 next to a 44px field. The URL ends in an ellipsis at rest (`http://t3code:18281/s/0…`). |
| R3 | Title case in a summary | Fixed | It now reads "What happens when I switch?". |
| R4 | Placeholder clipped | Fixed | The placeholder reads "Usernames or emails", with a visible "Separate names with commas." hint underneath. |
| Sol | Phone Add/Create height | Fixed | Both Add buttons are 44px next to 44px fields. "Create and add tag" is 44px. |

### Why R1 is still broken (objective root cause)

`web/gallery/detail.css:101` adds
`.share-replace-row{flex-direction:column;align-items:flex-start;gap:6px}` inside
`@media(max-width:767px)`. That rule only works on a flex container. But `.btn-row`
gets `display:flex` only from `web/gallery/edit.css:22`, and the detail page does not
load that file. On the live detail page, `getComputedStyle(.share-replace-row).display`
is `block`. The helper `<span class="text-sm muted">` therefore stays inline and wraps
next to the inline-flex button, exactly as in round 6.

**Fix:** add `display:flex;flex-wrap:wrap;align-items:center;gap:8px` to
`.share-replace-row` in `detail.css`, outside the media query. The existing phone rule
then puts the helper on its own line. Alternatively, make the helper `display:block`
on phones. Add a browser or E2E check that asserts the helper's top edge is below the
button's bottom edge at 390px, so this cannot pass on markup alone again.

## Other observations (none blocking alone)

- **Objective, minor:** in the 390px editor's "Create new tag" group, the Name and hex
  fields are 320px wide while the section is 338px wide. Their right edges stop 18px
  short of the Add button's right edge above them, which is visible in dark mode.
  Fix: let the group span the full width (`width:100%`, or remove the max-width).
- **Objective, minor:** in the share sheet, the divider under the `council-guest` grant
  row is slightly shorter than the dividers in the sections around it.
- **Objective, minor:** on the phone editor, "Return to Run Log" and Delete are 36px
  tall, while nearby phone controls are 44px.
- **Aesthetic:** the share sheet's link-access explainer is still a five-line block
  above the link at 390px. It is readable, and I am not counting it against the score.
- **Positive, observed:** the 320px dark Search page is clean. The clear button sits
  inside the field, cards and titles are even, the sharing glyph lines up, and the
  bottom tabs have aligned icons and a clear active tint. In the dark editor, the
  sections with dividers, the recessed fields, and the restrained CodeMirror palette
  all hold up.

## Verdict

Five categories score 8 in both themes. Responsive polish stays at **7.5 in light and
7.5 in dark**. The most visible phone-sheet flaw from round 6 (R1) still renders
exactly as before, because its CSS fix targets a flex property on an element that is
not a flex container on this page. R2–R4 and Sol's Add/Create refinement are verified
fixed. Once the one-line `display:flex` fix is in, I expect responsive polish to reach
8 in both themes. **This is not a passing round for Opus.** The candidate needs a fix
and then two consecutive fresh passing rounds.
