# Exh-zrjo — Round 6 visual audit (Opus, independent)

Build: frozen `7eaf8354` on `origin/main` `44def51a`, app at `http://t3code:18280`.
Rubric: the six categories in [visual-brief.md](visual-brief.md), each scored
separately for light and dark from its weakest material surface. Round 5
scores are not carried over; each improvement was checked again here.

## Method

- In my own preview tab (`reuseExistingTab=false`), I set the appearance with
  `preview_set_appearance` and confirmed it with
  `matchMedia('(prefers-color-scheme: dark)').matches`: `true` in dark mode,
  `false` in light mode.
- **Observed live:** the 390px share sheet (`#share-panel` / `.share-modal`) on
  Run Log, both themes, scrolled top to bottom. I opened it with `showModal()`
  and closed it without changes.
- **Observed in captured screenshots** (`docs/screenshots/Exh-zrjo/visual-round-6/`):
  `editor-1280` (light/dark), `editor-390-dark`, `history-320-dark`, `gallery-1280-dark`,
  `gallery-390-dark`, `search-results-390-dark`, `add-390-dark`, `settings-1280-dark`,
  `share-390-dark`.
- **Not re-inspected this round:** card menu, duplicate dialog, the viewer
  posture popover, the URL/paste add modes in light, shared spaces at 320px,
  and Search empty/miss. I took the measured contrast values (dark field border
  `#647083` at 3.14:1, editor placeholder/meta at 4.5:1 or above) from the change
  notes and E2E results; I did not measure them again.
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

## What was fixed (verified independently)

- **Dark control borders (my round-5 D1):** fixed. Header Search, Accounts, the
  settings button and the editor's View/Save buttons have quiet outlines now.
  Fields read as recessed wells, so they are no longer the loudest edges on the
  gallery or the editor. The title field and the CodeMirror frame clearly frame
  their content without glaring.
- **CodeMirror:** it uses the app's own palette in both themes. The gutter,
  active line (`#232b36` on `#1a1d21`) and doctype/meta text are readable. The
  13px code is in proportion with the 15px UI text.
- **Editor sections:** these are now headed rows with dividers instead of nested
  boxes, with a clear hierarchy. The version list at 320px aligns well: the
  v-number column, a bold label, the date, a Current badge, and View/Restore at
  equal height.
- **Phone share sheet:** both data choices stack at full width, the copy is
  short and in sentence case, and the diagrams are gone. The selected card's
  tint and border are clear in both themes. The 116px text towers are gone.
- **Gallery:** cards have even title rows, and the dark default tiles are
  quieter. The phone tab icons line up, with full labels kept. Search results
  have an integrated clear button.
- **Settings:** the label and value columns are aligned, and each heading has an
  icon. Account deletion has a tinted entry that is restrained but clear.

## Remaining findings (none blocking alone, but they keep responsive below 8)

### R1 (both themes, observed): phone share "Replace link" row breaks raggedly
`.share-modal`, 390px: the helper text "The current link stops working." sits
beside the Replace link button and wraps, so "working." falls alone onto a new
line under the button. This is the most visible composition flaw left on the
sheet the user cares about.
**Fix:** at narrow widths, put the helper on its own line under the button
(`flex-basis:100%`, or a column layout), or keep it on one line beside the
button.

### R2 (both themes, observed): link field and Copy button heights differ
In the same sheet, the read-only share URL field is about 42px tall and the
Copy button about 36px. They are vertically centered, but the mismatch is
obvious next to the "paired rows align" change made elsewhere. The URL is also
truncated mid-character (`/s/0f`), with no fade.
**Fix:** make Copy as tall as the phone field (44px) in this row, or use the
36px field size here too.

### R3 (both themes, observed): title case left in a summary
In the same sheet, the expander reads "What Happens When I Switch?". Everything
around it is in sentence case, including the "Embed tile" expander just above.
**Fix:** change it to "What happens when I switch?".

### R4 (both themes, observed, minor): long placeholder truncated in the People field
"Usernames or emails, separated by comm…" is clipped at 390px.
**Fix:** use a shorter placeholder, such as "Usernames or emails", and keep the
separator hint in the helper text.

### R5 (aesthetic, not blocking)
- At 1280 in both themes, Save changes in the editor uses the outlined
  secondary style, beside View artifact. This is fine while "All changes saved"
  is showing. If it does not become the filled primary style once there are
  unsaved changes, the main action on the page has no emphasis. I did not test
  the dirty state.
- The Unit Converter default tile is still a fairly large olive block in dark
  mode. It is desaturated enough to be acceptable, but it remains the heaviest
  tile in the grid.
- The light-theme CodeMirror is noticeably less colored than the dark theme
  (tags and attributes are close to the base ink). It is coherent, just flat.
  I am not counting it against the scores.
- The collapsed editor section rows (about 61px) look slightly top-heavy at
  1280; there is more space under each label than above it.

## Verdict

This is a large improvement over round 5. Every dark-mode complaint I raised
before (loud borders, the foreign editor palette, muddy tiles, muted section
headers) is fixed and verified on the rendered page. Eleven of the twelve
scores reach 8. Responsive polish is 7.5 in both themes because of R1–R4, all
on the phone share sheet. They are small CSS and copy changes, and with them
fixed I would expect this category to reach 8. Under the brief, this is not a
passing round for Opus. Any round that does pass still needs a fresh
confirmation round.
