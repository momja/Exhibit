# Round 25 — independent Opus visual review (target 9)

Reviewer: **claude-opus-5-5**, 2026-11-05, live session approximately 01:39–01:45 UTC.

**Application under review**

- Frozen application **d30dbbe92f9f5535a33f59be25f7c9f9c4be4511**. Served assets **e29fcce0e122**, confirmed from live versioned asset URLs on the editor, Library, Search, Settings and Add.
- Base: origin/main **44def51a**.
- HEAD at exit was **b63f3561** (evidence only). A read-only `git diff d30dbbe9 HEAD -- internal web` was empty.

**What I read**

- `visual-brief.md`, `nine-brief.md`, `visual-round-25-changes.md` and `nine-validation.md`.
- Both R24 reports in full, and both R23 reports (Opus in full; Sol's score table and coverage sections).
- `visual-round-24-changes.md` in full.
- Rounds 5–22 and change notes 6–23 through the brief's cumulative summary of findings, responses and objections. I did not reread every older report line by line, so treat that history as summarized, not re-audited.

**What I did not read**

- `round-25-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md`, `nine-opus-resume-prompt.md`
- the ticket and any parent aggregate

My scores are fresh. They were not shaped by any peer rating. Tests did not decide any score.

**Verdict: PASS for Opus. All twelve fresh scores are 9.**

- **The R24 finding is fully repaired.** That finding was the Network & permissions grammar: 12/700 headings, 16/400 labels, a filled local Add, an empty void, a clipped placeholder and a centred caret.
  - It now uses the editor's shared roles, in both schemes and at desktop and phone widths.
  - It also holds in a populated draft state that I created and then removed without saving.
- **No other material surface fell below the excellent bar** in the states I inspected.
- What remains is trivial finishing, listed below.

This single-model report establishes no joint pass and makes no council-completion claim. The incoming joint streak is zero.

## Twelve scores

| Category | Light | Dark | Weakest material surface (observed live) and why it still earns 9 |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | **Network & permissions now matches the editor's type roles.**<br>• At 1280, "Network allowlist" and "Capabilities" are h3 at **14/600/21px**, identical to "Create new tag".<br>• The six capability names are `field-label` at **13/600/19.5**; at 320 they become **14/600** captions like every other phone caption.<br>• The empty note is the 13/19.5 helper.<br>• Elsewhere the hierarchy holds:<br>&nbsp;&nbsp;– 26/650 page titles;<br>&nbsp;&nbsp;– 16/600 sheet titles (Filters and Duplicate are identical);<br>&nbsp;&nbsp;– 14px source completion and readable Find.<br>• Residue (trivial): the Add-method segment labels are heavy at 320. |
| Composition and spacing | **9** | **9** | **The permissions panel now reads as a composed two-column group at 1280.**<br>• Heading → helper → caption → field/Add on the left; heading → six 44px label/select rows on the right.<br>• About 220px stays open below the Add field on desktop. This is a natural consequence of the columns, now explained by "No origins approved."<br>**Phone layout at 320:**<br>• The summary caret is centred on the first line: icon y15–29 against the 21px first line at y11–32.<br>• A populated draft row puts the origin on its own line, with Remove on the next line.<br>**Other surfaces hold:**<br>• Versions dates share x716 in both rows.<br>• The phone stack is Save state → Cancel → Export + note → Import + note → hairline → Erase.<br>• Share opens at the top. |
| Color harmony | **9** | **9** | The slate/blue system holds in both schemes:<br>• Dark Add uses the secondary surface (`rgb(32,35,40)` fill, `rgb(62,70,82)` border); fields sit on `rgb(26,29,33)`.<br>• Completion uses the app's own panel, with a `rgb(28,41,64)` selection.<br>• Danger is a quiet coral outline.<br>• Default tiles and muted presets are restrained. |
| Contrast and emphasis | **9** | **9** | **One filled primary per task area, now including permissions.**<br>• At 1280, both the allowlist Add (`btn-sec`, white fill, slate ink, plus icon) and Tags Add are outlines. "Create and add tag" and Save changes lead.<br>• The empty helper (`rgb(93,104,120)` light / `rgb(154,160,169)` dark) is clearly subordinate to the 14/600 heads.<br>• In Share, the selected data mode and the primary Done lead clearly. |
| Component visual consistency | **9** | **9** | The permissions section now shares:<br>• the group-head token;<br>• the `field-label` caption;<br>• labelled 44px selects;<br>• the 44px secondary "+ Add" (76×44, beside Tags Add 73×44);<br>• the 83×44 outline Remove in draft rows.<br>Sheets (Share, Duplicate, Edit tag and Filters) share their header, footer and title grammar. The card menu uses 44px items, and receipts use readable sentence-case heads. |
| Responsive visual polish | **9** | **9** | **Phone and short viewports:**<br>• At 320, the placeholder `https://example.org` measures **137.85px** inside the field's **164px** text box, with no clipping, and the document is 320px wide.<br>• Share opens at `scrollTop 0` with focus on Close at 390×640 (dark), 320×640 (light) and 640×360 (dark); dark 390×640 also reopens at 0.<br>• At 640×360 the 77 / 172 / 77 header/body/footer split stays tight but intentional.<br>**Wider layouts:**<br>• At 768, rail rows are 44px at y76/124/172/220.<br>• Phone tabs are balanced, and Search miss at 390 is balanced. |

## R24 → R25 repairs, verified live

All observations are on `/artifacts/461183aa…/edit` with Network & permissions expanded.

| Repair | Viewport and scheme | Measured |
| --- | --- | --- |
| Group heads | 1280 light / dark; 320 dark | h3 at 14px / 600 / 21px. Ink is `rgb(48,58,73)` in light and `rgb(204,208,214)` in dark. Identical to "Create new tag". |
| Capability captions | 1280 light / dark | `span.spacer.field-label`, 13 / 600 / 19.5, `rgb(82,97,116)` light / `rgb(151,158,170)` dark. Rows are 44px with the label centred against a 132×44 select. |
| Capability captions | 320 dark | 14 / 600, matching the phone caption rule. |
| Secondary Add | 1280 light / dark | `btn btn-sm btn-sec`, 76×44, plus icon, 14 / 500 slate ink. Tags Add is 73×44 with the same class. |
| Empty-state helper | 1280 / 320 | `p.form-help.allowlist-empty`, "No origins approved.", 13/19.5 desktop and 14/21 phone. Visible in both schemes. |
| Persistent "Add origin" caption and complete example | 320 dark / light | The caption sits above a 184×44 field, so the full placeholder fits. |
| Caret on first line | 320 dark | The summary wraps to 83px (three lines). The caret is at y15 with height 14, beside line 1. |
| Populated draft row (unsaved specimen) | 320 dark / light | **What I did:** typed `https://api.council-specimen-unsaved.example.org` and pressed the section's Add. This only changes the page's working copy until "Save changes" is pressed.<br>**What I saw:** the status became "Unsaved changes" and the summary read "· 1 approved origin". The row is 268×81, with the origin on line 1 as an ellipsised `code` chip and an 83×44 Remove on line 2. The document stayed 320px wide.<br>**How I restored it:** clicked Remove. The status returned to **"All changes saved"**, the empty helper reappeared, and the summary read "No network access". Nothing was saved. |

Prior repairs re-observed live this round:

- **Tags**
  - 32×32 chip actions at 1280; 44×44 at 320.
  - Full-width 268×44 "Create and add tag" at 320.
  - Edit tag sheet at 1280 light: aligned well and hex row, muted presets with the current one marked, Delete (outline) / Cancel / Save split.
- **Stored state at 320 light**
  - The count "· 1 key" appears once the panel loads. It is not missing, which resolves my R24 "seen once" note.
  - 44px move/remove squares.
  - Paired Export and Import notes; Erase is a quiet outline behind a hairline.
- **Versions at 1280 light:** both date cells start at x716. View and Restore are 36px, together at x860 / x947.
- **Card receipt at 1280 dark:** sits within the viewport above its trigger; readable heads.
- **Duplicate at 320 dark:** 18px checkboxes with helpers; full-width primary and Cancel.

## Other surfaces observed live

| Surface | Viewport, scheme and `matchMedia` | What I saw |
| --- | --- | --- |
| Library | 1280 light (false) | Four 236px cards, 32px kebab slots, aligned title rows, 240px rail. |
| Library | 1280 dark (true) | Card receipt open. |
| Shared with me | 768 dark (true) | Active label in accent, owner attribution, Owners filter group. |
| Shared by me | 768 light (false) | Active row; all rail rows 44px. |
| Search miss | 390 light | Integrated clear, dashed empty card, centred copy, 44px Clear search, active Search tab. |
| Share | 390×640 dark (true) | Opens at top: client 477 / scroll 1061. Wheel-scrolled to the bottom: Revoke, data-mode cards (the selected one leads), public-link rule, disclosure, pinned Done. Reopen at 0. |
| Share | 320×640 light (false) | Notice, link toggle with helper, URL/Copy pair, Replace link with helper, Embed disclosure. |
| Share | 640×360 dark (true) | Body 172 / 947, footer flush. Here I set `scrollTop` from script to measure the end, rather than scrolling by wheel. |
| Card menu → Duplicate | 320 dark (true) | Menu items are 186×44. Cancelled. |
| Settings | 320 light | Aligned uppercase facts, restrained delete card with its blocked reason. |
| Source Find / Replace | 1280 dark (true) | Searched "meta" with Next. Field fills to Next / Previous / All / Close; whole option units. Query cleared. |
| Source completion | 1280 dark (true) | Ctrl+Space on the `--surface` line: 227×242 panel, app palette, 14px rows. Dismissed with Escape. |
| Add, Agent mode | 320 dark (true) | Selected segment, captions, full-width Start building / Cancel. |
| Filters | 390 light (false) | 16/600 title, 14/600 group heads, 44px rows, full-width Done. |

## Minor opportunities (not decisive)

- **Populated allowlist chip.** In dark, the origin `code` chip uses `rgb(22,24,28)`, darker than the panel, across the full 268px row width. It reads as a slightly heavy black strip with 1×6px padding. A field-tinted or transparent mono line would sit more quietly. Light (`rgb(245,246,248)`) is fine.
- **Desktop Replace row.** At 1280, Replace and Replace all end at x993 while Close ends at x1035, so the second row's right edge does not meet the first row's.
- **Settings phone buttons.** At phone widths, Accounts, Sign out and Delete account are intrinsic-width, while the editor's phone actions are full-width. Defensible as standalone card actions.
- **Small native and style details.**
  - Native dark unselected radio: grey disc.
  - Add segments: heavy weight and 46px height, versus 44px controls.
  - Uppercase fact eyebrows versus sentence-case captions: a role split.
  - The 240px rail at 768.
  - Quiet source hints.
- **Desktop permissions column.** About 220px of open space remains below the Add field at 1280. It is now explained, but could be balanced by placing the Add row above the list if this is ever revisited.

## Coverage and limits

**Session and data handling**

- I used only my own tab, **`tab_1m_0d23e38c-d413-4e76-b0eb-59687a206343`**. It was opened after `preview_status` with `reuseExistingTab:false, open:false`, its exact id was passed on every call, and it is now closed. I was already signed in.
- Scheme was set only with `preview_set_appearance`, never by a root class. `matchMedia('(prefers-color-scheme: dark)')` was checked as follows:

| Width | Light (`matchMedia` false) | Dark (`matchMedia` true) |
| --- | --- | --- |
| 1280 | Editor, Library | Editor, Library, Find, completion |
| 768 | Shared by me | Shared with me |
| 390 | Search (`matchMedia` not read there), Filters (checked) | Share 390×640 |
| 320 | Share 320×640, Settings and the state panel (`matchMedia` not read on those two) | Permissions, Duplicate, Add |
| 640×360 | — | Share |

- **Short-viewport Share:** I did not open 390×640 light, 320×640 dark or 640×360 light this round. Only the sizes and schemes listed above were checked.

**Writes made**

- No saves, creates, deletes, grants, transfers, erases, link replacements, data-mode changes, builds, tests, commits, capture writes or subdelegation.
- Temporary changes, all restored:
  - one unsaved allowlist draft row, removed by its own Remove; the status was confirmed "All changes saved";
  - a Find query, cleared;
  - completion opened and dismissed without accepting.
- Dialogs and menus were dismissed unchanged. **I wrote only this file.**

**What was observed live, and what was not**

- Evidence is live screenshots plus computed geometry and style. Saved prior captures were not used.
- Not re-exercised this round:
  - **Go to line:** the shortcut did not open the panel in my session, so I did not observe it.
  - Replace actions; active-row selection paint; the 1280 / 320 syntax matrix.
  - The widget panel; populated assets; the viewer More menu and viewer receipt.
  - URL / Paste modes; 768 Library cards.
  - Search with results and blank search.
- Not exercised at all: long or arbitrary data, enlarged text, invalid states and hosted identity.
- Single Chromium engine with emulated viewports. No physical phone or keyboard, alternate engine or accessibility certification.
- User-authored tool and widget designs are excluded.

## Scores (light / dark)

- Typography: 9 / 9
- Composition and spacing: 9 / 9
- Color harmony: 9 / 9
- Contrast and emphasis: 9 / 9
- Component visual consistency: 9 / 9
- Responsive visual polish: 9 / 9
