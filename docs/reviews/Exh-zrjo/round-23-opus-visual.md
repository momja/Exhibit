# Round 23 — independent Opus visual review (target 9), post-reset execution

Reviewer: **claude-opus-5-5**, 2026-11-05, live session approximately 00:54–00:57 UTC.

**Application under review**

- Frozen application **5f1021e3495ad3639e0c5e51859124e44edc79f8**. Served assets **c948e619826d**, confirmed from the live versioned asset URLs on the editor, Library, Search, Add and Settings pages.
- Base: origin/main **44def51a**.
- HEAD at exit was **5eb86c93** (evidence commits only). A read-only `git diff 5f1021e3 HEAD -- internal web` shows only `internal/api/state_test.go`, which is the stated validation-only change. No runtime template, CSS or JS differs.

**What I read**

- `visual-brief.md`, `nine-brief.md`, `visual-round-23-changes.md` and `nine-validation.md`.
- Both R22 reports in full.
- Earlier rounds 5–21 (reports, the Opus quota and deferral records, change notes 6–22) through the brief's full summary of findings and responses, plus selective reading. I did not read all of them line by line.

**What I did not read**

- `round-23-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md`, `nine-opus-resume-prompt.md`
- the ticket and any parent aggregate

My scores are fresh. They were not shaped by any peer rating. Tests did not decide any score.

**Verdict: FAIL against the all-twelve-at-least-9 target.**

The R23 repairs are real and good:

- Save state is the sole filled task primary.
- Export and Import are paired with their own notes.
- Erase is a quiet outline, behind a hairline.
- Phone panel actions are 44px.
- Inspector icon buttons are uniform 36 / 44 squares, and Remove property is right-aligned.
- The widget help has a proper gap.

The data-section pocket I scored down in R22 is now finished in both schemes. However, a wider pass over the same editor finds three adjacent places where the 44px phone standard and the shared action grammar still are not applied. They are small individually, but they sit on the same material editor page, and they are the same class of inconsistency the last two rounds were closing. Component consistency and Responsive polish therefore stay at 8.5 in both schemes.

This single-model report establishes no joint pass and makes no council-completion claim. The incoming joint streak is zero.

## Twelve scores

| Category | Light | Dark | Weakest material surface (observed live) |
| --- | ---: | ---: | --- |
| Typography | **9** | **9** | Shared helper rhythm holds everywhere I sampled:<br>• Desktop 13/19.5, phone 14/21.<br>• Page titles 26/650; captions are sentence case, weight 600.<br>• Receipt headers, Share and Duplicate copy, and Search empty states read cleanly.<br>Residue (trivial): Tag chip text is 12px, and the Add method segment labels are noticeably bold at 320. |
| Composition and spacing | **9** | **9** | The Stored state and assets transfer groups now read as pairs:<br>• Desktop: a 180px action column, with the note 12px beside it.<br>• Phone: 8px from button to its note, then 16px to the next group.<br>• Erase sits behind a hairline.<br>• Widget help starts 12px below the tile.<br>History columns align. Share opens and reopens at the top. One minor exception is described under component consistency. |
| Color harmony | **9** | **9** | A coherent slate/blue system:<br>• Dark Save is a light-blue fill with dark ink.<br>• Erase is now coral text inside a muted red outline rather than a solid fill.<br>• Restrained default tiles; the Unit Converter tile has an olive tint in dark, which is still in family.<br>• Muted presets, and an integrated source surface. |
| Contrast and emphasis | **9** | **9** | One filled primary per task area: Save state, Duplicate artifact, Start building and Create and add tag. Destructive entries are quiet at rest. The current Share mode card and the selected preset lead clearly. |
| Component visual consistency | **8.5** | **8.5** | Three editor controls still bypass the shared size and placement rules:<br>• **Tag chip** Edit and Remove buttons are **20×20** at every width, beside 32px or larger controls on desktop and 44px controls on phone.<br>• **Add property** is **36px** high, top-aligned beside its 44px field and select, while the identical New key / type / Add key row has a **44px** button.<br>• **Create and add tag** is an intrinsic **192px** primary on phone, while every other phone primary on the page is full width (Save state, Return to Library, Duplicate, Start building). |
| Responsive visual polish | **8.5** | **8.5** | At 390 and 320, in both schemes, the 20×20 chip controls are the only sub-44 interactive targets I found on the phone editor. Everything else was raised in R23, including inspector squares, Add record and Add property. Share at 320×640, its reopen, and landscape 640×360 are correct. Landscape keeps a 77 / 172 / 77 header/body/footer split, which is tight but intentional. |

