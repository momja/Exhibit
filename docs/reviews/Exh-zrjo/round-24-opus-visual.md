# Round 24 — independent Opus visual review (target 9)

Reviewer: **claude-opus-5-5**, 2026-11-05, live session.

**Application under review**

- Frozen application **329c50aa704256090eaa1354eac734d5f04a2339**. Served assets **bc0938a74a48**, confirmed from live versioned asset URLs on the editor and on Library.
- Base: origin/main **44def51a**.
- HEAD at exit was **3c797a16** (evidence only). Read-only `git diff 329c50aa HEAD -- internal web` and the uncommitted diff under `internal web` were both empty.

**What I read**

- `visual-brief.md`, `nine-brief.md`, `visual-round-24-changes.md` and `nine-validation.md`.
- Both R23 reports in full.
- The headers and findings of R20–R22 (both models).
- Change notes 21–23.
- Earlier rounds through the brief's full summary of findings and responses, plus targeted greps of earlier reports. I did not reread all of rounds 5–19 line by line.

**What I did not read**

- `round-24-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md`, `nine-opus-resume-prompt.md`
- the ticket and any parent aggregate

My scores are fresh. They were not shaped by any peer rating. Tests did not decide any score.

**Verdict: FAIL against the all-twelve-at-least-9 target.**

- All three R23 findings from my previous review are repaired live, and the repairs are good:
  - tag-chip actions are 32 / 44 square slots;
  - Add property is 44px, matching its row;
  - phone Create and add tag is full width.
- Share, the transfers, the helper rhythm, history, Find, Search, Settings and Add all hold at a 9 level.
- **However, one material editor section that no earlier round critically examined still uses an older grammar: Network & permissions.** It sits directly under Tags and is always one click away on every artifact's edit page.
  - Its sub-headings are 12/700 while the rest of the form uses 13/600 captions and 14/600 group heads.
  - Its capability labels are 16/400 body text rather than captions.
  - Its allowlist **Add** is a filled primary, while the identical "Add" for Tags two blocks above is a secondary outline.
  - Its empty allowlist leaves a large blank column on desktop.

That keeps three categories at 8.5 in both schemes. This single-model report establishes no joint pass and makes no council-completion claim. The incoming joint streak is zero.

## Twelve scores

| Category | Light | Dark | Weakest material surface (observed live) |
| --- | ---: | ---: | --- |
| Typography | **8.5** | **8.5** | **Network & permissions.**<br>• "Network allowlist" and "Capabilities" are `h3` at **12px / 700**.<br>• On the same page, "Create new tag" is 14/600, and the field captions ("Add existing tag", "Name", "Color") are 13/600 (`field-label`, 19.5 line height).<br>• The six capability names (Downloads … Location) are **16px / 400** plain spans. So the row labels are larger than the headings that introduce them, and larger than every other field label in the editor.<br>• Elsewhere the hierarchy is excellent: 26/650 titles, 13/19.5 helpers on desktop and 14/21 on phone, and readable Find. |
| Composition and spacing | **9** | **9** | Tags, Stored state transfers, Erase, assets, history and Share are well grouped.<br>• Network & permissions at 1280 is the weak spot. The empty allowlist column (x234–616) has nothing below its input, leaving about 260px of empty well beside six capability rows, with no "No origins approved" line.<br>• At phone widths the summary's chevron is centred on a three-line summary rather than its first line.<br>Both are noticeable but local, so they do not drop the category on their own. |
| Color harmony | **9** | **9** | The slate/blue system holds in both schemes:<br>• Dark primaries are light blue with dark ink.<br>• Danger is a quiet coral outline.<br>• The Settings delete card uses a restrained red frame.<br>• Muted presets. |
| Contrast and emphasis | **8.5** | **8.5** | **Two filled primaries compete on the editor.**<br>• At 1280, in both schemes, "Create and add tag" (filled) and the allowlist "Add" (`btn btn-sm`, filled `rgb(35,85,158)` light, light-blue fill dark) are both in view at once.<br>• The analogous Tags "Add" is `btn-sec`.<br>• A local "add to list" action outranks the section's own creation primary and the page's Save changes. Every other section was brought onto "one filled task primary, locals secondary" in R23. |
| Component visual consistency | **8.5** | **8.5** | The same section breaks three shared rules:<br>• heading role (12/700 instead of the caption or group-head tokens);<br>• label role (16/400 instead of 13/600 captions);<br>• button role (filled "Add" with no icon, beside an outline "＋ Add" with an icon in Tags and "＋ Add key" / "＋ Add property" in state).<br>Everything else I checked agrees, including the new 32 / 44 chip slots, the 44 property row, the 36 / 44 utilities and the sheet shells. |
| Responsive visual polish | **9** | **9** | 320 and 390 are clean, with no document overflow:<br>• 44px chip actions and a full-width tag primary.<br>• Share at 320×640 opens at `scrollTop 0` with focus on Close, scrolls fully and keeps Done pinned.<br>• Search miss, Add, and the phone tabs are clean.<br>Small residue: at 320 the allowlist placeholder clips mid-word ("Add origin (https://exar"). |

## Material finding: Network & permissions predates the shared form grammar

This was observed live on `/artifacts/461183aa…/edit` with the section expanded. Nothing was typed into it or saved.

- **1280 light / dark.**
  - Headings "Network allowlist" and "Capabilities" are 12/700. The capability labels are 16/400 (`span.spacer`).
  - The allowlist "Add" is 76×44 and filled. The Tags "Add" is 73×44 `btn-sec` with a plus icon.
  - The allowlist column is empty below its input. `#allowlist-rows` is empty, and no empty-state text is shown.
