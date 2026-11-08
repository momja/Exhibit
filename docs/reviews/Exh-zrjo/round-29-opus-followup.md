# Round 29: independent Opus follow-up review

Reviewer: **claude-opus-5-5**, 2026-11-05, about 05:52–06:00 UTC for the live session, plus context reading and supplementary-image review.

- **Frozen runtime:** 575559b5abaadc997a6faf62be3669e4881cfe27
- **Served assets:** `2f37c8103d52`. I confirmed this from versioned `/assets/v/2f37c8103d52/…` URLs on both fixtures.
- **PR273 base:** ab0135c9
- **Populated fixture:** http://t3code:18280, with its separate renderer on :18281
- **Empty fixture:** http://t3code:18380, with its renderer on :18381

**Verdict: FAIL for Opus.** Thirteen of the fourteen scores are 9. **Component visual consistency is 8.5 in both schemes.** The cause is the new destination empty states: four sibling empty or zero-result views use three different typographic grammars. The details and a treatment are below.

This is one model's independent result. It is not a joint result and it is not a council-completion claim. The current joint streak stays at zero regardless.

## Independence and what I read

**Read:**
- `visual-round-29-changes.md`
- `followup-validation.md`
- `round-28-sol-followup-interrupted.md`
- `visual-round-28-changes.md`
- `round-27-sol-followup.md`, as the template for prior findings
- The archived 28/27 brief, with its cumulative findings, responses and unresolved judgments
- The Save-state manifest and images

**Did not read:**
- `round-29-sol-followup.md` or its status file
- `round-29-sol-followup-status.json` or `followup-rating-verification.json`
- `followup-scores.json`, `followup-summary.md`, `followup-resume.md` or the resume prompt
- Any nine-scores, summary or resume file, the ticket, or any parent aggregate or timer

A directory listing showed those filenames exist. I opened none of them. I ran no tests, builds, git diffs or commits. Prior scores and passing tests did not set any rating.

## Fourteen scores

| Category | Light | Dark |
| --- | ---: | ---: |
| Typography | **9** | **9** |
| Composition and spacing | **9** | **9** |
| Color harmony | **9** | **9** |
| Contrast and emphasis | **9** | **9** |
| Component visual consistency | **8.5** | **8.5** |
| Responsive visual polish | **9** | **9** |
| Save discoverability and feedback | **9** | **9** |

### Why component visual consistency is 8.5 (both schemes)

This round requires the empty and zero-result states, and they are the main new surface. They do not share one component grammar. I checked these on the empty fixture at every width and scheme, and the Search miss on the populated fixture:

| View | What it renders |
| --- | --- |
| **Starred empty** | 52px icon tile, then one **bold 16px/600** sentence that mixes headline and instruction ("Keep your favorites close. Choose Star in an artifact's action menu."), then a secondary **44px** "Browse your artifacts" (→ `/search`). The sentence-to-button gap is about 11px, against 12px between the icon and the text. |
| **Shared empty** | Icon tile, then a **bold 16px/600 instruction set as three bold lines at 390px** ("Shared artifacts will appear here. Share one of yours, or ask someone to share with you."), with no action. The color is `rgb(204,208,214)` in dark. |
| **Search miss** | Icon tile, then a **600 headline** ("No artifacts match your search."), a **muted 400 helper** ("Try another title, keyword, or tag."), and a 44px Clear search. This is the best-structured of the set. |
| **Search welcome** | Illustration, then a **12px uppercase accent eyebrow** ("WELCOME TO EXHIBIT") that is the h2 and the only text, with no headline or helper. About 40px of air sits between it and a **48px** filled primary "Create your first artifact". That height differs from the 44px standard used everywhere else. |

Each view is individually clean, and the behavior is now correct (see the next section). Together they read as three authors. Instructions are set entirely in bold, and the welcome hero has a label but no statement. At the "excellent, only trivial refinements" bar, this kind of divergence across one destination family is a material finishing gap, not a trivial one. It is identical in both schemes, so both get 8.5.

Two smaller contributors (not enough on their own to drop below 9):

- The Edit tag sheet's primary Save uses a **check** glyph. The editor header Save uses a **floppy** glyph. "Save" now has two icons in the same editor, and the check is the one the user called confusing.
- The editor's Save failure message starts with a **text "✗" character** plus "Error:". This is seen only in the supplementary 320 captures. It is not a Phosphor icon, and the prefix repeats what the amber "Review" status already says.

