# Round 20 — independent Opus visual review (target 9)

Reviewer: **claude-opus-5-5**, 2026-11-04, about 19:52–20:25 UTC, after the 19:50 UTC quota reset.

**Application under review**

- Frozen application **8504bc516e4be2ed7fd8369b7f3823632cc6272f**. Served assets **33556024040a**, confirmed from live versioned asset URLs at entry and again at exit.
- Base: origin/main **44def51a**.
- HEAD at exit was **57acffec** (evidence commits only). A read-only `git diff 8504bc51..HEAD -- internal web` was empty.

**What I read**

- `visual-brief.md` and `nine-brief.md`.
- The model reports for rounds 17–19, read in full: R17 Opus, R17 Sol (partly), R18 Sol and R19 Sol. The R18 and R19 Opus files are records with no scores.
- Change notes 6–20 and `nine-validation.md`.
- Earlier reports are known through the brief's summary.

**What I did not read**

- `round-20-sol-visual.md`
- `nine-scores.json`, `nine-summary.md`, `nine-resume.md` and `nine-opus-resume-prompt.md`
- the ticket and any parent aggregate

My scores are fresh. They were not shaped by any peer rating. Tests did not decide any score.

**Verdict: FAIL against the all-twelve-at-least-9 target.** The two latest repairs are verified and excellent:

- **History columns:** desktop rows now share their columns.
- **Share:** the sheet scrolls correctly.

Most surfaces are now at a 9 level. However, one material surface that earlier rounds never examined still sits clearly below that bar: the **card / viewer "Sandbox posture and sharing" popover**. A small family of off-token helper texts and a misaligned tag color row also remain. Three categories score 8.5 in each scheme.

This single-model report establishes no joint pass and makes no council-completion claim. The incoming target-9 streak is zero.

## Twelve scores

| Category | Light | Dark | Weakest material surface |
| --- | ---: | ---: | --- |
| Typography | **8.5** | **8.5** | The posture/sharing popover sets 4–5-line paragraphs at 12px with `line-height: normal` (about 14px), under a 10.5–12px header scale. Two helpers are 12px/normal while every other helper uses the 14/21 token. "Create new tag" is a 12px/700 heading that is smaller than the 13px/600 captions it introduces. |
| Composition and spacing | **8.5** | **8.5** | The card popover opens 52px past the viewport bottom (document grows 800 → 852). Scrolling to read it then exposes an empty band below the 240px rail panel. The tag color row is inset 7px on both edges from its field column. The Paste phone footer puts a cramped helper between the primary button and Cancel. |
| Color harmony | **9** | **9** | The slate/blue system is coherent in both schemes, with restrained tiles, presets and source surfaces. The only off-palette note is trivial: the native color well's neutral `rgb(107,107,107)` frame in dark. |
| Contrast and emphasis | **9** | **9** | Primaries lead quiet destructive actions, and selected methods and choices are unmistakable. In source, the current Find result clearly leads the quieter other matches. The muted helpers are readable. |
| Component visual consistency | **8.5** | **8.5** | The same `capability-popover` header renders as 12px/600 sentence case on the card and as a 10.5px uppercase eyebrow in the viewer. `text-sm muted` helpers bypass the shared helper token. The Filters sheet lacks the shared modal family's header rule, and its list is indented 8px from its own title and Done. The native color well is unstyled. |
| Responsive visual polish | **9** | **9** | 1280, 768, 390 and 320 are each deliberately composed: sheets, stacked footers, Find, completion inset, history reflow, tabs and rail. The phone-specific blemishes (the Paste helper and the lone "me" on two tab labels) are counted once, under typography and composition. |

## Material findings

### 1. Posture and sharing popover: dense type, no viewport collision handling, two header idioms

**Observed live, both schemes.**

- **Library card:** `/` at 1280×800 → Run Log card → `button[aria-label="Sandbox posture and sharing"]`.
- **Viewer:** `/artifacts/461183aa…` at 390×844 → pill shield (`.pip-posture`).

**Measured on the card popover** (`.capability-popover`, absolute, `overflow: visible`, `max-height: none`):

- It occupies y578–852 in an 800px viewport, and `documentElement.scrollHeight` becomes **852**.
- After scrolling to read it, the rail's panel colour ends at y748. Below it is a bare page-ground band, visible in both light and dark.
- Body text is **12px / `normal`**. The Sharing paragraph runs five tight lines inside a 234px-wide box. Headers are 12px/600, sentence case.

**Measured on the viewer popover:**

- Meta line 11.5px, header "Sandbox posture" 10.5px uppercase, body 12px / `normal`, Manage link 12px.
- The four-line "Fully contained — no network, download, clipboard…" paragraph reads cramped next to the rest of the app, where helpers use 14/21.

