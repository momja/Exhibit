# Round 17: Opus visual review (fresh 9/10 baseline)

Reviewer: **claude-opus-5-5**, independent. 2026-11-04.

**Inputs**

- **Read:** `nine-brief.md`, `visual-brief.md`, and both round-16 reports (as prior context only).
- **Not read:** the round-17 Sol report, `nine-scores.json`, `nine-summary.md`, `nine-resume.md` and the ticket.

**Application and environment**

- **Application:** frozen `01109602`. Running asset version `150ec936001f`, confirmed from the live script URLs.
- **Tab:** only my own T3 tab, `tab_12_0d23e38c-…`, opened with `reuseExistingTab:false`. I passed its id on every call and closed it at the end.
- **Scheme:** set with `preview_set_appearance`. `matchMedia('(prefers-color-scheme: dark)').matches` read `false` / `true` alongside the measurements.

**Session note.** Partway through, my tab's session lapsed and redirected to `/auth/login`, most likely because of session expiry or another tab. I signed back in as the scratch admin; this created no data.

**Data left unchanged**

- No saves, creates, deletes, grants, replacements or data-mode changes.
- Share, Duplicate, Edit tag and Find were each opened and dismissed with Escape.
- Run Log still reads "All changes saved".
- The Paste method was switched back to Agent; I typed nothing into it.

## Scores

| Category | Light | Dark |
|---|---|---|
| Typography | 8 | 8 |
| Composition and spacing | 8 | 8 |
| Color harmony | 8 | 8.5 |
| Contrast and emphasis | 8 | 8 |
| Component visual consistency | 8 | 8 |
| Responsive visual polish | 8 | 8 |

**Verdict for the 9 target: FAIL.** No category reaches 9 in either scheme. Against the old 8 threshold the interface still passes: I found no regression and nothing broken.

**Why there are no 9s.** A 9 means intentional and carefully finished on the material surfaces, with only trivial refinements left. The library, cards and phone tabs come close. The secondary surfaces do not:

- forms and dialogs
- the Share sheet
- Edit tag
- Duplicate
- version history
- page headers

They still carry visible idiom drift: different label styles, four control heights, two neutral color families, mixed footer alignments, and one footer that breaks at 320. None of these is severe on its own. Together they read as "well-maintained utility", not "excellent".

This is a single-model result. It makes no council claim.

## Earlier repairs re-checked live (holding)

- **Desktop rail.** At 768 light the three rows are 215×44 at y=76/124/172, each label has one line box, and document overflow is 0. At 1280 light and dark the labels fit on one line, and the counts sit separately at x≈211.
- **Phone tabs.** At 390 light (Library and Search) there are four equal tabs, the active one gets a fill and a bold accent label, and overflow is 0.
- **Native Find at 390 light.** Flat 44px controls; Next is the filled primary; there are 44px option targets. Editor source was unchanged afterwards.
- **Earlier dark fixes still hold** in the Library, Shared with me, the card menu, the Duplicate scrim and editor syntax:
  - quiet structural borders
  - restrained default tiles: muted olive "UC"/"SP"
  - integrated dark source
- **Not re-verified this round:** the selection paint matrix. It is unchanged code and both models verified it in rounds 13–16.

## Category assessments and what would earn a 9

### 1. Typography: 8 / 8

**Why it earns 8**

- The base is a clean system-ui scale.
- Gallery h1 is 26px/650 and the subtitle 14px muted; card titles are 14px/600; rail items are 14px; section heads are 11px/600 uppercase.
- The source editor has full syntax colour in both schemes.

**What holds it below 9: there are at least four label idioms for the same job.**

| Where | Label style |
|---|---|
| `/new`, all widths | UPPERCASE, bold, tracked ("TITLE optional", "DESCRIPTION") |
| Editor (`/edit`) and Duplicate dialog | 12px regular, sentence case ("Artifact title", "Title"), `#656565` |
| Share sheet | 12px/700 sentence-case section heads ("Link access", "People") in neutral `#656565`, sitting *above 14px body copy*. The section heading is smaller than the text it introduces. |
| Settings, all widths | UPPERCASE micro-labels for facts ("SIGNED IN AS"), with the values at different weights: `admin` bold, Storage values regular |

**Other typography issues**

- **Helper text has no rhythm token.**
  - The Versions explanation at 320 dark is 12px with `line-height: normal` (about 14px), so it is visibly cramped.
  - Share explanations are 14px on a 21px line.
  - The Duplicate option title and its helper are 13px and 12px in the *same* colour (`rgb(171,176,184)` dark), so the option name does not lead.
- **Placeholders.** Title, Search, People, Find and Replace are italic; Description is upright.
- **Page titles vary for no reason.** Gallery h1 is 26px, the editor h1 24px/700, Settings "Your account" 28px/650 on phone. `/new` stacks a header title, "Add artifact", over a second display heading, "Three ways in.", so the page has two competing h1-level titles.

