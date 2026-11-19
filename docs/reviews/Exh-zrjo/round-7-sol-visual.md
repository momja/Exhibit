# Round 7 — independent Sol visual review

Candidate: `a242183a` on `origin/main` base `44def51a`; HEAD independently confirmed. Reviewed the running app at `http://t3code:18280` against [the fixed visual brief](visual-brief.md). Read both round 5 reports, both round 6 reports, and the round 6/7 change responses. These scores come from fresh rendered inspection, not inherited scores or test-suite results.

**Verdict: all twelve scores reach 8 in this Sol review. One advertised phone refinement is not actually fixed: the replacement-link helper still wraps beside its button.** I classify that remaining isolated wrap as a minor finish defect, rather than a substantial visual issue. This verdict does not verify the claim that all four share fixes landed, and does not establish a joint passing round or complete the required two-round council streak.

## Scores

An 8 means polished and coherent without a substantial visual issue. It allows small refinements; it does not mean exceptional design. Each category considers the weakest materially inspected app-owned surface, not an average that conceals a weak page.

| Fixed category | Light | Dark | Fresh visual basis |
| --- | ---: | ---: | --- |
| Typography | 8 | 8 | Coherent heading, label and metadata hierarchy; system type on controls; readable code and secondary text. Share headings and the remaining disclosure now agree in sentence case. |
| Composition & spacing | 8 | 8 | Desktop cards and editor sections have orderly alignment and rhythm. Settings facts align; narrow history groups metadata and actions clearly. The share comparison has comfortable full-width options. The localized helper wrap below is a remaining refinement. |
| Color harmony | 8 | 8 | Neutral shell, blue emphasis and restrained default tile tints belong together. The recessed dark code surface fits the app rather than looking like a foreign theme. |
| Contrast & emphasis | 8 | 8 | Headings and actions lead over quiet secondary outlines. Source placeholder, gutter and doctype remain readable. Selected sharing choices and owner attribution are clear; dirty Save gains filled emphasis. |
| Component visual consistency | 8 | 8 | Buttons, fields, source editor, menus and dialogs have compatible type, boundaries and surfaces. Phone Copy and editor Add/Create actions now align in height with neighboring fields. |
| Responsive visual polish | 8 | 8 | Gallery/tab icons, integrated Search clear, narrow history and stacked sharing choices remain coherent. The one ragged helper wrap is visually untidy but leaves a short, complete, readable sentence and does not create a cramped or broken material surface. |

No 9–10 is justified: these are well-resolved utility interfaces with dense forms and ordinary opportunities for further finishing.

## Verification of the four requested phone share refinements

Route: `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332`, viewer Share → `#share-panel`. Inspected live at 390px and 320px in **both** explicit color schemes, with scrolling through the sheet.

1. **Replacement helper on its own row — NOT fixed.** At 390px, “The current link stops” starts beside Replace link and “working.” falls beneath the button at the left edge. At 320px, “The current link” starts beside the button and “stops working.” falls below it. Both themes show the same layout. This is observed rendered evidence, not an inference from the supplied notes.
2. **Copy height and URL truncation — fixed.** Live geometry reports the public URL input and Copy button both **44px** high. The URL visibly ellipsizes at rest. A separate live focus/select check returned `text-overflow: ellipsis` at rest, `clip` when focused, and selection `0–58` over the complete 58-character read-only value. The embed Copy control also measured 44px when queried; its expanded presentation was not visually re-inspected.
3. **Disclosure casing — fixed.** Scrolled rendered text says “What happens when I switch?” in the same sentence-case treatment as its neighbors.
4. **People placeholder — fixed.** “Usernames or emails” is fully visible, including at 320px; “Separate names with commas.” appears below the field. Live DOM inspection confirms the accessible name remains “Usernames or emails, separated by commas”. No names were entered and no grant changed.

### Remaining refinement: replacement-link helper layout

**Location/theme:** viewer route above, `#share-panel .share-replace-row`, light and dark, 390/320px.

