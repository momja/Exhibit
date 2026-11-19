# Round 26 — independent Opus visual review (target 9, confirmation round)

Reviewer: **claude-opus-5-5**, 2026-11-05, live session about 01:54–02:10 UTC.

## Application under review

- Frozen application **d30dbbe92f9f5535a33f59be25f7c9f9c4be4511**.
- Served assets **e29fcce0e122**, confirmed from live versioned asset URLs (`/assets/v/e29fcce0e122/…`) on Library, Shared with me, the editor and the viewer.
- Base: origin/main **44def51a**.
- HEAD at exit: **b28055d0** (evidence only).
- `git diff d30dbbe9 HEAD -- internal web e2e` touches exactly one file, `internal/api/gallery_test.go` (+5/−2). This is the validation-only expectation update in a787bd5e. There is no other runtime, template, CSS, JS or e2e difference, and the working tree is clean under those paths.

## What I read

**In full:**
- `visual-brief.md` and `nine-brief.md`
- both R25 reports and both R24 reports
- `visual-round-25-changes.md`
- `nine-validation.md`

**Read without line-by-line verification:**
- change notes 20–24 (read in full)
- rounds 5–23 and change notes 6–19, through the brief's cumulative summary of findings, responses and objections. I did not re-read every older report line by line, so treat that history as summarised, not re-audited.

**Not read:**
- `round-26-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md`, `nine-opus-resume-prompt.md`
- the ticket and any parent aggregate

My scores are fresh. They were not shaped by a peer rating or the target. Tests did not decide any score.

## Verdict: PASS for Opus. All twelve fresh scores are 9.

On this unchanged application I found no material visual weakness on any surface I inspected, in either scheme or at any of the four widths. What remains is trivial finishing, listed below.

This report is one model's result. It does not by itself make a second joint pass, and I make no claim that the council is complete.

## Twelve scores

| Category | Light | Dark | Weakest material surface I observed, and why it still earns 9 |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | **Hierarchy holds everywhere I looked:** 26/650 page titles, 16/600 sheet titles, 14/600 group heads, 13/600 desktop captions (14/600 on phone) and 13/19.5 helpers (14/21 on phone).<br>**Network & permissions** matches its neighbours at 1280, 768 and 320 in both schemes.<br>**Weakest spot:** the state inspector's data-key labels (`id`, `date`, `km`, `note`) are 13px regular rather than caption weight. That is defensible, because they show user data keys rather than app captions. |
| Composition and spacing | **9** | **9** | **Rhythm and grouping hold:**<br>• Versions dates share a column.<br>• Transfer notes pair with their own buttons.<br>• Erase sits behind a hairline.<br>• The permission columns are explained by "No origins approved."<br>• At 320 the origin field, caption and first-line caret are correct.<br>**Weakest spot (trivial):** in Share, the gap from a section head to its first content differs. It is **8px** under Link access, **18px** under People and **12px** under Saved data (measured at 640×360 light; the same is visible at 1280 light). It reads as a slight rhythm wobble, not a structural problem. |
| Color harmony | **9** | **9** | **One palette across both schemes:** slate neutrals with one blue accent.<br>• Dark fields, selects and inputs all compute `rgb(26,29,33)` with an `rgb(100,112,131)` border, identical between permissions and Tags.<br>• Completion uses the app panel `rgb(32,35,40)` with a `rgb(28,41,64)` selected row.<br>• Danger is a quiet coral outline.<br>• Default tiles and presets are muted.<br>**User-tool exclusion:** the light Run Log tool body seen under the dark preference is user content and is excluded. |
| Contrast and emphasis | **9** | **9** | **One filled primary leads each task area:** Create and add tag, Save state, Save, Duplicate artifact and Start building.<br>• The allowlist and Tags Add buttons are secondary.<br>• The current Find match is `rgb(219,231,251)` against 8% accent echoes, so it clearly leads.<br>**Weakest spot:** the disabled Regenerate on the widget panel is a 55%-opacity filled primary, and its reason ("Agent support is off…") appears only in a title tooltip. This is specific to a scratch server without `pi`. It is legible and clearly inactive. |
| Component visual consistency | **9** | **9** | **Shared parts agree:**<br>• Sheets (Share, Duplicate, Edit tag, Filters) share their header, scrolling body and footer.<br>• Card and viewer menus use 44px phone items.<br>• Square inspector utilities are 36px on desktop and 44px on phone.<br>• Find, Go and completion use app colours.<br>**Weakest spots (minor):**<br>• Find's Next is filled, but Go's "Go" is an outline.<br>• On the desktop state panel, controls in field rows are 44px while standalone actions (Save state, Cancel, Export) are 36px. That is an intentional compact/row split, but you can see it. |
| Responsive visual polish | **9** | **9** | **Short Share sheets open at the top with Close focused, scroll fully by real wheel, and reopen at 0. Measured in both schemes:**<br>• 320×640: 477 / 1124px body, scrolls to 647, reopens at 0 (dark and light).<br>• 390×640: 477 / 1061px, scrolls to 584 in light and in dark, reopens at 0 in dark.<br>• 640×360: 172 / 947px, opens at 0 in dark and light, scrolled by wheel in light.<br>**No document overflow** at any of 1280 / 768 / 390 / 320.<br>**Phone tabs** are balanced, and the Search active state is clear. |

## Coverage by width and actual scheme

The scheme was set only with `preview_set_appearance`, never by a root class. `matchMedia('(prefers-color-scheme: dark)')` was read after each switch.