**To earn 9**

1. Define one field-label style: 12–13px, 600, sentence case, `--color-label`. Use it in `/new`, the editor, Duplicate, Edit tag, Share and Find.
2. Keep UPPERCASE tracked text only for *navigation/section eyebrows*: rail Collections/Tags and Settings fact keys.
3. Make Share's section heads at least the body size: 14px/600 in ink, not grey.
4. Add one helper-text token (13px / 1.45, muted) and apply it to the Versions, Duplicate, Share and Storage notes.
5. Give option titles in Duplicate the ink colour at 14px/500 so they lead their helpers.
6. Use upright placeholders everywhere.
7. Use one page-title size (26px/650) for the gallery, Settings, editor and `/new`. Make the header title *or* the display heading the h1, not both.

### 2. Composition and spacing: 8 / 8

**Why it earns 8**

- Gallery grids are even: four 236px columns at 1280 and two 232px columns at 768.
- The rail, cards and Settings cards have consistent gutters.
- The phone Library is calm.

**What holds it below 9**

- **Edit tag at 320 dark** (`/edit` → Tags → pencil on Health). The footer wraps to Delete (filled, left) · Cancel (right), with **Save alone on a second row at the left**. This is the clearest unfinished composition in the app.
- **Card footer anchors (1280/768, both schemes).** The title's first line is centred around y≈317, the tag dot sits at 313, and the kebab glyph spans 316–334 (centre 325). That gives three vertical anchors in a 62px row.
- **The incoming card at 768 dark** (`/shared`) leaves a large dead band between the title and the owner row.
- **Space below the header differs by page.** At 390 the h1 sits about 29px below the header on the Library but **12px** below it on Settings and `/new`, where "Three ways in." and "Your account" crowd the header.
- **Five page-header idioms:**
  1. global header with Search (gallery)
  2. Search page header with no field: a lone logo on desktop `/search?q=packing`
  3. "← Library  Settings" on desktop, but an icon-only "← Settings" on phone
  4. editor: "← Library" with labelled buttons on desktop, unlabelled eye/check squares on phone
  5. viewer floating pill
- **Header action drift.** On phone Search, the "+" button sits 4px left of where it sits on Library (x=274 vs 278).
- **Share sheet at 390 light.**
  - Three long paragraphs, repeating "connected accounts" three times.
  - A 6-line Link explanation.
  - Replace link is 32px under a 44px URL/Copy pair.
  - Revoke ends 15px inside the section edge.
  - The Done footer scrolls away rather than staying pinned.
- **Desktop `/search`.** No rail item is active, so the page has no "you are here".

**To earn 9**

1. Make all dialog footers one rule: right-aligned, with the primary rightmost. Put destructive actions at the far left only as an outlined/ghost-danger button. At ≤400px, stack full-width in the order primary, secondary, destructive. Apply this to Edit tag, Duplicate and Share.
2. Card footer: put the title, tag dot, posture glyph and kebab on one row and centre them on the title's first line (`align-items: first baseline`, or offset the kebab by -8px). Pin the owner row on incoming cards directly under the title.
3. Use one page-top spacing token (24px desktop, 28px phone) on every page below the header.
4. Use one secondary-page header: back arrow + "Library", page title, right-side actions. Keep it identical on desktop and phone; phone icon buttons may stay icon-only but keep the same order.
5. Share:
   - Cut the copy to one sentence per section and move the connected-accounts note to a single footnote.
   - Make Replace link and Revoke the same 44px height on phone, flush right.
   - Use a sticky footer for Done.
6. Give `/search` on desktop an active marker: a rail "Search" item, or a results count beside the heading.

### 3. Color harmony: 8 light / 8.5 dark

**Why**

- The dark palette is a single cool-slate ramp (`#16181c` ground, `#202328` panel, `#e8eaed` ink, `#9aa0a9` muted, `#363b42` hairlines) with a light-blue accent. It is coherent, and the default tiles are restrained.
- The light palette mixes **two neutral families**, visible side by side:
  - **Blue-slate:** ink `#17202e`, label `#626975`, border `#d3d9e2`, card border `#dce0e6`, ground `#f5f6f8`, slate-*.
  - **Pure grey:** `--color-muted` / `--color-faint` / `--color-placeholder` `#656565`, `--color-ink-mid` `#555`, hairlines `#e0e0e0` / `#d9d9d9`, hover fill `#e8e8e8`.
- Places where the two families meet:
  - **Share at 390 light:** body text `rgb(85,85,85)` and section heads `rgb(101,101,101)` next to `rgb(23,32,46)` ink.
  - **Rail hover on "Daily tools" at 768 light:** a flat grey `#e8e8e8` pill beside the blue-tinted active "Library" pill.
  - **Editor field labels:** `#656565`.