## Material finding: three remaining exceptions to the editor's shared control grammar

All observed live on `/artifacts/461183aa…/edit`. Disclosures were opened locally only, and nothing was edited or saved.

**1. Tag chip actions (both schemes, every width)**

- The Health chip is 114×26 at 12px type.
- The "Edit tag Health" pencil and the "Remove tag Health from this artifact" × are **20×20** buttons. Measured:
  - 1280 light: x300 and x325, y122.
  - 390 light: x96 and x121.
  - 320 dark: x92 and x117.
- The rest of the editor follows a 36px compact-utility rule on desktop and a 44px rule on phone (`matchMedia('(pointer:coarse)')` was false, and the phone sizing is width-driven elsewhere).
- Visually, the chip is a pleasant pill. But it is now the only control family on the phone editor that was not brought onto the 44px standard, and the remove × is a real per-artifact action.

**2. Add property row (desktop, both schemes)**

- At 1280: input 180×44 and select 106×44, then **Add property 137×36**, top-aligned, so its bottom edge sits 8px above the field's.
- The New key row two blocks below uses select 44 plus **Add key 102×44**.
- At phone widths both rows are 44, so this exception is desktop only. In a screenshot it reads as a button that has not been resized to its row.
- Add record (standalone, 36) is fine as a compact utility.

**3. Create and add tag width (phone, both schemes)**

- At 390 and 320: 192×44, left-aligned, at x26 or x30.
- On the same page:
  - Save state: 268 or 330, full width.
  - Return to Library and Delete: full width.
  - Duplicate artifact and Start building primaries: full width.
- This is the only filled phone primary on the page that does not span its column.

**Treatment to earn 9.** No ornament or new behaviour is needed.

1. Give the chip's Edit and Remove a proper slot: 32×32 on desktop (matching the card kebab and posture target) and 44×44 on phone. Expanding the hit area while keeping the 12px glyph is fine.
2. Size Add property to its row (44), as Add key already is. Alternatively, make both compact, but make them agree.
3. Use the phone full-width primary rule for Create and add tag, as for Save state.

## R23 repairs, verified live

**Stored state actions**

- 1280 light:
  - Save state is filled `rgb(35,85,158)`, 118×36.
  - Cancel is secondary.
  - Export JSON and Import JSON are 180×36 secondaries in a shared column, each with its note starting at x426 on the same row.
  - A hairline follows.
  - Erase all data is `btn-danger-outline`: white fill, `rgb(180,35,24)` text, `rgb(255,187,187)` border.
- 320 dark (`matchMedia` dark true):
  - Save state, Cancel, Export, Import and Erase are each **268×44**.
  - The Export note starts 8px below Export and is 14/21. The Import note follows the same pattern.
  - Erase is coral text in an outline.
  - Document width 320, no overflow.

**Inspector controls**

- Move up, Move down and Remove record are 36×36 squares on desktop and 44×44 on phone.
- Delete this key and Remove property share right edge x1010 (36 wide) at 1280 dark, and x250 (44 wide) at 320 dark.
- Nested record controls sit 9px further in, inside their card. That is consistent with the nesting.

**Stored assets (1280 light)**

- Export assets ZIP and Import assets ZIP sit in the same 180px column, with notes beside them.
- Paragraph gaps are 12px.
- The empty-state line precedes the transfers.

**Widget help (1280 light)**

