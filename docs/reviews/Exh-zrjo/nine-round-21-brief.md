Independent Exhibit visual council — ROUND 21, target9/10. This is a fresh review of a new frozen application, not an old8 confirmation.

Original user brief: "iterate on exhibits ui in a new worktree. Aim for consistency and stellar UI/UX patterns. Use a council of codex sol and Claude opus subagents to rate the interface in each round, have them score the interface 0-10, then iterate on their feedback. Continue this in a loop until the interface is scoring an 8 or higher consistently across all categories." Subsequent instructions: show actual before/after screenshots, rebase off latest, test real prefers-color-scheme dark, use the council to identify wrong dark choices, and use a separate UI-only typography/composition/color rubric. Latest user: "Now climb to a 9/10."

Frozen APPLICATION 74544552a816a3ac3a6dd0c04f8d0713fe223cce, asseta48067e4e6f5. Worktree /home/maxomdal/.t3/worktrees/Exhibit/feature-Exh-zrjo-ui-council, branch feature/Exh-zrjo/ui-council, fetched base origin/main44def51afa43019771fd258bc61b28e3ebf4613b. HEAD may advance for evidence only; app code stays frozen throughout this joint round.
Preview http://t3code:18280/render18281; scratch admin/changeme. Run Log /artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332 and /edit.

READ-ONLY except your assigned unique report. No builds/test suites/commits/app edits/capture writes/persisted fixture changes/subdelegation. Use your own T3 tab, never other tabs. No valid saves/creates/deletes/grants/link replacement/data-mode changes. Temporary unsaved specimens may be opened then restored/cleared without saving. Exclude user-authored widget/tool designs and credential-dependent agent execution. A light user tool body under dark preference is not an app-owned defect.

Independence: Do NOT read your peer's ROUND21 report or nine-scores.json, nine-summary.md, nine-resume.md, ticket or other parent aggregate/checkpoint files. All completed reports through round20 are PRIOR context; R18 Opus is a quota record and R19 Opus only a deferral record, without scores. No peer scoring agreement or target-induced inflation.

Required context: read docs/reviews/Exh-zrjo/visual-brief.md, nine-brief.md, BOTH models' reports for visual rounds5–20 (R18 Opus: round-18-opus-unavailable.md; R19 Opus: round-19-opus-deferred.md), ALL visual-round-6/7/8/10/11/12/13/15-changes.md and visual-round-18/19/20/21-changes.md. R10 Opus only recorded quota unavailability, not scores. Read nine-validation.md for current verification context; tests do not determine visual scores. The original workflow rounds1–4 were old-base historical results.

Score0–10, independently LIGHT and DARK for EACH fixed category (twelve scores total):
Typography; Composition and spacing; Color harmony; Contrast and emphasis; Component visual consistency; Responsive visual polish.
8=polished/coherent without substantial visual issues;9=excellent, intentional, harmonious and carefully finished on material surfaces, with only rare trivial refinements;10=exceptional. Judge the weakest material surfaces, not an average or just the latest diff. A utility interface can earn9 through clarity and craft without decoration/new features. BOTH models must give all twelve>=9 in TWO consecutive joint rounds on unchanged application. Target9 incoming joint streakZERO. Prior all8 passes do not qualify. Give honest sub9 scores whenever warranted and say precisely what treatment would earn9.

