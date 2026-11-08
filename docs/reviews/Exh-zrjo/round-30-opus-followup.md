# Round 30: independent Opus follow-up review

Reviewer: **claude-opus-5-5**, 2026-11-05. The live session ran about 06:19–06:29 UTC, after reading the context.

- **Frozen runtime:** 42c94eb1cb12a74108885cc1cbb7aaf7d11f5721
- **Served assets:** `aa7a00afd7a2`. I confirmed this from the versioned `/assets/v/aa7a00afd7a2/…` URLs on both fixtures, at entry and at exit.
- **PR273 base:** ab0135c9
- **Fixtures:** populated http://t3code:18280 (renderer :18281); empty http://t3code:18380 (renderer :18381).
- **Runtime scope:** the strict Git check over `internal web e2e cmd scripts Makefile go.mod go.sum` against 42c94eb1 was empty at entry and at exit (diff and porcelain), with no exclusions. I used no repo-wide diff.

**Verdict: PASS for Opus. All fourteen scores are 9.**

This is one model's independent result. It is not a joint result and not a council-completion claim. The incoming joint streak is 0/2, and only the parent can pair this with the peer's Round 30 report.

## Independence and reading

**Read:**
- The operative Round 30 header and the archived 29/28/27 briefs, with their cumulative findings, responses and unresolved judgments
- `visual-round-30-changes.md` and `visual-round-29-changes.md`
- `followup-validation.md`
- Both prior Round 29 reports in full (`round-29-opus-followup.md`, `round-29-sol-followup.md`)
- The opening of `visual-brief.md`
- Earlier history only through the cumulative archive. I did **not** re-read every Round 5–28 report line by line.

**Did not open:**
- Any current Round 30 peer report or status file
- `followup-scores`, summary, resume or rating-verification files
- Any nine-* aggregate, the ticket, or any parent timer

The validation file reports the full 21-case E2E run and the standard Go run as still in progress. I did not assume they passed, and I ran no tests or builds. Tests did not set any score.

## Fourteen scores

| Category | Light | Dark |
| --- | ---: | ---: |
| Typography | **9** | **9** |
| Composition and spacing | **9** | **9** |
| Color harmony | **9** | **9** |
| Contrast and emphasis | **9** | **9** |
| Component visual consistency | **9** | **9** |
| Responsive visual polish | **9** | **9** |
| Save discoverability and feedback | **9** | **9** |

### Why component consistency rises from 8.5 to 9

My Round 29 score of 8.5 rested on four sibling empty and zero-result views written in three different grammars. That is now **one grammar, measured live**.

On the empty fixture, at **1280×800, 768×900, 390×844 and 320×740**, in **light and dark**, I checked 24 route/width/scheme views. In each one, `matchMedia('(prefers-color-scheme: dark)').matches` matched the scheme and horizontal overflow was 0.

| View | Visual | Headline | Helper | Action |
| --- | --- | --- | --- | --- |
| Search welcome | Illustration, 340×200 desktop; 280/272×180 phone | "Welcome to Exhibit" | "Build with an agent, paste HTML, or start from a URL." | Filled **Create your first artifact → `/new`**, 225×**44** |
| Starred empty | 52px icon tile | "No starred artifacts yet" | "Choose Star in an artifact's action menu to keep it here." | Secondary **Browse your artifacts → `/search`**, 187×44 |
| Shared empty | 52px icon tile | "Nothing shared yet" | "Share one of your artifacts, or ask someone to share with you." | None, as intended |
| Search query miss (empty fixture, "zebra") | Icon tile | "No artifacts match your search." | "Try another title, keyword, or tag." | Secondary **Clear search**, 44 |
| Search filter miss (populated, "zebra" + Health tag) | Icon tile | Same | Same | **Clear search and filters** (label correctly names the filters) |

Shared properties across every view:
- **Headline:** 16px / 600 / 24px, `rgb(48,58,73)` in light and `rgb(204,208,214)` in dark.
- **Helper:** 14px / 400 / 22.4px, muted `rgb(93,104,120)` in light and `rgb(154,160,169)` in dark.
- **Rhythm:** visual → headline → helper → action is **12 / 12 / 12px** in every view, width and scheme.
- **Actions:** all 44px. The welcome's 48px outlier is gone.
- **Accent:** icon tiles take the accent, `rgb(35,85,158)` in light and `rgb(126,170,240)` in dark.

The illustration and the icon tile vary by role, as the brief allows. Text and control roles are identical.

**Resets and routes:**
- On the empty fixture, a real **Clear search** returned to the welcome state with an empty field, "Search" h1 and header intact, still in dark.
- On the populated fixture, **Clear search and filters** returned to `/search` with all 6 cards and no chips.
- The prior lost-CTA bug has not returned: welcome links to `/new`, and Starred links to `/search` with no self-link and no Share instruction.