This popover is the library's one way of saying, in words, everything that is shared and reachable; the PRD's enumeration principle depends on it. It reads like a debug tooltip rather than a finished component, and it is the clearest remaining gap between "polished" and "excellent".

**Treatment**

1. Set popover body copy on the shared helper token (13–14px, about 1.45–1.5 line height).
2. Use one section-header style in both places: the card's sentence-case 600 heading, or the eyebrow, but not both.
3. Let the card popover be wider than its 234px card (about 300px), so paragraphs drop to three lines or fewer.
4. Add collision-aware placement: open upward when there is not enough room below, or clamp to the viewport with a `max-height` and internal scroll. The popover must never extend document height.

No new content or ornament is needed.

### 2. Off-token helpers and a heading smaller than its labels

**Share's replace-link helper.** "The current link stops working." is `SPAN.text-sm.muted` at **12px / `normal`**. Every other Share helper is **14px/21px**. Observed at 320 light.

- Share also uses two helper tones in one field group: `rgb(93,104,120)` above the People field and `rgb(71,85,105)` below it.

**Add → Paste footer.** "Network origins are surfaced for approval before anything runs." is also `text-sm muted`, **12px / `normal`**.

- Desktop: it sits tidily left of Cancel and Add to library.
- 390 dark and 320 dark: it wraps to two cramped lines *between* the full-width primary and Cancel, splitting the otherwise standard phone action stack.

**Editor → Tags (1280, both schemes).** The "Create new tag" `H3` is **12px/700**, while the captions beneath it ("Name", "Color") and the parallel "Add existing tag" caption are **13px/600**. A group heading smaller than its children inverts the hierarchy.

**Treatment**

1. Map `text-sm muted` in form and dialog contexts to the shared helper token and one helper colour.
2. On phones, place the network-origins note above the action stack (or below Cancel) so the primary and secondary buttons stay together.
3. Make "Create new tag" at least caption size (14px/600, ink), or give both tag groups the same subsection style.

### 3. Tag color row misaligned, with an unstyled native well

**Observed live.**

- Editor Tags panel at 1280, light and dark: the name field spans x234–554, but the color input starts at **241** and the hex field ends at **547**.
- Edit tag sheet at 320 dark: the name field spans x21–299, but the color well spans **28–72** and the hex field **80–292**.

`.color-custom-row` carries `padding: 6px` (plus a 1px border), which produces a 7px inset on both edges, visible as a jog under every full-width field above it. The native `input[type=color]` paints a `rgb(107,107,107)` neutral frame around the swatch in dark, the one pure grey left in an otherwise slate palette.

**Treatment**

1. Remove the row's horizontal padding (or move it inside the controls) so the well and hex field sit on the field column's edges.
2. Give the well the field surface, border and radius tokens, and remove the native wrapper padding and background.

### 4. Smaller composition items (sub-9 contributors only in combination)

- **Share, 390 (both schemes).** The "Anyone with the link…" checkbox label is a 44px target with top-aligned content (y255–299), so the visible gap to its explanation is about 42px. At desktop the same gap is about 24px. Centre the label content in its target, or let the helper follow at the standard helper gap.
- **Filters sheet, 390 light.** No header rule, unlike Duplicate, Edit tag and Share, and the content is not a separate scroll body. Collections and Tags links are padded 8px in from the "Filters" title and the Done edge. Adopt the shared sheet shell and one inset.
- **Phone tabs (390 and 320).** "Shared with / me" and "Shared by / me" leave a lone word on the second line. `text-wrap: balance` would give "Shared / with me" and "Shared / by me" without changing the upstream labels.
- **Duplicate, 1280 dark.** The share-settings helper strands "enabled." on its own line. Use `text-wrap: pretty` on helpers.
- **Card footer, 1280 and 768.** The kebab glyph sits about 6–8px below the title's first line and the tag dot. On phones it is aligned. This is optical only and retained from earlier rounds.

## Prior-repair verification (fresh, live)

- **Shared desktop history columns (R20 change). Verified.**
  - Run Log `/edit` → Versions, 1280, both schemes. Each row is `subgrid`; both dates start at **x715.91**, and both action cells span **859.65–1046**.
  - View and Restore sit side by side; Current is end-aligned (badge right edge 1042 vs button edge 1046, a trivial 4px).
  - The table now reads as one column of facts. The 108px stagger is gone.
- **Phone history grouping. Retained.** At 320 dark the rows fall back to `40px 220px`, with version, title, date, badge or actions stacked. Return to Library and Delete are centred full-width.
- **Share scroll, paint and lower content (R19). Verified.** No setting was changed at any width.

  | Width | Scheme | Body clientHeight / scrollHeight | What I saw |
  | --- | --- | --- | --- |
  | 1280 | dark | 612 / 919 | Wheel-scrolled to People, both complete saved-data choice cards, the public-link rule and "What happens when I switch?" |
  | 390 | dark | 681 / 1070 | Scrolled to the full lower sheet |
  | 320 | light | 577 / 1133 | Scrolled through Saved data |

  The radios stay inside their cards and Done stays fixed and separate. There was no detached paint and no document overflow.