All earlier findings, responses and unresolved objections:
- R5 rebased baseline exposed foreign dark editor surfaces, weak gutter/doctype/placeholder contrast, Arial vs system controls, loud dark borders/default tiles, nested hierarchy, misaligned card/history/settings/tab facts, phone Share towers, Search clear/owner/casing. 7eaf8354 introduced shared typography/fields/controls, quiet structural borders vs editable boundaries, integrated source surfaces, readable metadata, aligned rows/icons, restrained dark default tiles, contextual Search/clear/owner chip, stacked phone Share, preserving upstream global Search/fixed tabs/scroll/outside-press.
- R6/R7 exposed orphaned Share helper, Copy36 vs URL44, title casing and clipped People prompt. a242183a/4d725ba2 made paired44 controls/short visible helper/casing and explicit phone flex layout; choices/tag fields/footer filled the width. R8 both all8.
- R9 found missing LIGHT syntax due nonfallback override suppressing basicSetup fallback. 1c86da10 restored full explicit readable light syntax plus dark specificity and differentiation/contrast regression.
- R10 Sol found stock focused selection overriding app colors; Opus no-score quota. 7e96d783 matched actual native hierarchy; real selection rgb219,231,251 light /39,54,74 dark.
- R11 Opus found opaque active-line paint above selection. b1b43d96 changed active paint to25% alpha; actual selected-word/layer-composition regression verified visible distinction and readable token contrast in both editors/all widths/light-dark-light.
- R12 found lime word echoes dominating selection in light and tiny native Find9.1px/15–16px gradient controls. 30bdaa5d set8% accent echoes, stronger selection/current result, integrated native Find/Replace/Go/completion/detail/arrow/bracket/fold/special-char chrome with readable flat controls, phone44 targets, retained240px source viewport and native behavior.
- R13 both all8; R14 Opus newly found bold Shared with me wrapping at desktop (44→53.3px row, shifts next9.3px) while Sol omitted desktop shared-space check. 01109602 widened desktop rail208→240 without nowrap/clipping and strengthened actual text-line/stable-position Chromium assertions; phone tabs unchanged. R15/R16 both all12=8 on that unchanged application, completing old8 milestone. Full validation passed, no recurring automation remains. This does not satisfy9.
- R17 NEW9 baseline on01109602: Sol light/dark[8.5,8,9,8,8,8]; Opus light[8,8,8,8,8,8],dark[8,8,8.5,8,8,8]. Both identified inconsistent field labels/helpers/checkboxes/placeholders, conflicting dialog footer layouts, dense Share and secondary actions, weak narrow Tag Save/Delete hierarchy, page/header/card alignment, native chrome finishing. Sol additionally discovered desktop Versions176px track vs178.18px required -> View/Restore wrap into96px row.
- R18 234da736 response: shared sentence-case600 captions/helper rhythm/upright prompts/native-semantic18px checkboxes,44 standard/36 compact controls; common26px/650 page titles/top spacing/visible Library return. Duplicate/Tag/Share share fixed header/footer plus scrolling body; trailing desktop primary and explicit full-width primary/secondary/quiet destructive phone stack. Short visible Share warning/rules, body-size section heads,44 Replace/Revoke, no unused15px gutter, sticky Done. Intrinsic desktop Versions actions, tinted Current badge/readable explanation; full-width centered buttons. Find field fills to fixed Close, whole option units, consistent Close, centered completion with8px viewport inset. Final parent-caught Previous word split corrected with deliberate30/40/30 navigation allocation and strengthened rendered line-count assertion. Card optical anchors/owner inset aligned; Search visible/active in desktop rail; shorter balanced empty helper/44 Clear; persistent selected Add tint; light secondary neutrals stay slate; muted preset suggestions with selected/custom state. Shared Stimulus color-picker only owns working state; existing tag API/htmx retain save/render. New module and info icon explicitly in build registries. See full change note.
- Existing source syntax/selection/active alpha/echo hierarchy intentionally unchanged. Current9 is not promised merely by implementing these repairs.

Unresolved/retained judgment calls:240px rail remains31% at768 but two232px cards fit; Sol17 called further rail work optional. Quiet source echoes/active hint accepted as hierarchy/contrast tradeoff, but reassess. New field/sheet/preset grammar must itself earn9 across surfaces. Remaining native completion detail/special-character/bracket variants, arbitrary long data and real hosted identity were not exhaustively covered earlier. No physical-phone/alternate-engine claim. Necessary visible shortcomings remain scoreable even if old reports called them minor or newly implemented.

Live review method:
1 preview_status, then OWN preview_open(reuseExistingTab:false,open:false,url); exact tabId every call. If auth needed use scratch above.
2 Explicit real preview_set_appearance light/dark and matchMedia('(prefers-color-scheme: dark)').matches proof at1280/768/390/320. No root-class simulation. Reconfirm current assets.
3 Broad representative Library/cards, incoming/outgoing shared spaces with active labels, Search blank/results/miss (run, packing, not-in-this-library), Add all methods, Settings, editor Tags/history/source, viewer toolbar/popovers, card menu, Duplicate, Share. Open/cancel only; no persisted mutations. Native Find/Replace/Go/completion are normal source UI and IN SCOPE. Recheck retained source selection/syntax as needed but do not spend nearly all the review repeating only its paint matrix.
4 Judge rendered composition, typography, color, emphasis, shared component details and meaningful responsive finishing. Distinguish observed coverage, supplementary saved images and source inference. Screenshots alone/test passes do not prove quality.
5 Write all twelve numeric scores plus pass/fail, route/theme/viewport evidence for material findings, prior-repair verification, minor opportunities and honest coverage/limits. For sub9 explain the concrete remaining visual weakness and coherent treatment; avoid arbitrary ornament/new features. No council-completion claim from one model.