**Rendered views I looked at:** Search welcome at 1280 light, Starred at 320 dark, the query miss at 320 dark, and the filter miss and normal Search at 1280 light. At 320 the Starred panel and actions sit well clear of the fixed 46px tab bar.

The other surfaces I inspected (editor, sheets, Share, source chrome, menus, Activity) stay within the shared token and control grammar.

**Residual nits, not score-relevant:**
- Only the miss headline ends in a period. The other three headlines do not.
- The welcome helper wraps into two lines at 356px on desktop, where it has room for one. It reads as deliberately balanced, not crowded.

## The three user defects

**1. Selected-preset corner: repaired.**
- **Geometry, live:** the cell has an 8px outer radius and a 1px accent border. The `::after` badge is 16×16 with radius 4, set at top and right 3px inside the border. The inner border radius is 7, and 7−3 = 4 matches the badge radius, so the corners are concentric.
- **Visual check:** I scaled only the preset group (×5 at 320 light, ×6 at 1280 dark) and reverted it with no persistence. The badge follows the cell's arc with an even gap, and there are no fringe pixels at the outer corner.
- **Coverage:** Tags creation at 320 light and 1280 dark, and the Edit tag sheet at 320 dark (64×44 cells) and 1280 dark (49×36).
- **Remaining overlap:** on the compact 34.75×36 desktop creation cell, the dot is now 18px. The badge still overlaps roughly the upper-right third of it: badge x 14.75–30.75 / y 4–20 against dot x 8.4–26.4 / y 9–27. It reads as an intentional overlay badge, not broken pixels, and the larger phone and sheet cells clear it. This is a refinement only. A 16px dot, or a 1px surface-coloured ring on the badge, would remove the overlap.

**2. Tag pill spacing: repaired.**
- **Desktop:** each tag is a compact 28px capsule (Health 70, Reading 80, Test 55, Utilities 76px wide) with a 4px gap to two separate **32×32** utilities.
- **Phone:** at 320 the same capsules sit beside **44×44** utilities.
- There is no padded chip wrapping the actions at any width I checked.

**3. Header Save: clear.**
- **Reversible title edit in all 8 width/scheme pairs:** I appended " — local review" and restored the title, never clicking Save. In every pair:
  - Clean state: **Saved**, Save disabled at opacity 0.55, `aria-busy="false"`.
  - Dirty state: **Unsaved**, Save enabled as the filled primary. Light is `rgb(35,85,158)` with a white label; dark is `rgb(126,170,240)` with an ink label.
  - Restored state: **Saved** and disabled again.
  - Real typing at 1280 light and 390 dark gave the same result.
- **Controls now stay put:** the 54px status box and the 82×44 Save are identical across clean and dirty states. At 1280 the status sits at x1120 and Save at x1182, with View at x962.9. At 390 they are x230 and x292, and at 320 x168 and x230. This fixes Round 29's 13px View shift and 2px Save shift.
- **Deep scroll:** I scrolled to the bottom with the editor dirty (scrollY 414/314/475/611 by width). The header stayed at y6, with Unsaved and an active Save.
- **Label and glyph:** "Save" plus a floppy icon at every width, with a 17px label on phones. Overflow was 0.
- **Consistent Save icon:** the header, the Edit tag sheet and Save state all use `ph-floppy-disk`. Round 29's sheet check glyph is gone, and selection badges keep their check.

## Current Save outcome evidence (supplementary, not my own execution)

`docs/screenshots/Exh-zrjo/followup-save-states-round-30/manifest.json` names **42c94eb1 / aa7a00afd7a2**, from the passing current 21-case E2E run on disposable artifacts with a deliberate 503. I viewed these images:

- **`saving-390-light` and `saving-1280-dark`:** "Saving…" beside a visibly subdued named Save.
- **`save-error-320-light` and `save-error-320-dark`:** amber **Review**, with a pinned notice below the sticky header. The notice has a single Phosphor warning icon and plain text, "Temporary save failure. Try again." The old raw "✗"/"Error:" prefix is gone. Save remains active for a retry.

The live DOM matches this template: `p.editor-save-message[role=alert]` containing a `ph-warning` icon and a text span.

I did not run a persistent save, an error, or a retry myself.

The `followup-round-30/` populated corpus had **no manifest** at the time I checked, so I did not use it as evidence. `round-30-after/` names 42c94eb1 / aa7a00afd7a2, but I relied on my own live views instead.

## Other live coverage

**Share sheet** (real `#share-open` on the Run Log viewer; nothing changed):

| Viewport | Light | Dark |
| --- | --- | --- |
| 320×640 | Opened at top, Close focused, mid-scroll paint checked | Opened at top, Close focused. Body 477/1124, wheel-scrolled to 647, lower People/Revoke, data cards, rule and disclosure all reachable. Reopened at 0. |
| 390×640 | Opened at 0 (477/1061), scrolled to 584 | Opened at 0, scrolled to 584 |
| 640×360 | Opened at 0 (172/947), scrolled to 774.5 | Reopened at 0, scrolled to 774.5 |