- **Saturated colours outside the palette:**
  - Danger is pure `#c00` in light, and Edit tag's Delete becomes a large salmon fill in dark.
  - The tag swatches are saturated preset hues (`#ef4444`, `#f59e0b`, `#10b981`, `#3b82f6`…) while the tags actually in use are muted (Health `#377a64`). Health's own colour matches no swatch and no swatch shows as selected.
  - In dark, the native input selection in Duplicate is the browser's saturated blue over a light-blue focus ring.

**To earn 9**

1. Re-base every light grey token onto the slate ramp: muted about `#5f6773`, hairline about `#e1e5eb`, hover about `#eceff4`.
2. Tone danger to a brand-compatible red (about `#b42318` light; dark already `#f2726a`), used as text and outline at rest.
3. Replace the tag presets with a muted set harmonised with the existing tag dots, and show the current colour as selected.
4. Set `::selection` in app inputs to the editor's selection tint.

Dark is closer: it needs only the danger fill and the swatch set.

### 4. Contrast and emphasis: 8 / 8

**Why it earns 8**

- Primary actions are clear: filled blue Add artifact, Start building and Next.
- Text contrast is good and the source selection leads.
- Settings' danger zone is appropriately quiet.

**What holds it below 9**

- **Tags panel at 320 dark.** Two filled primaries compete in one section ("+ Add" and "+ Create and add tag").
- **Edit tag at 320 dark.** The *destructive* action is the loudest element in the dialog: a solid salmon Delete, louder than the blue Save.
- **Versions.** The "Current" badge is filled with the *ground* colour (`rgb(22,24,28)` on a `rgb(32,35,40)` panel in dark), so it reads as a hole rather than a status.
- **Duplicate.** The option names and their helpers have identical colour.
- **Native checkboxes.** Share and Duplicate use native 13px checkboxes, and in dark the unchecked box is a pale grey square that is the brightest thing in the dialog body.
- **Light `/new`.** The selected method has only a 1px accent border; dark adds a ring.
- **Share at 390 light.** The muted grey section heads are weaker than the body they introduce.

**To earn 9**

1. Allow one filled primary per region. In the Tags panel, "Add" becomes secondary, or the two forms become tabs.
2. Make destructive buttons outline/ghost-danger at rest everywhere: Edit tag, the editor footer and Settings.
3. Give the "Current" badge the accent tint (`--color-accent-tint` with accent ink).
4. Style checkboxes as 18px app controls with an accent fill.
5. Give the selected method a segmented-control fill: accent-tint background plus 2px accent border, in both schemes.

### 5. Component visual consistency: 8 / 8

**Why it earns 8**

- Cards, menus, the rail, phone tabs and source panels share surfaces and radii.
- The native CodeMirror chrome is integrated.

**What holds it below 9: control heights come in four tiers, chosen per surface rather than per role.**

| Height | Where |
|---|---|
| 32px | Replace link, Revoke, desktop Find |
| 36px | Duplicate field and buttons, editor title field, Settings buttons, Clear search, Sign in |
| 40px | Header buttons, desktop Search field |
| 44px | Share URL/Copy/Done, phone fields and tabs |

**Other inconsistencies**

- **Dialog footers:** Duplicate is left-aligned "Cancel · Duplicate artifact"; Share has Done right-aligned; Edit tag has a split/wrapped footer.
- **Close affordances:** Find's borderless × versus Go-to-line's bordered Close. Share has no header close button, only a scrolled-away Done.
- **Labels:** see Typography.
- **Checkboxes:** native in Share and Duplicate.
- **Dialog presentation on phone:** Filter is a bottom sheet, but Edit tag and Duplicate are centred cards at 320.
- **Sign-in page (`/auth/login`, 1280 dark, seen after the session lapse):** an unheaded card with a centred small prompt over left-aligned labels and 36px fields. It is the least designed app surface.

**To earn 9**

1. Use a two-step control scale: **compact 36px** for desktop inline and secondary actions, **default 44px** for all phone controls and all dialog fields and actions. Map every button and field to one of the two by role.
2. One modal shell: header with title and × close, body, sticky footer under the shared footer rule.
3. Present modals as bottom sheets at ≤480px, matching Filter.
4. Use one close style: a 32/44px bordered icon button.
5. Bring the sign-in page onto the same card/label/field system.

### 6. Responsive visual polish: 8 / 8

**Why it earns 8**

- No overflow anywhere I measured.
- Phone tabs are excellent.
- The 768 two-column grid is comfortable.
- The editor reflows sanely, and source stays visible with Find open.

**What holds it below 9: specific narrow-width breakages that a 9 would not tolerate**

