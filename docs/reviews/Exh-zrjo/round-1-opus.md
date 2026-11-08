# Exh-zrjo UI council: round 1 (Claude Opus), baseline at 0831c8a9

## Method and a capture defect

**The saved baseline screenshots are all the sign-in page.** All 14 files in
`docs/screenshots/Exh-zrjo/baseline/` share two hashes (one per width). The cause
is that `/tmp/exhibit-zrjo-auth.json` scopes its cookie to `t3code.local`, but the
app is reached at `t3code`. **Re-capture the baseline before using it as a
regression reference.**

What I did instead: I signed in through `POST /auth/local` and drove Playwright
(the copy bundled with shot-scraper). It captured all 8 in-scope routes at
1280/390/320 in light, and at 1280/390 in dark. It also ran DOM checks:
overflow, target sizes, accessible names, headings, landmarks and tab order.
Then I tested these by hand:

- the card action menu
- the duplicate dialog, cancelled with Esc
- empty submits in Add (agent and paste modes)
- the viewer's control pill and its menu
- keyboard focus in the gallery
- 200% root font size

The captures are in `/tmp/zrjo-opus-r1/out/` (not committed). Nothing was saved
or created. Tagged states, filter chips, populated shared spaces and URL-ingest
errors were **not observed**, because the fixture has no tags or collections and
a URL fetch would create an artifact. Findings marked *(source)* are inferred
from code rather than seen.

## Scores

| Category | Score |
|---|---|
| Visual hierarchy | **6** |
| Component consistency | **6** |
| Navigation / discoverability | **7** |
| Workflow feedback | **5** |
| Accessibility | **6** |
| Responsive behavior | **7** |

## What is already good (observed)

- **Navigation.** There is a clear spaces model: a desktop rail and phone bottom
  tabs with counts. `/` focuses search, with `aria-keyshortcuts`. The editor's
  "My library" return keeps context.
- **Duplicate dialog.** It has a native modal, autofocuses the title with the
  text selected, and Esc returns focus to the card trigger. The phone menu items
  are 44px tall.
- **Phone layouts.** No horizontal overflow at 390 or 320 on any route. Phone
  header targets are 44×44. Inputs are 16px on touch.
- **Dark mode.** It is coherent across every route, with no contrast cliffs
  spotted.

## Blocking findings, in priority order

### 1. Validation and status feedback on Add are weak and inconsistent (feedback, a11y)

- **Observed on `/new`, Paste mode:** an empty submit writes "Paste an artifact
  first." as plain body text in `#status`, below the editor. It is not styled as
  an error, the field is not marked, and focus does not move.
- **Observed in Agent mode:** the same mistake shows the browser's native
  "Please fill out this field." bubble. That is two validation idioms on one
  page.
- *(source)* In `web/gallery/new.js` `ingest()`, the `#status` element has no
  `role="status"`/`aria-live`. The "Add to library" button is not disabled while
  "Uploading…" or "Fetching page…" runs, so it can be submitted twice.
- **Fix:**
  - Use one inline-error pattern: an error token colour and icon, plus
    `aria-invalid` and `aria-describedby` on the field.
  - Move focus to the invalid field.
  - Put `role="status"` on progress and `role="alert"` on errors.
  - Set `aria-busy`/`disabled` on the submit button while the request is in
    flight.
  - Apply the same pattern to URL mode.

### 2. Empty states are bare sentences with no recovery (feedback, hierarchy)

- **Observed:** with no matches, `/?q=…` shows only "No artifacts match your
  search." in 14px muted text, top-left. It does not echo the query or offer a
  "Clear search" action. The `×` in the search box is the only way back.
- **Observed:** on `/shared-by-me` at 1280, "Nothing shared yet. Share an
  artifact from its share panel." is wrapped into a ~210px column. It names a
  panel without saying where that panel is, and gives no link.
- **Source:** `gallerygrid.tmpl` line 19, `p.empty`.
- **Fix:** add a shared `emptyState` partial with an icon, a heading, one line of
  explanation and an action:
  - search: "Clear search"
  - filters: "Clear filters"
  - Shared by me: "Open your library", plus a hint pointing at the share button
    in the viewer pill
  - Shared with me: say who can share with you

  Remove the narrow max-width.

### 3. Header and title patterns differ on every page (consistency, navigation)

There are three header shapes:

- Library spaces: logo, search, and admin, add and settings actions.
- `/new` and `/settings`: "← Gallery" plus a title. On phone it becomes an
  unlabeled ← arrow.