**Objective evidence:** live computed style is `display: block` on `.share-replace-row`, while its span remains `display: inline`. The span's 390px bounding box spans two lines, beginning at y≈365.9 while the button spans y≈355.4–387.4. Subsequent source inspection of `web/gallery/detail.css` lines 99–102 explains the failed response: the phone rule sets `flex-direction: column`, `align-items: flex-start` and `gap: 6px`, but does not establish a flex container.

**Suggested fix:** add `display: flex` to that narrow-screen rule, or make the helper a block below the button. Recheck 390px and 320px in both themes. This is a concrete layout discrepancy with the change note. Its classification as a small finish defect rather than an under-8 blocker is my aesthetic judgment: the sentence is short and legible, other controls align, and the sheet no longer has the material comparison-card problem from round 5. I have not silently counted this issue as resolved.

## Other fresh regression observations

- **Observed live, desktop light/dark:** gallery/card metadata and default Unit Converter tile; dark card action menu; duplicate dialog in both themes, cancelled without submission. The quiet header/secondary boundaries remain, and dialog buttons share the surrounding app typography.
- **Observed live, editor light/dark:** populated source, first doctype line and gutters at 1280px; expanded Tags at 390px; Versions at 320px. Source and section headings are readable in both schemes. Add/Create phone actions measure **44px** and visually match the fields.
- **Observed live, dirty-state emphasis:** temporarily appended text to the title at desktop in dark. Save changes changed from outlined to filled blue and the status became “Unsaved changes”. Restored the exact original title and dispatched input; the button returned to `btn btn-sec` and the clean state. Nothing was saved. Dirty emphasis in light was not separately exercised.
- **Observed live:** incoming Search `q=packing` at 320px in both schemes. Clear stays inside the field, owner attribution remains discernible, and fixed tab icons align above wrapping labels.
- **Observed live:** `/new` Paste at 390px in both schemes. Placeholder is readable: live colors are light `rgb(94,104,119)` on white and dark `rgb(164,174,189)` on `rgb(26,29,33)`. This confirms the actual rendered editor theme; I did not rerun a full contrast calculation or inspect every syntax token.
- **Observed in current rendered PNGs:** 390px gallery both themes; Settings at 1280/390 both themes; Add agent at 1280 both themes; URL and Paste at 390 both themes; incoming dark and outgoing light shared spaces; dark applied filters; desktop dark Search miss; phone light library search miss; 320px light invalid Paste. These samples retained the coherent controls, spacing, tab chrome and neutral/blue palette. Agent execution and URL fetching were not performed.

## Method and limits

- Own T3 tab `tab_d_0d23e38c-d413-4e76-b0eb-59687a206343`, opened with `reuseExistingTab: false`; that ID was passed on every subsequent browser call. Used the `t3code` host so widget frames could render.
- Explicit `preview_set_appearance` calls selected actual light/dark media emulation. Live `matchMedia('(prefers-color-scheme: dark)').matches` returned false/true at the representative 1280, 390 and 320 CSS-pixel widths. Layout sizes were 1280×800, 390×844 and 320×740.
- Supplementary captures came from `docs/screenshots/Exh-zrjo/visual-round-7/`. Its manifest, captured at `2026-11-04T08:05:49.489Z`, lists 68 states with zero mismatched dark-media assertions. I inspected the samples identified above, not all 68 images. The manifest is capture-process evidence, not independent live verification of every state.
- User-authored tool/widget bodies are excluded, including the light Run Log body under a dark preference. App-owned toolbar, cards/default tiles, source editor, menus and dialogs are included.
- Coarse-pointer devices, software keyboards, alternate engines, arbitrary long names/data, post-save screens, every overlay and every route/theme/size combination were not tested. Viewer posture popover and More menu were not reopened this round. Settings and the URL/agent method presentations were assessed from current captures rather than interacted with live. No complete functional test suite was run by this reviewer; supplied test passes do not determine these visual scores.
- No valid saves, creates, deletes, grants or data-mode changes. Only the assigned report was written; no production edits, commits or delegation.