- **Edit tag footer wraps at 320** (above).
- **Find panel at 390 light.**
  - The Find field is 213px, leaving a dead gap before ×.
  - "Match case" and "Whole word" wrap to two lines beside a one-line "Regex".
- **Truncation at 320.**
  - The existing-tag select truncates to "Choose an existing ta" (320 dark).
  - The Paste prompt clips to "Paste HTML artifact source her…" (320 dark).
- **Search miss at 390 light.**
  - "tag." is orphaned on its own line.
  - Clear search is 126×36 under a 44px field.
- **"Return to Library" at 320 dark:** full-width, but the label is left-aligned (`text-align:start`).
- **Phone header shifts** between pages (filter button present or absent, so the "+" moves 4px), and the phone editor header has unlabelled eye/check squares.

**To earn 9**

1. Stack the dialog footer at narrow widths (Composition 1).
2. Find at ≤480px:
   - Put field and × on one row, with the field flexing to fill.
   - Lay out the three options as a single row of equal chips, or a 2+1 grid with nowrap labels ("Case", "Word", "Regex", or with tooltips).
3. Use shorter placeholders and prompts: "Choose a tag…", "Paste HTML here".
4. Search miss: use `text-wrap: balance` on the help text and make Clear search 44px on phone.
5. Centre labels in full-width buttons.
6. Give phone header actions fixed slots so they do not shift between pages.

## 3–5 coherent change groups (priority order)

**Necessary for 9**

1. **Form and dialog system pass.**
   - One label style, helper-text token, upright placeholder and 18px app checkbox.
   - A two-step control height scale (36 compact / 44 default).
   - One modal shell (header close, sticky footer, footer alignment rule, bottom sheet ≤480px).
   - Apply to Duplicate, Edit tag, Share, Find/Go and `/new`.
   - This single group addresses most of what holds Typography, Component, Responsive and part of Composition below 9.
2. **Light neutral unification and calmer accents.**
   - Re-base the light greys on the slate ramp.
   - Brand-toned outlined danger at rest.
   - Muted tag-swatch presets with the current colour selected.
   - App `::selection` in inputs.
   - Accent-tint "Current" badge.
   - One filled primary per region (Tags panel).
   - This is the Color harmony light item and most of the Contrast items.
3. **Share sheet rewrite for density.**
   - One sentence per section and a single connected-accounts footnote.
   - Section heads at body size in ink.
   - 44px secondary actions flush right on phone.
   - Sticky Done.
   - Share is the weakest high-traffic dialog and the most visible "dense" surface.
4. **Page frame consistency.**
   - One secondary-page header (back + title + fixed action slots).
   - One page-top spacing token.
   - One page-title size; remove the double heading on `/new`.
   - An active location for desktop `/search`.
   - Card footer row alignment (kebab and dot centred on the title's first line; owner row pinned on incoming cards).

**Optional (taste, not required for 9)**

5. A fluid rail at 768 (for example, `clamp(200px, 26vw, 240px)`).
6. A designed sign-in card.
7. A segmented control with icons for the Add methods.
8. A result count on Search.

## Minor observations carried (reassessed)

**Still present and folded into the groups above**

- Share density and 32px secondary actions; Revoke inset 15px.
- Edit tag footer and solid Delete.
- Bold `admin` vs regular Storage values.
- Italic placeholders.
- Paste prompt and tag-select truncation at 320.
- Find gap and wrapped options at 390; borderless Find close vs bordered Go close.
- Left-aligned Return to Library.
- Thin light Add-method selection.
- Browser-blue input selection.
- Card kebab sitting low.
- Duplicate idioms and checkboxes.
- Search-miss orphan and 36px Clear.

**Not re-measured this round**

- Completion popup centring and edge inset.
- Go-to-line.
- Folding and bracket variants.
- The very faint secondary find tint and the 25% active band: accepted earlier, not re-judged.

## Coverage and limits

**Live, with explicit scheme and matchMedia proof**

| Width | Light | Dark |
|---|---|---|
| 1280 | Library, Search `?q=packing`, editor `/edit`, Settings | Library, card menu, Duplicate, sign-in page |
| 768 | Library | `/shared` |
| 390 | Library, Search miss, viewer → Share (top and bottom, measured), editor Find panel | Settings (full page) |
| 320 | `/new` Agent | `/new` Paste, editor Tags panel, Edit tag dialog, Versions and footer |

**Not covered live this round**

- 768 dark Library and 768 light Shared.
- 320 light Share and 390 dark Share.
- Admin users page.
- Filter sheet.
- Completion, Go-to-line and folding.
- Viewer posture popover and More menu.
- Shared by me at phone widths.

**Limits**

- One Chromium engine with emulated viewport and scheme; no physical phone or alternate engine.
- Token values were read from live stylesheets, which is source inference supporting the rendered observations.
- I wrote no captures. This report is the only file I wrote.
