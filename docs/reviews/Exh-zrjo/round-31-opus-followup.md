# Round 31: independent Opus confirmation review

**Reviewer:** claude-opus-5-5, 2026-11-05. This is a fresh execution after the earlier Round 31 Opus attempt failed without scores. Live session ran about 10:52–11:20 UTC.

**Interface reviewed:**
- **Production application:** `42c94eb1cb12a74108885cc1cbb7aaf7d11f5721`
- **Assets:** `aa7a00afd7a2`, confirmed from versioned asset URLs on both fixtures at entry and at exit
- **Validation baseline:** `c1156ef15c7f5caa282b1068214492bce42018e1`

**Scope check:**
- The strict scope (`internal web e2e cmd scripts Makefile go.mod go.sum`) against c115 was **empty** for both `git diff` and porcelain status, with no exclusions. HEAD at exit was `b95670a4…`; later commits are evidence only.
- The complete 42→c115 scoped diff is exactly two test files: `internal/api/assets_test.go` and `internal/api/galleryspaces_test.go`. I did not hide or ignore any other file. No runtime change was observed, so the candidate is not invalidated.

**Verdict: PASS for Opus. All fourteen scores are 9.**

This is one reviewer's independent result. It is not a joint result and does not complete the council. The incoming streak is 1/2, and only the parent can pair this report with the peer's.

## Reading and independence

**Read:**
- The operative Round 31 header and the archived 30/29/28/27 briefs, with their cumulative findings
- Both Round 30 reports in full (`round-30-opus-followup.md`, `round-30-sol-followup.md`)
- `visual-round-30-changes.md` and `visual-round-31-changes.md`
- `followup-validation.md`
- `round-31-validation-baseline.json` and `round-31-filter-fixture.json`
- `round-31-opus-followup-unavailable.md`, the parent's record of the failed attempt, which contains no scores

**Not opened:**
- Any Round 31 Sol report, status, launch or rating-verification file, or any other Round 31 status file
- `followup-scores`, summary, resume or timer files, nine-* aggregates, the ticket, or any parent checkpoint

`visual-brief.md` and `nine-brief.md` were read in earlier rounds. This round I relied on their content as restated in the archived brief rather than re-reading them.

**Validation context:** the safe validation note reports the standard full Go run as **PASS (exit 0, 431.62s)** on c115, and the other checks as passing. I ran no tests or builds. Tests did not set any score.

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

No score is below 9, so no treatment is mandatory. Optional refinements are listed at the end.

## Method

**Tabs:**
- Main fixture: `tab_2e_0d23e38c…`, profile `default`, at :18280. It was already signed in.
- Empty fixture: `tab_2d_0d23e38c…`, profile `incognito`, at :18380. I signed in with admin/changeme in incognito only.
- Both tabs were opened after `preview_status` with `reuseExistingTab:false` and `open:false`. Every call named its exact tab ID.

**Scheme and size:** each scheme was set with real `preview_set_appearance`, and `matchMedia('(prefers-color-scheme: dark)').matches` was checked at each pair. I used no root-class simulation. Settled screenshots were saved and viewed.

**Discarded frames:** three of my own probes disturbed the page. I discarded those frames and did not score them as product defects:
- A CSS `zoom` on the preset grid broke its fixed tracks. I replaced it with a reverted `transform: scale(6)`.
- A programmatic `scrollIntoView` scrolled the CodeMirror scroller 41.5px horizontally.
- A click on a wide `.cm-line` scrolled it horizontally, and a double key press stacked two Go dialogs.

## Filter-only miss: the new live specimen

The fixture was valid: `http://t3code:18380/search?tag=Review%20empty`, with one unassigned tag and zero artifacts.