### Treatment to earn 9

1. **Give the four empty and zero-result views one partial grammar:**
   - Icon tile or illustration.
   - A short **600 headline**.
   - A **muted 400 helper** at the existing helper size and line-height, with the established 8–12px rhythm.
   - An optional **44px** action.
2. **Apply it to each view:**
   - **Starred:** headline "No starred artifacts yet" or similar. Helper "Choose Star in an artifact's action menu." Keep Browse your artifacts.
   - **Shared:** headline "Nothing shared yet". Helper with the share/ask sentence. Keep it without an action, as PR273 intends.
   - **Search welcome:** keep the illustration and eyebrow if wanted, but add a real headline or one-line helper so the eyebrow is not the only statement. Use the standard 44px primary height.
3. **Save glyphs:** use one Save glyph app-wide (floppy, or no glyph in sheets), and a Phosphor warning icon in the save-failure notice.

No new feature or decoration is needed.

## Required empty-fixture coverage (fresh in this round)

- **Setup:** own incognito-profile tab on :18380, signed in with the scratch account. I did not sign into :18380 in the default profile.
- **Method:** at each of **1280×800, 768×900, 390×844 and 320×740**, in **light and dark**, I used real `preview_set_appearance` and a real tab resize. Then I loaded `/`, `/search` and `/shared` into viewport-sized same-origin frames and measured them. Every frame reported `matchMedia('(prefers-color-scheme: dark)').matches` matching the scheme. Backgrounds were `rgb(245,246,248)` in light and `rgb(22,24,28)` in dark. Horizontal overflow was **0** in all 24 route/width/scheme views.
- **Actions, identical in all 8 width/scheme pairs:**
  - **Search welcome:** **Create your first artifact → `/new`** (249×48).
  - **Empty Starred:** **Browse your artifacts → `/search`** (187×44). There is no Share guidance and no self-link, so the R28 defect is gone.
  - **Empty Shared:** no action, neutral explanation.
- **Rendered views I looked at:**
  - Starred: 1280 light and dark.
  - Search welcome: 1280 light, 768 light, 390 dark, 320 dark.
  - Shared: 390 dark.
- The welcome illustration is composed and on-palette in both schemes. At 320 its left card reaches x≈11 inside a 12px gutter, but stays inside the viewport.

## The three user defects

**1. Selected preset corner: repaired.**
- On the selected cell (Tags creation picker, 1280 light), the outer radius is 8px with a 1px accent border. The badge is 16×16 with radius 4, inset 3px top and right. That gives a 7px inner border radius, and 7−3 = 4 matches the badge radius, so the contours are concentric.
- I enlarged only the color group with a local CSS transform, then reverted it with no persistence. The badge follows the cell's arc cleanly with an even gap, and there is no fringe at the outer corner.
- I saw the same treatment in the dark Edit tag sheet (Health green) and at 320 dark (61×44 cells).
- **New observation:** on desktop the cells are only 34.75×36px and the swatch is 22px. The badge then **covers roughly the upper-right quarter of the swatch dot**. It reads as an intentional overlapping badge rather than broken pixels, but the swatch looks slightly crowded at 1× desktop. On 61–81px phone cells it barely touches the dot. This is a refinement only, for example reducing the dot to about 18px on compact cells or giving the badge a 1px surface-colored ring. It does not change any score.

**2. Tag pill spacing: repaired.**
- The Health label capsule is compact, with the label at 39px text width.
- At 1280 the item is 138×32 with two separate **32×32** utilities. At 320 it is 162×44 with **44×44** utilities, outside the capsule.
- There is no wasted chip padding at any width I saw.

**3. Header Save clarity: repaired.**
- At every width the header shows a floppy glyph, the word "**Save**", and a separate Saved/Unsaved status. The eye control (View artifact) stays distinct.
- **Reversible title test in all 8 width/scheme pairs:** I changed the title to "Run Log — local review" and set it back to "Run Log", never clicking Save. In every pair this gave Unsaved with an enabled filled primary (light `rgb(35,85,158)` with white label, dark `rgb(126,170,240)` with dark label), then Saved with Save disabled. Overflow was 0 throughout.
  - **Save sizes:** 78–80×44 at 1280 and 768 with a 14px label. 78×44 at 390 and 320 with a 17px label.
  - Two cycles at 320 dark and 1280 dark were by real typing. The rest used an input event on the field.