LATEST PRIOR FINDINGS/RESPONSES:
R18 Sol[9,7,9,8,7,7] BOTH schemes: typography/harmony excellent, but Share FIELDSET.modal-body reported overflow and refused actual wheel/PageDown/scrollTop; lower settings clipped, positioned radios painted over pinned Done. Parent independently reproduced this. Opus18 failed BEFORE scores at actual5-hour limit, reset19:50UTC Nov4. R19 Opus was deferred and never launched; no assumed scores.
R19 a65d7c6b/b1ef638450cc changed all three sheets to a regular flex scroll DIV with semantic fieldset INSIDE. Stable #share-body htmx fieldset/native pending/focus kept. Actual scroll/paint/footer/lower-disclosure E2E first failed old binary then passed both schemes320/390. Parent and Sol19 independently verified complete lower Share at1280/768/390/320, scrolling306.5/306.5/389/556px, no detached native radio, fixed footer, final expanded paragraph reachable. Source matrix30s timeouts under concurrent heavy jobs pass unchanged in unloaded recheck17.7s; no timeout/assertion relaxed. Stale Go markup checks updated; focused4 pass1.307s. R19 full15 E2E pass44.3s; native modal transactions/focus/pending/swap group6.292s.156 authentic captures, allb1ef638450cc, no page errors/document overflow,8 wheel-scrolled lower Share.
R19 Sol independently scored BOTHschemes[9,8.5,9,9,9,9]. Remaining composition issue: independent per-row max-content action cells Current77.91px vsView/Restore186.35px cause identical timestamps to start824.35 vs715.91,108.453125px apart at1280/both. Buttons now fit, but table facts stagger. Read round-19-sol-visual.md; do not inherit its scores.
PRIOR R20 8504bc51/33556024040a response: parent history list shares intrinsic tracks, desktop rows use subgrid; widest date/action group determines shared columns. Phone independent grouping unchanged; no fixed176px track, shrinking text or decorative work. Existing history E2E strengthened to compare actual date/action x positions across3 versions at1280/768 both schemes while retaining320 enlarged-text/viewer control checks. First failed old binary at108.453125px drift, then passes3.0s. Parent1280 actual light/dark: both dates715.90625, action cells859.6484375, older action pair same y. Actual before/after shown/saved nine-history-comparison.html. No other app visual change. Read visual-round-20-changes.md.

Remaining/coverage judgments from19: generous240px rail optional, two232px cards comfortable at768; faint source echoes/active band are intentional hierarchy; actual selectedmeta leads and allsource28568/All changes saved. Completion opened in existingCSS at27:8,1280dark/390both/320both: centered/readable, x50.55–312 at320 (8px inset), no acceptance. Property-type square marker utilitarian optional. Small card-kebab optical differences optional, not a table-structure issue. Various long/enlarged/invalid/dirty/post-save states and native detail/special-character/arbitrary-bracket variants remained sampled, not exhaustively certified. Reassess honestly; these are not exemptions.


LATEST R20 FINDINGS: BOTH reports are now prior-round context and must be read.
Sol all12=9; Opus bothschemes[8.5,8.5,9,9,8.5,9], so JOINT FAIL/streakZERO.
Opus independently verified history alignment and actual Share scroll/paint,
but discovered the card/viewer posture receipt had12px/normal cramped text,
inconsistent10.5px uppercase vs12px headers and no viewport collision handling:
card popup y578–852 grew800px document to852, exposed rail ground below scroll.
Other material findings: Share Replace/Paste footer off-token12px helpers,
People helper two gray tones,12/700 tag group heading below13/600 captions,
Tag well/hex row7px inset and native pure-gray frame. Smaller items: Share42px
checkbox/helper gap, Filters missing shared divider/inset8px, phone shared tab
orphan me, Duplicate orphan enabled., desktop kebab6–8px low.
Current R21 response: READ visual-round-21-changes.md in full for implementation,
rendered measurements, verification and limitations. Registered Stimulus panel
placement, shared helper/header rhythm, native color-row/frame alignment,
standard sheet/checkbox description, balanced tabs, and real32px desktop kebab
slot fix address these findings. These changes do not determine your scores.

Current verification: build passes;468 Node pass16.292s on currentJS;
16 E2E pass59.9s; focused modal/navigation Chromium20.531s. Full Go short and
188 authenticated current captures are being refreshed on frozen source; read
nine-validation.md for actual current status. Old156 captures on33556024040a
are historical. Read current source/live views; tests and screenshots cannot
substitute for independent rendered judgment. Do not run builds/tests yourself.
Current source stays frozen throughout this joint round, later evidence commits
allowed. No recurring automation remains.

Review the ENTIRE material app at the same honest9 bar in real LIGHT/DARK at
1280/768/390/320 with your own T3 tab and matchMedia proof. Include actual
card/viewer posture receipts, long helpers/form/color controls/Filters, history
columns/phone grouping, actual Share scroll/paint/lower disclosure, native source
Find/Replace/Go/completion/selection, Library/Search/Settings/Add/navigation and
menus/dialogs. Do not restrict review to the last fix or demand ornament/new
features. Judge the weakest material surfaces; report precisely what would
earn9 for any sub9 rating. Preserve limits and distinguish actual fresh coverage,
prior/supplementary images, and source inference. Read-only except assigned
unique report; no valid fixture mutations. Restore temporary drafts/close tab.
Do NOT read round-21-sol-visual.md or round-21-opus-visual.md except your OWN
assigned output, or parent scores/summary/resume/ticket. Prior completed reports
through20 (both20) are permitted. Fresh scores must be independent. Your single
report cannot establish council completion. Return report path and all12 scores.