**What every one of the 8 width/scheme pairs showed:**
- Search h1 and the active-filter chip "Review empty ×" (36px high on desktop, 44px on phone)
- Desktop rail Tags entry selected with a check
- Dashed empty panel with a 52px accent sliders tile
- Headline "No artifacts match your filters." at 16/600 (`rgb(48,58,73)` light, `rgb(204,208,214)` dark)
- Muted 400 helper: "Remove a filter to see more of your library."
- Secondary **Clear filters** button, 119×44, linking to `/search`
- Rhythm of exactly 12/12/12px, and horizontal overflow 0

Phone clearance between Clear filters and the tab bar was 108px at 320×740 and 279px at 390×844.

**Rendered views I inspected:** 1280 light and dark, 768 dark, 390 light, 320 dark. The other pairs were measured on real tab resizes.

**Real Clear filters clicks** (three, at different widths and schemes):

| Width / scheme | Result |
| --- | --- |
| 320 light | Returned to `/search` with the welcome state and the `/new` CTA 77px above the tab bar |
| 390 dark | Returned to `/search` with the welcome state |
| 1280 dark | Returned to `/search` with the welcome state |

In each case the URL was `/search`, the chip was gone and the query empty, and the header and h1 were preserved. The page showed "Welcome to Exhibit", the illustration, and **Create your first artifact → `/new`** at 44px. The search field received focus, which reads as a deliberate next step. Overflow was 0. The prior lost-CTA bug has not returned.

## All five empty contexts

**Live rendered views:**
- Starred empty at 320 dark
- Shared empty at 320 light
- Query miss at 768 light
- Welcome at 390 dark
- Filter-only miss at the pairs listed above

**Full matrix:** to cover 5 contexts × 4 widths × 2 schemes (40 views), I used viewport-sized same-origin frames under the real scheme emulation, measured, then removed them. All 40 had:
- Headline 600 and helper 400
- Visual→headline→helper rhythm of 12/12, and 12px to the action wherever one exists
- 44px actions with correct targets: Browse your artifacts→`/search`, Create your first artifact→`/new`, Clear search→`/search`, Clear filters→`/search`, and no action on Shared, as intended
- Overflow 0

Phone action-to-tab-bar clearance ranged from 77px to 385px.

Frame-based views are supplementary to the real resizes. Representative frames were confirmed live.

**Judgment:** this is one component grammar. The icon tile versus the illustration varies by role, while the text and control roles stay identical.

## Populated destinations

| View | What I saw |
| --- | --- |
| Starred, 1280 light | Two starred cards. The footer star, tag dots, posture link and kebab share one center line (cy 322.5). |
| Shared, 1280 dark | Incoming "Shared packing list" with owner attribution, beside owned Run Log. Active rail "Shared 2", and a header search scoped to this space. |
| Activity, 1280 dark | Running agents with Refresh, the "Data change history · Coming soon" notice, and recent changes with version chips and history links |
| Search hit "run", 390 light | Two results. Owned Run Log shows star, tags, link and kebab. The phone tab bar marks Search as active. |
| Search, 320 light | Full card list, used for the card menu |

## Exact user-reported defects

**1. Selected-preset corner: repaired, verified live.**
- **Measured geometry** (Tags creation, 1280 dark): cell 34.75×36, radius 8, border 1. The `::after` badge is 16×16, inset 3/3, radius 4, with no border.
- **Visual check:** at 6× (transform only, then reverted), the badge corner is concentric with the cell's inner arc with an even gap. There are no fringe pixels at the outer corner.
- **Rendered pairs:** creation at 320 light (61×44 cells, where the badge clears the 22px dot), Edit tag Health at 1280 dark, and Edit tag Reading at 390 light. In the Reading sheet the custom colour field took the selected outline because #8a5db0 is not a preset, which is consistent.

**2. Tag pill spacing: repaired.**
- Capsules are 28px high: Health 70, Reading 80, Test 55 and Utilities 76px wide.
- Utilities sit outside the capsule: 32×32 on desktop (4px gap) and 44×44 on phones.
- At 320 each tag wraps to its own 52px row, with no wasted chip padding.