- The footer hit-test owned its region in every case.
- Landscape uses the illustrated two-card data-mode variant with centred copy, which is intentional. Its 77px header and 77px footer around a 172px body are tight but orderly. That is a known minor issue.
- The dark unselected radio is still a grey filled disc (known minor).
- "Each person connects their own accounts." wraps short, leaving the right half of the line empty (trivial).

**Source editor** (Run Log, 1280):
- **Light:** native Find with selected text, flat Next/Previous/All, a fixed square Close, whole-unit options, and a Replace row. The source viewport is retained.
- **Dark:** Go to line (Ctrl+Alt+G), a compact readable row. The selection is `rgb(39,54,74)` and the echo on the next line is visibly quieter.
- **Dark completion** (Ctrl+Space): an opaque `rgb(32,35,40)` popup, 14px, with the selected row `rgb(28,41,64)` and text `rgb(232,234,237)`. I dismissed it without accepting.
- No edit was made: still Saved.

**Expanded panels** (320 light):
- **Network & permissions:** allowlist head, "No origins approved.", origin caption and field with a secondary Add, and six 44px capability rows.
- **Stored state · 1 key:** helper rhythm, a 44px square inspector utility, nested record cards, and full-width 44px Save state, Cancel, Export and Import, each with its paired helper, then a hairline and a quiet outlined Erase.

**Destinations:**
- Activity at 1280 dark.
- Starred populated at 768 light, with the card menu (Unstar/Edit/Duplicate, opaque, aligned icons). The menu was dismissed.
- Search populated at 1280 light.

**Add and Settings:**
- Add artifact at 390 dark: Agent selected, the field family, full-width Start building and Cancel.
- Settings at 390 dark: computed headings (26/650 page title, 15/600 sections) and no overflow, but I did not inspect it visually.

**Minor observations (none change a score):**
- **Activity:** every row repeats "Artifact version saved." beneath its title, nine 136px rows in all. The "No agents" paragraph spans 992px while the neighbouring blocks cap at 880px. Dropping the filler line would tighten the list.
- **Back links:** Add and Settings label theirs **"Library"** (→ `/`), but PR273's destination is named **Starred**. This is wording drift, not a visual defect.
- **Save state:** in the expanded Stored state panel, **Save state** is an enabled filled primary even when there is no state edit. The header Save goes quiet when clean, and this panel does not. This is a small inconsistency in the clean-state grammar. It does not lower the Save score, because the header Save, the outcome notice and retry are clear.
- **Fixture change I did not make:** Run Log's tags changed during my session. Its card first showed no tag dots, and later the API and the card showed Health/Reading/Test/Utilities. I saved and attached nothing; I only opened and dismissed Edit tag. I attribute the change to other actors on the shared fixture.

## Coverage limits (honest)

**Not inspected visually this round:**
- Phone layouts of the populated Search, Shared and Activity destinations. Phone destinations were verified on the empty fixture only.
- 768 Tags or Edit tag, and Tags in 320 dark, 1280 light and 390 entirely.
- The expanded widget panel, Versions and the lower Stored assets block.
- The Filters sheet, the Duplicate dialog body, the viewer receipt and More menu, and the card posture popover.
- Settings, which I measured but did not view.
- 768 Share.

**Sampled only:**
- Native source chrome at a single width per mode: Find in light, Go and completion in dark.
- The empty-state matrix used same-origin, viewport-sized frames under real scheme emulation. I confirmed representative views on real tab resizes.

**Not exercised:**
- Persistent save, error or retry, and permission Review/resolution.
- Long or custom tags, populated assets or origins, enlarged text, hosted identity, and populated running agents.

**Environment:** one Chromium with real `prefers-color-scheme` emulation. No physical phone, Safari, software keyboard, screen reader, or accessibility certification. User-authored tool and widget bodies were excluded from scoring.

## Restoration and side effects

- **No persistent changes:** no persistent save, create, delete, grant, star, transfer, erase, link replacement or data-mode change.
- **Local changes undone:**
  - Title specimens were restored without Save.
  - The Edit tag and Share sheets were dismissed with Escape or Done.
  - Find and Go were closed, and completion was dismissed without accepting.
  - The zoom transforms were reverted (`style=""`).
  - Details panels were reset by a reload.
- **Normal side effect:** navigation's automatic opened receipt occurred, as normal product behaviour.
- **Final editor reload:** title "Run Log", header **Saved**, Save **disabled**, `aria-busy="false"`, 0 open dialogs, 0 overflow, assets `aa7a00afd7a2`.
- **Tabs:** I used only my own tabs, `tab_27_0d23e38c-…` (default profile, main fixture) and `tab_26_0d23e38c-…` (incognito, empty fixture), with the exact ID on every call. I signed into the empty fixture only in incognito. **Both tabs are closed.**
- **Files:** I ran no builds, tests, commits, app edits or subdelegation. This report is the only file I wrote.