| Width | Light (`matchMedia` false) | Dark (`matchMedia` true) |
| --- | --- | --- |
| 1280 | • Library<br>• editor widget / Versions / source<br>• Find / Replace with Next<br>• Go to line<br>• viewer Share full sheet | • Library with card receipt<br>• editor Tags / permissions / Stored state (top and actions) / Stored assets / widget / Versions<br>• Edit tag sheet<br>• completion |
| 768 | • Shared with me<br>• editor permissions and state | • Shared by me via an actual rail click: rows 44px at y76 / 124 / 172 / 220, `aria-current` on Shared by me |
| 390 | • Search miss<br>• Settings<br>• viewer More menu<br>• Filters sheet<br>• Share 390×640 (first open and scroll) | • Search "run" results<br>• Add (Agent)<br>• viewer receipt<br>• card menu → Duplicate (cancelled)<br>• Share 390×640 (reopen and scroll) |
| 320 | • Share 320×640 (reopen at 0)<br>• Add (Paste) | • Editor permissions: placeholder `https://example.org` in a 184px field; caret on the first line<br>• Share 320×640 (first open and wheel scroll)<br>• Search "packing" (incoming card with owner chip) |
| 640×360 | Share (open and wheel scroll) | Share (open) |

## Prior repairs re-verified live

**Permissions grammar**
- 14/600 heads.
- Shared 13/600 captions.
- 44px selects identical in style to the Tags selects.
- Secondary "+ Add".
- Empty helper.
- Complete example at 320.
- First-line caret.

**Stored state**
- "· 1 key" count after load.
- Save state is the only filled action.
- Export and Import each sit with their own note.
- Hairline, then outline Erase.
- Assets paragraphs separated by 12px.
- Widget help sits below the tile.

**Versions**
- Dates aligned in one column.
- View / Restore on one line.
- Current badge tinted.

**Card receipt (1280 dark)**
- 320×323 at y191, flipped above the trigger.
- Document height stayed 800.

**Share**
- Opens at the top.
- Lower radios stay inside their cards.
- Done stays fixed.
- Revoke is aligned.

**Edit tag**
- Aligned colour well and hex field.
- Muted presets, with the current one marked.
- Delete / Cancel / Save split.

**Duplicate**
- 18px checkboxes with helper text.
- Full-width primary and Cancel on phone.

**Find**
- After Next, the selected match is `rgb(219,231,251)` and the echoes are 8% accent.

**Go to line**
- This time the panel opened and was observed: label, field, Go and Close.

## Investigated and dismissed

- **Done accent outline at phone sizes** (Share at 320 and 390, Filters at 390). The Done button showed an accent border and ink. Source markup is plain `btn btn-sec`, and at 640×360 it computes neutral. The accent was `:hover` from the mouse pointer left at the previous click position. It is not a styling inconsistency.
- **Card monogram tiles in my first load.** These appeared only because I first opened `localhost:18280`, where the render origin's `frame-ancestors` refuses framing. Every observation in this report is from `t3code:18280`, where the widgets render.
- **Blue strip under the 640×360 Share sheet.** This was the user's tool content, dimmed behind the backdrop. Excluded.

## Minor opportunities (not decisive)

- **Share section heads:** make the head-to-content gap equal under Link access, People and Saved data (currently 8 / 18 / 12).
- **Find vs Go:** Find's Next is filled; Go's "Go" is an outline. Either make Go filled or make Next secondary.
- **Disabled Regenerate:** add a short visible reason under the disabled button, instead of a tooltip only.
- **Retained older judgements,** reassessed and still trivial:
  - unselected native dark radios are grey discs;
  - the Current badge sits about 4px inside the action column's edge;
  - desktop Replace / Replace all end before Close;
  - Add segments are heavy (700);
  - uppercase fact eyebrows versus sentence-case captions;
  - the generous 240px rail;
  - quiet source echoes;
  - the short landscape Share body (172px) is small but intentional;
  - the repeated public-link sentence.

## Coverage limits

**Session and data handling**
- I used only my own tab, `tab_1o_0d23e38c-d413-4e76-b0eb-59687a206343`.
  - It was opened after `preview_status` with `reuseExistingTab:false, open:false`.
  - Its exact id was passed on every call, and it is now closed.
  - I was already signed in.
- No saves, creates, deletes, grants, link replacements, data-mode changes, transfers, erases, builds, tests, commits, capture writes or subdelegation.
- Temporary local interactions, all restored:
  - a Find query, cleared through the field and closed;
  - Go to line, opened and dismissed;
  - completion, opened and dismissed without accepting. "All changes saved" was confirmed afterwards.
- Dialogs and menus were dismissed unchanged. **I wrote only this file.**

**Evidence**
- Evidence is live screenshots (saved by the preview tool under its own browser-artifacts directory) plus computed geometry and styles. No prior captures were used.
- Source was consulted only once, to identify the Done button's class.

**Not exercised this round**
- Populated allowlist, referenced or blocked origin rows (I did not create a draft row this round).
- Populated asset rows, and arbitrary long data.
- Enlarged text.
- Invalid, dirty or post-save states.
- Completion detail panes and special-character or bracket variants.
- Hosted identity.
- Replace and Replace all actions.
- Desktop selection paint outside Find.
- 768 Library card grid in light (only shared spaces at 768).
- 1280 Search.
- Settings in dark and the lower part of Settings.
- URL ingest mode.

**Engine limits**
- Single Chromium engine with emulated viewports and a fine pointer.
- No physical phone or keyboard, alternate engine or accessibility certification.
- User-authored tool and widget designs are excluded.

## Scores (light / dark)

- Typography: 9 / 9
- Composition and spacing: 9 / 9
- Color harmony: 9 / 9
- Contrast and emphasis: 9 / 9
- Component visual consistency: 9 / 9
- Responsive visual polish: 9 / 9