- **Native source chrome.** All in a temporary Paste draft that was cleared afterwards (see Coverage).
  - **Find at 1280 light:** the field flexes to Next, Previous, All and a bordered × (all 36px).
  - **Search highlighting:** with "visitor", the current result has the selection fill and outline, and the second match has the quiet accent tint, so they are clearly distinct.
  - **Checkboxes:** the option checkboxes are the shared 18px accent controls.
  - **Find at 320 dark:** field plus 44px ×, a whole-unit 30/40/30 Next/Previous/All row, options wrapping 2+1, a full Replace row, and no document overflow.
  - **Completion:** I pressed Control+Space at `color` in CSS, at 1280 in both light and dark. The list rows are centred with a clear selected tint and app surfaces; nothing was accepted.
- **Duplicate.** At 320 light and dark it is a bottom sheet with Title, 18px checkboxes, option titles leading their helpers, and a full-width primary over Cancel. At 1280 dark it has a trailing Cancel / Duplicate footer. Dismissed unchanged.
- **Edit tag at 320 dark.** Sheet header Close, a 4×2 muted preset grid with the selected Health cell clearly marked, and Save, Cancel and outlined Delete in a full-width stack. Dismissed unchanged; the colour row issue is reported in finding 3.
- **Library and spaces**
  - 1280 light and dark: four 236px columns with widgets rendering.
  - 768 dark: two cards beside the 240px rail, which is comfortable and makes rail work optional.
  - 768 dark `/shared`: active bold label on one line, count separate, owner chip at the card inset.
  - 320 light: card menu opaque with even icon and label rows.
- **Search.** 768 light results for `run`: the active rail item, an integrated clear, and two result cards. 390 dark miss for `not-in-this-library`: a balanced two-line empty state, a 44px Clear, and the active Search tab.
- **Add.** 390 dark Agent and Paste: segmented methods with a persistent selected fill, and the complete "Paste HTML here…" prompt. 1280 light Paste: method cards with an icon, title and helper; the footer has the helper left and actions right.
- **Settings.** 320 light covers Profile, Administration, Storage, Sign out and the danger zone with its disabled-state explanation. 1280 dark has aligned fact columns and consistent value weights.
- **Viewer.** At 390, the More menu in dark and light has opaque surfaces and even rows. The pill at 320 keeps 6px between the title and Share. The posture popover is reported in finding 1. The light Run Log body is user-authored and not scored.

## Retained judgement calls, reassessed

- **240px rail at 768.** Comfortable; not a 9-blocker.
- **Quiet source echoes and active band.** Still the right hierarchy: the selection and current Find result lead and syntax stays readable. I re-checked Find live but did not redo the full paint matrix.
- **Completion's square property-type marker.** Utilitarian, not a blocker.
- **Share's four-line warning on phones.** Necessary and proportionate.

## Coverage and limits

**Session and data handling**

- I used only my own tab, `tab_19_0d23e38c-…`, opened with `reuseExistingTab:false, open:false`, and passed its exact id on every call. It is now closed.
- I first loaded `localhost`, where frame-ancestors blocked the widgets, so I switched to `t3code:18280` and signed in as the scratch admin. Nothing from the `localhost` load was scored.
- Scheme was set only with `preview_set_appearance`. `matchMedia('(prefers-color-scheme: dark)').matches` returned the expected false/true at 1280, 768, 390 and 320. I used no root-class simulation.
- The temporary Paste draft was cleared before I left: hidden `#body` length **0**, placeholder restored.
- Every dialog, menu and popover was dismissed unchanged.
- I made no saves, creates, deletes, grants, link replacements, data-mode changes, builds, tests, commits, capture writes or subdelegation. **I wrote only this file.**

**What was observed live, and what was not**

- Primary evidence is live screenshots plus computed geometry and style; the full observed set is listed above. I did not use saved captures in `nine-round-20/`, and none of my scores rely on them. Source was not read for scoring; the class names cited come from the live DOM.
- Sampled rather than exhaustive:
  - I did not check every route, theme and width combination.
  - The posture popover was checked at card 1280 in both schemes and in the viewer at 390 light.
  - I did not exercise long or arbitrary data, custom colours, enlarged text, invalid or dirty states, or the completion detail pane, special characters and bracket variants.
  - The admin users page and Go-to-line were not re-checked this round.
- Single Chromium engine with desktop pointer emulation. No physical phone, alternate engine, hosted identity or accessibility certification.
