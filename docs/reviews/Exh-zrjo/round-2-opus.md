# Exh-zrjo UI council: round 2 (Claude Opus), frozen build baf7a0c1

## Method

- **Screenshots:** I inspected the round-2 set in `docs/screenshots/Exh-zrjo/round-2/`:
  - gallery 1280 light
  - filter miss 390 dark
  - search miss 1280
  - add-error 320 light
  - editor 1280
  - history 320 dark
  - viewer 1280 and 320
  - Settings 390

  These are authenticated app states. The round-1 warning about login-screen captures applies only to the original baseline attempt.
- **Hands-on checks (observed):** I used my own T3 tab (`tab_4_…`), signed in as admin, at 1280 and 320px. I checked:
  - the empty paste submit on `/new`, which is validated client-side and sends no request
  - the viewer control sizes, plus the More menu with click, End, Escape and the return of focus
  - the size and name of each gallery card trigger
  - 200% root font size on the gallery
  - the editor's phone Save control, its CodeMirror name, version button heights and overflow
- **Source:** I read `web/gallery/new.js` (`ingest`, `showFieldError`, `clearFeedback`, `lockIngest`) and `new.tmpl`.
- **Untested:**
  - successful creation, and the double-submit race (the parent's E2E tests cover it)
  - URL-ingest server errors
  - share panel internals
  - the duplicate submit
  - the unsaved-changes dialog
  - screen-reader output beyond the DOM attributes
  - real touch hardware

  Nothing was created, saved or changed.

## Scores

| Category | Score | Rationale |
|---|---:|---|
| Visual hierarchy | **8** | **Observed:** the visible "Your library" h1 and intro, the primary labelled "+ Add artifact", readable 14px bold card titles, a quiet Save when the editor is clean, the "Editing artifact / Run Log" heading, and the titled "Network & permissions · No network access · Ask before…" summary. These fix every round-1 hierarchy finding. **Residual (minor):** `/new` still stacks the "Add artifact" header title over a second hero heading, "Three ways in."; Settings does the same with "Settings" over "Your account". |
| Component consistency | **8** | **Observed:** a shared 56px header, consistent bordered buttons, one empty-state pattern (icon, heading, line, action) for search and filter misses, shared card surfaces, and one name, "Library", for the return destination. The viewer pill uses the same icon set and focus behavior. **Residual (minor):** on phones, `/new` and `/settings` show a bare ← with a hidden "Library" label, while the editor shows "← Library". In Settings, only Storage and Delete account have heading icons. |
| Navigation / discoverability | **8** | **Observed:** the card trigger is always visible (32×36, named "Actions for …"). The viewer shows the tool name and a direct desktop Edit control. Empty states link back to recovery. The space rail and phone tabs are clear. Editor return keeps `return_to`. Accounts as a secondary desktop shortcut is acceptable. |
| Workflow feedback | **7** | **Better (observed):** field-level errors carry `aria-invalid`, focus moves, and the empty states have actions. **Blocker (observed and confirmed in source):** an empty Paste or URL submit shows the inline error *and* "Uploading…" at the same time (`add-error-320-*.png`; live: `#status` = "Uploading…" while `.cm-content` has `aria-invalid=true`). This is the exact workflow that was blocked in round 1, and it now tells the user two contradictory things. |
| Accessibility | **8** | **Observed:** the editors are named ("HTML source", "Artifact source"). Errors use `role=alert` and `aria-describedby`. Text scales with the root font size (h1 26→52px, card title 14→28px at 200%, no overflow). Phone controls are 44px. The viewer controls are 36px on desktop, and its More menu supports End, Escape and focus return. **Residual:** the contradictory status above is also announced, because `#status` is a polite live region. The viewer menu items are 30px tall on desktop, which is acceptable for a fine pointer. |
| Responsive behavior | **8** | **Observed:** no horizontal overflow at 320px on the gallery or editor. Version rows wrap, with the date on its own row and 44px View/Restore buttons. The viewer pill fits at 320px with the name. Filter chips and the empty state fit at 390px dark. Dark mode is coherent. **Minor:** in history at 320px, "2 saved versions" runs straight on from the description paragraph with no spacing, and the "current" marker is unstyled plain text. |

## Blocking finding

**B1: a validation error leaves a stale "Uploading…" status.** Route `/new`, Paste or URL mode, empty submit; element `#status`.

- **Cause:** `ingest()` sets `status.textContent = 'Uploading…'` before validating. The early `return`s after `showFieldError(...)` never clear it.
- **Effect:** sighted users see an error and a progress message together. Screen-reader users hear "Paste an artifact first." as an alert, followed by "Uploading…" from the polite region.
- **Fix:** validate first, and set the progress text only after `payload` is built, immediately before `lockIngest(true)`. Alternatively, clear `status` inside `showFieldError`.
- **Regression test:** add a Node or E2E assertion that `#status` is empty after a blank submit in each mode. The existing capture `add-error-320-*.png` shows the bug, so the tests that passed do not check it.

## Non-blocking refinements

1. Drop the duplicate hero heading on `/new` ("Three ways in.") or the header title, and do the same for Settings ("Your account" under "Settings").
2. On phones, use one back pattern: either show "Library" next to the arrow everywhere, or nowhere.
3. Editor versions panel: put space between the description and the "N saved versions" count, and style "current" as a badge.
4. Settings: give every section heading an icon, or none.

## Verdict

Every round-1 blocker I could verify is fixed. Card actions, CodeMirror names, inline validation with focus, empty-state recovery, gallery context, the "Library" naming, the permission summary, phone history, viewer control size and keyboard behavior, and relative type have all been addressed.

One new, concrete workflow-feedback defect remains (B1), so this round **does not pass**: workflow feedback is 7. The fix is a few lines. If B1 is fixed and nothing regresses, I expect workflow feedback to reach 8 next round, but that has to be re-verified then.
