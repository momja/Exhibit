# Round 8 — independent Sol visual review

Frozen candidate: `4d725ba252d637e64c6c3c648bffe0d83fc27bd1`; HEAD and merge-base with `origin/main` (`44def51afa43019771fd258bc61b28e3ebf4613b`) independently confirmed. App: `http://t3code:18280`, asset version `9f7135f0fccb`. Read the [visual brief](visual-brief.md), both models' round 5/6/7 visual reports, and the round 6/7/8 change notes. Rounds 1–4 are excluded.

**Verdict: all twelve scores reach 8. The replacement-link helper is now actually fixed on the rendered phone sheet. No substantial visual blocker found in the inspected app-owned surfaces.** This is a passing Sol review. If Opus also passes this round, it establishes the first joint pass only; a separate fresh joint confirmation round remains required.

## Scores

Each score reflects the weakest materially inspected surface, without averaging a weak surface away. An 8 means polished/coherent with no substantial visual issue, rather than excellent or exceptional design.

| Fixed category | Light | Dark | Current visual evidence |
| --- | ---: | ---: | --- |
| Typography | 8 | 8 | Consistent system controls, clear page and section hierarchy, readable source/gutters and secondary copy. Share headings/disclosure use sentence case. |
| Composition/spacing | 8 | 8 | Desktop card titles align; editor sections and phone history have orderly rhythm. Share helper occupies its own row, with comfortable full-width choices below. Settings facts align and stack coherently. |
| Color harmony | 8 | 8 | Neutral shell, blue emphasis and restrained tile tints remain coherent. Dark source surface belongs to the surrounding app. |
| Contrast/emphasis | 8 | 8 | Content and headings lead over quiet secondary borders. Editable fields remain identifiable; doctype/gutters and owner attribution are readable; selected share choices have clear emphasis. |
| Component visual consistency | 8 | 8 | Buttons, fields, card menu, dialogs and editor share compatible type and surfaces. Phone URL/Copy and tag Add/Create pairs have matching heights. |
| Responsive visual polish | 8 | 8 | The ragged helper wrap is gone at 390/320px. Sharing choices stack with readable measures; Search clear stays integrated, tab icons align, and narrow version entries group metadata/actions clearly. |

These are resolved utility interfaces with some dense explanatory content. Fixing prior defects does not justify inflating the scores to 9–10.

## Fresh verification of the phone Share sheet

**Observed live:** Run Log viewer `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332` → `#share-open` → `#share-panel`, at 390×844 and 320×740 in both explicitly emulated themes. Scrolled through the sheet without changing sharing controls.

- **Replacement helper: fixed.** `.share-replace-row` computes to `display:flex` and `flex-direction:column`. At 390px the button bottom is y=387.4 and helper top y=393.4; at 320px they are y=429.4 and y=435.4. Both light and dark have the same **6px gap**. The complete sentence sits below the button, with no stranded word or text beside it. This is live geometry and visual observation, not acceptance of the change note.
- **URL/Copy: retained.** The visible public-link input and adjacent Copy each measure **44px** high in all four theme/width combinations. The URL visibly ellipsizes at rest. Copy was not invoked.
- **People: retained/improved.** “Usernames or emails” remains fully visible, with “Separate names with commas.” below it in the surrounding paragraph treatment. The grant-row divider now reaches the section edges.
- **Saved-data choices: retained.** Both stack vertically with clear selected/unselected treatments. They measure **312px wide** at 390px and **242px wide** at 320px. At 320px each is 113px high, with readable descriptions rather than text towers. The sentence-case disclosure is visible below them.
- The dialog's `scrollWidth` equals `clientWidth`: **356/356** at 390px and **286/286** at 320px in both themes. No horizontal overflow was observed.

## Other fresh observations

- **Live, 1280×800, both themes:** library/card typography, metadata alignment and default tiles; populated Run Log editor, section headings, doctype and gutters. The dark library retains quiet outlines; the dark source panel is recessed and integrated. The light source remains readable despite relatively little syntax coloring. Light input font computes to `system-ui, sans-serif` and gutter text to `rgb(94,104,119)` on a white editor.
- **Live, dark desktop:** Run Log card action menu. Its spacing, icon/text alignment and surface agree with the cards and shell.
- **Live, 390px, both themes:** expanded editor Tags. Name/color creation fields now reach the section's right edge; Add/Create actions measure 44px and visually match the neighboring fields.
- **Live, 320px, both themes:** editor source/history region and expanded Versions. The version, label, timestamp, badge and action grouping remain clear. Incoming Search `/search?q=packing` has an integrated clear button, discernible owner chip, coherent card title and aligned fixed-tab icons.
- **Current PNG observations:** gallery at 390px, Settings at 1280/390px, Add agent at 1280px, Paste/URL at 390px, and duplicate dialog at 390px, all in both themes. Also sampled dark incoming shares, light outgoing shares, dark desktop global Search miss and light phone filter miss. These retain the same coherent typography, boundaries and spacing. Settings and creation/duplicate presentations were assessed from captures, not interacted with live this round.

## Evidence and limits

- Own T3 tab `tab_g_0d23e38c-d413-4e76-b0eb-59687a206343`, opened with `reuseExistingTab:false`; its ID was supplied on every subsequent browser call. Used `t3code` for widget CSP compatibility.
- Explicit `preview_set_appearance` selected light/dark. Live `matchMedia('(prefers-color-scheme: dark)').matches` confirmed false/true at representative **1280, 390 and 320 CSS-pixel** checks. No class-based dark substitute was used.
- Supplementary PNGs came from `docs/screenshots/Exh-zrjo/visual-round-8/`. Its manifest is timestamped `2026-11-04T08:13:25.183Z`, lists **68** captures, and has **zero** mismatched recorded media assertions. Only the samples identified above were visually inspected; the manifest does not prove every surface was independently reviewed.
- Prior reports, supplied regression passes and change notes provided context. Scores come from current rendered evidence. No production-source inference is used to declare an unobserved surface fixed, and no test suite was rerun.
- No substantial blocker remains in the inspected surfaces. The Share sheet's long explanatory copy and the light editor's subdued syntax differentiation remain aesthetic refinement opportunities, not material legibility/composition defects.
- This is not every route/theme/viewport combination. Viewer posture/More popovers, expanded embed/disclosure content, editor dirty-state emphasis and footer targets were not freshly inspected this round. Actual coarse-pointer devices, keyboards, alternate engines, arbitrary long data, post-save states, agent execution and URL fetching were not tested.
- User-authored artifact/widget contents are excluded, including the light Run Log body under a dark preference. App-owned cards/default tiles, toolbar, source editor, menus and dialogs are included.
- No saves, creates, deletes, grants or data-mode changes. No production edits, commits or delegation. Only this assigned report was written.