- Tile at y384–516; help paragraph starts at y528 (12px gap), 13/19.5.
- Regenerate (disabled) and Remove are 36px.
- This repairs my R22 finding.

**Share**

| Viewport | Scheme | Result |
| --- | --- | --- |
| 320×640 | dark | Opens at `scrollTop 0` (client 477 / scroll 1124), with focus on header Close. Wheel to bottom shows the grant row, Revoke, both saved-data cards and the public-link line, with the native radios contained. Done stays fixed. Escape then reopen returns to `scrollTop 0`, focus on Close. |
| 640×360 | light | Opens at the notice. |

**Posture receipt (1280 dark)**

- Panel 320×323 at x264 / y191, ending at y514. It does not extend the document.
- 14/600 section heads and readable body copy.

**Other surfaces**

| Surface | Viewport and scheme | What I saw |
| --- | --- | --- |
| Card menu → Duplicate | 390 light | Menu items are 44px high. The sheet has a fixed header with a 44px Close, 18px checkboxes with helpers, and a full-width primary/Cancel stack. Cancelled. |
| Search miss | 320 dark | Integrated clear, balanced empty card, 44px Clear search, active Search tab. |
| Add, Agent mode | 320 dark | Selected segment, captions, full-width primary/Cancel. |
| Settings | 768 light | Aligned fact rows; 44px Accounts and Sign out; restrained Delete account card with its explanation. |
| Versions | 1280 light | Both dates at x716; actions grouped. Phone grouping at 390 light is clean, with 44px View/Restore. |
| Library | 1280 dark | Four 236px columns, 32px kebab slots, the 240px rail. |

## Minor opportunities (none decisive on its own)

- Landscape Share spends 154 of 360px on header and footer. Compacting the vertical padding at short heights would help.
- Native dark unselected radio is a filled grey disc.
- The Current badge's right edge sits 4px inside the Restore pair's right edge (1042 vs 1046).
- Settings: the Delete-account explanation uses body ink, while other card explanations use muted helper ink. This is defensible as emphasis.
- On phone, the selected tag preset renders as a wide tinted 77×44 cell with a corner check. It is clear, though a little heavier than the dots.
- Uppercase fact eyebrows versus sentence-case captions remain a consistent role split.
- The 240px rail is comfortable at 768.

## Coverage and limits

**Session and data handling**

- I used only my own tab, `tab_1i_0d23e38c-d413-4e76-b0eb-59687a206343`. It was opened after `preview_status` with `reuseExistingTab:false, open:false`, its exact id was passed on every call, and it is now closed.
- I was already signed in.
- Scheme was set only with `preview_set_appearance`. `matchMedia('(prefers-color-scheme: dark)')` was confirmed false at 1280, 640×360 and 390 light, and true at 1280 and 320 dark. 768 was checked in light only, and 390 dark was not sampled. I used no root-class simulation.
- I made no saves, creates, deletes, grants, transfers, erases, link replacements, data-mode changes, builds, tests, commits, capture writes or subdelegation. Dialogs and menus were dismissed or cancelled. **I wrote only this file.**

**What was observed live, and what was not**

- Evidence is live screenshots plus computed geometry and style. I used no saved captures.
- Not re-exercised this round: source Find/Replace/Go/completion and selection paint, the Edit tag sheet, Filters, shared spaces' active labels, URL and Paste Add modes, the viewer More menu, and admin users. For these I rely on the prior two rounds' live coverage and treat them as sampled, not certified.
- Not exercised at all: long or arbitrary data, populated asset rows, enlarged text, and invalid or dirty states.
- Single Chromium engine with emulated viewports and a fine pointer. No physical phone or software keyboard, alternate engine, hosted identity or accessibility certification.
- User-authored tool and widget designs are excluded.

## Scores (light / dark)

- Typography: 9 / 9
- Composition and spacing: 9 / 9
- Color harmony: 9 / 9
- Contrast and emphasis: 9 / 9
- Component visual consistency: **8.5 / 8.5**
- Responsive visual polish: **8.5 / 8.5**