- **390 dark and 320 light.** The layout stacks correctly. The headings remain 12/700 against 16px labels, and the filled Add stays prominent. The summary wraps to three lines, and the chevron sits beside the middle line. The placeholder is truncated.
- **Earlier coverage.** Only R23 Sol touched the section ("readable heading/caption hierarchy" at 390 dark). No earlier round compared it against the grammar that R18–R24 established elsewhere, so this is new rather than a regression.

**Treatment to earn 9.** Apply existing rules only; no ornament or new feature is needed.

1. Use the 13/600 caption role (or the 14/600 group head used by "Create new tag") for "Network allowlist" and "Capabilities".
2. Render the capability names as the 13/600 field-label role, or as body 14–15 with the label/select pair vertically centred, but consistent with how other labelled selects in the editor read.
3. Make the allowlist "Add" the same `btn-sec` with a plus icon as Tags "Add" and "Add key".
4. Give the empty allowlist a one-line muted helper (for example, "No origins approved yet."), using the existing helper token, so the desktop column is not a void.
5. Optionally, top-align the summary chevron to the first line when the summary wraps, and shorten the phone placeholder (for example, "https://example.com").

## R23 → R24 repairs, verified live

| Repair | Viewport and scheme | Measured |
| --- | --- | --- |
| Tag chip Edit / Remove | 1280 light / dark | 32×32 at x300 / x337. The chip grows to 36px; the glyph stays 12px. |
| Tag chip Edit / Remove | 390 dark | 44×44 at x96 / x145. |
| Create and add tag | 390 dark | 330×44, full column width (x30). |
| Create and add tag | 1280 | 192×44 intrinsic (appropriate on desktop). |
| Add property row | 1280 light | Field, select and button all 44; Add property is 137×44 at y5328, matching Add key 102×44. Add record stays a 36px standalone utility. |
| State actions | 1280 light | Save state is the only filled primary. Export / Import JSON (180×36) sit beside their notes. Erase is a quiet outline behind a hairline. Assets transfers match. |

## Other surfaces, observed live

| Surface | Viewport and scheme | What I saw |
| --- | --- | --- |
| Library | 768 light (`matchMedia` false) | 240px rail, two card columns, 32px kebab slots, aligned title rows. |
| Share | 320×640 dark (`matchMedia` true) | Opens at `scrollTop 0` (477 / 1124), focus on Close. Notice, link, Copy / Replace and the helper are in order. Scrolled to the bottom: the grant row with Revoke, the two saved-data cards with a contained native radio, the public-link helper and the disclosure. Done stays fixed. Closed with Escape. |
| Source Find | 1280 dark | Integrated panel: field fills to Next / Previous / All / Close, then option checkboxes, then Replace / Replace all. Readable syntax. The query was typed locally and then cleared. |
| Search miss ("not-in-this-library") | 320 light | Integrated clear, balanced dashed empty card, Clear search, active Search tab. |
| Settings | 768 dark | Aligned uppercase fact rows, 44px Accounts and Sign out, restrained Delete card with a disabled outline and its reason. |
| Add, Agent mode | 390 light | Selected segment, helper, captions, full-width Start building / Cancel. The darker fill seen in the screenshot was `:hover` from the pointer. |

## Minor opportunities (not decisive)

- Native dark unselected radio is a filled grey disc.
- Uppercase fact eyebrows versus sentence-case captions remain a consistent role split.
- The Add mode segment labels are bold-heavy at phone widths.
- The Settings delete explanation uses body ink, which is defensible as emphasis.
- When collapsed, the "Stored state" summary on phone omitted its "· 1 key" count. Seen once; not investigated.

## Coverage and limits

**Session and data handling**

- I used only my own tab, `tab_1k_0d23e38c-d413-4e76-b0eb-59687a206343`. It was opened after `preview_status` with `reuseExistingTab:false, open:false`, its exact id was passed on every call, and it is now closed. I was already signed in.
- Scheme was set only with `preview_set_appearance`, and I checked `matchMedia('(prefers-color-scheme: dark)')` on each pass. Every width was seen in both schemes on at least one surface:

| Width | Light (`matchMedia` false) | Dark (`matchMedia` true) |
| --- | --- | --- |
| 1280 | Editor | Editor, Find |
| 768 | Library | Settings |
| 390 | Add | Editor |
| 320 | Editor, Search | Share at 320×640 |

- I did not re-test 390×640 or landscape 640×360 Share this round. Prior rounds and the change notes cover them; I treat them as not freshly certified.
- I made no saves, creates, deletes, grants, transfers, erases, link replacements, data-mode changes, builds, tests, commits, capture writes or subdelegation. I typed only into the local CodeMirror Find field, then cleared and closed it. **I wrote only this file.**

**What was observed live, and what was not**

- Evidence is live screenshots plus computed geometry and style. I used no saved captures.
- Not re-exercised this round:
  - Replace / Go / completion and the selection paint;
  - the Edit tag sheet, Duplicate, the card menu and the posture receipt;
  - Filters, the shared spaces, Paste / URL modes and Versions.
  For these I rely on prior rounds and treat them as sampled, not certified.
- Not exercised at all: long or arbitrary data, populated asset rows or allowlist rows, enlarged text, and invalid or dirty states.
- Single Chromium engine with emulated viewports. No physical phone or keyboard, alternate engine, hosted identity or accessibility certification.
- User-authored tool and widget designs are excluded.

## Scores (light / dark)

- Typography: **8.5 / 8.5**
- Composition and spacing: 9 / 9
- Color harmony: 9 / 9
- Contrast and emphasis: **8.5 / 8.5**
- Component visual consistency: **8.5 / 8.5**
- Responsive visual polish: 9 / 9