- **Minor polish, not scored down:**
  - Saved and Unsaved use the same muted status color, so the filled Save carries the dirty signal on its own.
  - Toggling the status from Saved (37px) to Unsaved (52px) shifts View artifact about 13px, and Save itself moves 2px (80→78 wide).
  - Reserving the status width would remove both.
- **Supplementary current Save-state evidence:** `followup-save-states-pr273/manifest.json` names `575559b5` and `2f37c8103d52`. I viewed the Saving state at 390 light and the Saved state at 1280 dark. I viewed the failure state at 320 in both schemes: amber "Review", a readable in-view message with "Try again", and an active Save. These are current-source captures from the parent's passing E2E run. I did not run a persistent save, error or retry myself.

## Populated-fixture live coverage (own default-profile tab)

- **Starred** (1280 light): seeded RunLog and ReadingList cards, active rail item, coherent card footers. Hover states were excluded from palette judgments.
- **Search:**
  - 1280 dark results: rail filters, a 6-card grid, and an opaque Run Log card menu (Unstar/Edit/Duplicate).
  - Duplicate sheet: opened and dismissed with Escape. It has consistent captions, helpers and footer.
  - 390 light: a miss for "not-in-this-library", plus integrated Clear and the filter button in the phone header.
  - I did not do a hit query this round.
- **Shared** (390 light): the unified page shows an incoming card with owner attribution and an owned card with a link glyph. Phone tabs are balanced, at 88×46 each.
- **Activity** (1280 dark):
  - The running-agents empty line, the "Coming soon" data-history placeholder and the version rows are legible.
  - **Minor:** every row repeats "Artifact version saved." over nine rows, at 136px each. The "No agents" paragraph spans 992px while the neighboring blocks cap at 880px.
  - Composition stays acceptable. Dropping the repeated filler line would tighten the list.
- **Editor:**
  - Tags creation and Edit tag at 1280 light/dark and 320 dark.
  - Network, Stored state/assets and widget were seen collapsed only. I did not expand them this round.
  - Source was visible, with readable light syntax. I did **not** exercise native Find, Replace, Go or completion, or selection, this round.
- **Share** (viewer control, never `showModal`):
  - **320×640 dark:** body 477/1124. It opens at scrollTop **0** with Close focused. A real container wheel scroll reached lower People (Revoke), the data-mode cards, the public-link rule and the disclosure, with a clean fixed footer. Close and reopen returned to scrollTop **0** with Close focused.
  - **640×360 light:** opens at the top. Chrome takes about 77px top and bottom around a short body. That is the known minor, and it still scrolls.
  - Not repeated this round: 390×640 and other scheme pairs.
  - The dark unselected radio is still a gray filled disc. This is the known minor.

## Coverage limits

Sampled, not certified:
- Native source Find/Replace/Go/completion, actual selection, and expanded Network/state/assets/widget/Versions panels were **not** inspected this round. Earlier Sol reports cover them historically, and I make no current claim.
- Not visited: Add, Settings and the viewer receipt/More menu. I did not check Share 390×640, or 640×360 dark.
- Not exercised: multiple, long or custom tags; populated assets or origins; enlarged text; hosted identity.
- One Chromium engine with real scheme emulation. No physical phone, Safari, software keyboard, screen reader or accessibility certification.
- Validation in `followup-validation.md` (all pass) is context only and did not set scores.

## Restoration and side effects

- **No persistent changes:** no persistent saves, creates, deletes, grants, stars or preference writes. Viewer navigation's automatic opened receipt happened as normal product behavior.
- **Local edits undone:**
  - Title specimens were restored without saving.
  - The Edit tag and Duplicate sheets were dismissed unchanged.
  - The local zoom/transform on the color group was reverted.
- **Final reload of the edit page:**
  - Title "Run Log".
  - Header **Saved**.
  - **Save disabled**.
  - **0 open dialogs**.
- **Tabs:** both of my own tabs are closed. Main was `tab_20_0d23e38c-d413-4e76-b0eb-59687a206343` (default profile) and empty was `tab_21_0d23e38c-d413-4e76-b0eb-59687a206343` (incognito). I passed the exact tab ID on every call and touched no other tab.
- **Files:** this report is the only file I wrote.