**3. Header Save: clear and stationary.** I ran a temporary title dirty/restore at all 8 pairs (1280/768/390/320 × light/dark) and never pressed Save. At 1280 light and 320 light the edits were real keystrokes. The other six pairs used an equivalent `input`-event script.

Each pair showed the same sequence:
- **Clean:** Saved. Save disabled at opacity 0.55, `aria-busy="false"`, floppy glyph and the word "Save".
- **Dirty:** Unsaved. Filled primary: light `rgb(35,85,158)` with a white label, dark `rgb(126,170,240)` with an ink `rgb(14,23,38)` label.
- **Deep scroll:** with the editor dirty, scrollY was 414/314/474.5/611 by width. The header stayed at y6, still Unsaved with an active Save.
- **Restored:** Saved and disabled again.

Status, Save and View x positions were identical in clean and dirty states:

| Width | Status x | Save x / width / height | View x |
| --- | ---: | --- | ---: |
| 1280 | 1120 | 1182 / 82 / 44 | 963 |
| 768 | 608 | 670 / 82 / 44 | 451 |
| 390 | 230 | 292 / 82 / 44 | 178 |
| 320 | 168 | 230 / 82 / 44 | 116 |

The rendered 320-light deep frame showed "← Search · 👁 · Unsaved · [💾 Save]" with no truncation. The header Save and Edit-tag Save use the same floppy glyph; check marks appear only as selection badges.

**Save outcomes (supplementary, not my own execution):** the `followup-save-states-round-30/manifest.json` names 42c94eb1 / aa7a00afd7a2, from the passing 21-case E2E run with a deliberate 503. Its validity is unaffected by the test-only c115 commit. I viewed:
- **`saving-1280-light`:** "Saving…" beside a subdued, disabled Save.
- **`save-error-320-dark`:** amber "Review", with a sticky inline notice containing one warning icon and "Temporary save failure. Try again." Save stays active for a retry.

I did not run a persistent save, an error or a retry.

## Broader representative coverage

**Short Share sheet** (opened with the real `#share-open`; nothing changed):

| Viewport | Light | Dark |
| --- | --- | --- |
| 320×640 | Opened at top, Close focused. Body 477/1124. Wheel-scrolled to 647: lower People, Revoke, both choice cards and the disclosure all painted above the pinned Done. | Reopened at 0, Close focused. Wheel-scrolled to the bottom; same lower paint. |
| 390×640 | Opened at 0 (477/1061), wheel-scrolled to 584 | Opened at 0, mid-scroll paint viewed |
| 640×360 | Opened at 0 (172/947), wheel-scrolled to the bottom. Choice cards and footer clean. | Opened at 0, scrolled to 774.5. Radios paint inside their cards. |

The footer hit-test owned its region in every case.

**Viewer overlays** (390×640 dark): the posture receipt is readable, with headings and a divider before the link. The More menu (Open in new tab, Edit, Modify with agent, Download file, Hide controls) has aligned icons and an opaque surface.

**Source editor:**
- **768 light:** native Find and Replace with a real selection of `Run Log` in line 6. The panel has a flat Next/Previous/All row, a square Close and whole-unit options. A real **Next** click moved to line 180 with no horizontal offset. Still Saved.
- **390 dark:** the Go-to-line panel (`cm-dialog`) is a compact, readable label/field/Go/Close row. I opened two by accident and closed both.

**Expanded editor panels** (1280 dark):
- **Network & permissions:** an empty allowlist note, Add origin with a secondary Add, and six 44px capability selects.
- **Stored state · 1 key:** helper, nested record inspector with square utilities, Add record/property/key, Save state/Cancel, paired Export/Import explanations, then a hairline and a quiet Erase.
- **Stored assets:** explanation and warning paragraphs with distinct gaps.
- **Widget:** the app-owned frame, help text, Regenerate and Remove.
- **Versions:** both date starts at x716, Current badge, and View/Restore.