- Editor: "← My library" with its label kept on phone, plus View and Save.

The same destination is called "Gallery", "Library" and "My library". The
Library page's `<h1>` is the sr-only "Gallery". `/shared` and `/shared-by-me`
have an sr-only `h1` and a visible `h2` with identical text.

`/new` and `/settings` repeat their title as a second hero heading ("Three ways
in.", "Settings.").

**Fix:**

- Pick one name ("Library") and one back pattern: an arrow with a visible label
  at every width, or a 44px button with an `aria-label` consistently.
- Give Library a visible heading like the shared spaces have, and make that
  visible heading the `h1`.
- Drop either the header title or the hero heading on `/new` and `/settings`.

### 4. Primary actions are mis-weighted (hierarchy)

- **Gallery header, desktop (observed):**
  - The admin-only "Accounts" link is the only labelled button, 121px wide.
  - "Add artifact", the main creation action, is a 36px "+" icon.
  - Settings is a generic person-circle icon.

  **Fix:** label Add ("New artifact") as the primary button. Move Accounts into
  Settings, where it already lives (`profile.tmpl:61`), or show it as an
  icon-only tertiary control.
- **Editor, observed:** "Save changes" stays fully enabled at opacity 1 while the
  page says "All changes saved" (`#save-button.disabled === false`). On phone it
  is an icon-only checkmark. **Fix:** make it visibly quiet or disabled when
  clean and primary when dirty, and keep a "Save" label on phone.
- **Editor security summary, observed:** the row reads "0 origins · downloads:
  ask first · clipboard: ask first · …" with no section name, and wraps to 3
  lines on phone. **Fix:** give it a title ("Network & permissions") with a
  short summary such as "No network access · 6 capabilities ask first".

### 5. Card and viewer actions are hidden or icon-only (navigation, a11y)

- **Card menu, desktop (observed):** the card's `⋮` trigger measures 1×20px
  until hover or focus, and 22×22 when shown. Nothing indicates that cards have
  actions. Phone shows it, at 44px.
- **Viewer pill (observed):** five icon-only controls, measured at 30×30 with a
  22×30 drag handle on desktop and 36×36 with a 27×36 handle on phone, named by
  tooltip only. The pill floats over the artifact's content at bottom-left.
- **Delete (observed):** it is reachable only at the bottom of the editor. On
  phone it is a full-width red button directly under "Return to My library".
- **Fix:**
  - Keep the card trigger visible at reduced opacity and at least 32px, with a
    44px hit area on touch.
  - Make pill buttons at least 36px on desktop and 44px on coarse pointers.
  - Separate Delete from navigation with space or a divider, and use a
    non-full-width danger style.

### 6. Text resizing ignores the user's font preference (a11y)

*(source)* The gallery CSS sets 234 `font-size` values in `px` against 9 in
`rem`. A larger browser default font size therefore leaves almost all UI text
unchanged. Browser zoom still works, so WCAG 1.4.4 is technically met. Observed:
overriding the root size to 200% produced no layout change.

**Fix:** move the type scale in `tokens.css` to `rem`, starting with body,
labels and the 11–12px meta text (`index.css:170,186,205`).

## Lesser issues

- **Phone `/new` (observed):** Cancel is stacked *above* "Start building". Put
  the primary action first, or place them side by side.
- **Phone `/new` (observed):** Title placeholders ("Session timer" in Agent
  mode, "Untitled" in Paste mode) are close enough to value colour to read as
  filled values. They are also inconsistent between modes.
- **320px (observed):** the bottom tab "Shared with me" wraps to two lines while
  the other tabs use one, so the labels sit at different baselines.
- **320px, light capture (observed, unconfirmed):** the gallery's widget tiles
  rendered blank after 1.5s while 390px rendered them. This may be lazy-load
  timing. If it is real, it is an empty-looking gallery on small phones.
- **Settings (observed):**
  - "Accounts" and "Return to My library" are underlined text inside button
    chrome, unlike every other `.btn`.
  - Only Storage and Delete headings carry icons.
  - The subtitle mentions "connected agents", but no such section is shown.
- **Settings (observed):** the disabled Delete account button shows its reason
  beside it at 12px. This is fine, but the button reads as active pink rather
  than disabled.

## Untested

These were not exercised:

- the tag/collection filter rail and phone filter sheet (no fixture tags)
- populated shared spaces
- the share panel
- duplicate submit success and error
- URL-ingest error paths
- the versions panel and version viewer
- editor unsaved-changes dialog
- screen-reader output, measured beyond static checks