**Gallery overlays at 320:**
- Light: the card menu (Unstar, Edit, Duplicate) opened above its anchor, and the Duplicate sheet showed title, checkbox descriptions and a full-width primary/secondary stack. I cancelled it.
- Dark: the Filters sheet showed Collections and Tags with dots and a pinned Done.

**Add and Settings:**
- Add artifact at 768 dark: three method cards with Agent selected, and the field family above Cancel / Start building.
- Settings at 320 light: Profile, Administration and Storage cards, with fact labels and helper text.

## Minor observations

None of these lowers a score; each is an optional refinement:

1. **Save state clean-state emphasis:** in Stored state, **Save state** is an enabled filled primary with no state edit. The header Save goes quiet when clean, so the panel's commit should share that grammar: disabled or secondary until a state field changes. This is the most visible inconsistency in Save presentation, but it is local to an expanded panel, and the header, sheet and outcome feedback are clear.
2. **Compact preset overlap:** on the 34.75×36 desktop creation and sheet cells, the 16px badge covers roughly the upper-right third of the 18px dot. It reads as an intentional overlay with clean contours. A 16px dot, or a 1px surface-coloured ring on the badge, would separate them.
3. **"Library" back links:** Add and Settings label their back link "Library", while the PR273 destination is named Starred. The editor correctly says "Search" via `return_to`.
4. **Activity repetition:** every row repeats "Artifact version saved."
5. **Short landscape Share:** at 640×360 the 172px body needs several scrolls because of the generous header and footer.
6. **Dark radios:** the unselected dark radio is a grey filled disc.
7. **Headline punctuation:** only the two miss headlines end in a period. Starred, Shared and Welcome do not.

## Coverage limits

**Not exercised:**
- **Completion popup:** Ctrl+Space did not open it from my programmatic focus state, so I did not observe it this round.
- **Go-to-line in light, and Find in dark:** each was observed in one scheme only.
- **Populated states and accessibility:** populated origins or assets, long or custom tags, enlarged text, hosted identity and running agents.
- **Save outcomes:** persistent save, error and retry, and permission Review, beyond the supplementary images.

**Sampled only:**
- **Phone destinations:** populated Shared and Activity were not rendered at phone widths this round. Populated Search was.
- **Tags and Edit tag at 768:** measured through the Save cycle but not visually inspected.
- **Card posture popover and Embed tile:** not opened.

**Environment:** one Chromium with real `prefers-color-scheme` emulation. No physical phone, Safari, software keyboard, coarse-pointer or accessibility certification. User-authored artifact and widget bodies were excluded from scoring.

## Restoration and side effects

- **No persistent changes:** no save, create, delete, grant, star, transfer, erase, link replacement, data-mode change or fixture mutation. I did not create or attach any tag.
- **Normal side effect:** navigation's automatic opened receipts occurred.
- **Local changes undone:**
  - Title specimens were restored without Save.
  - The Edit tag sheets were cancelled. One Cancel click returned a transport reset, but the sheet has no save path, and the editor reloaded clean.
  - Share, Duplicate, Filters, the menus and the receipt were dismissed.
  - Find and both Go dialogs were closed, and the scroller was reset to scrollLeft 0.
  - The zoom and scale probes were removed (`style` attribute null, no remaining zoom or scale).
- **Final editor reload:** title "Run Log", **Saved**, Save disabled, `aria-busy="false"`, 0 open dialogs, overflow 0, assets `aa7a00afd7a2`. Tags were Health/Reading/Test/Utilities, unchanged from entry.
- **Fixtures:** the empty fixture ended at `/search` with the welcome state.
- **Tabs:** both of my own tabs are closed, and no other tab was touched.
- **Files:** no builds, tests, commits, app edits or subdelegation. This report is the only file I wrote.
