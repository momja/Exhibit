---
id: Exh-zrjo
status: in_progress
deps: []
links: []
created: 2026-11-04T06:41:33Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, ux]
---
# Iterate on UI consistency and visual craft to9/10 with Codex and Opus council

Audit and improve Exhibit interface in a dedicated worktree. Use independent Codex Sol and Claude Opus council reviews to score visual hierarchy, consistency, navigation/discoverability, workflow feedback, accessibility, and responsive behavior. Iterate until both reviewers award every category at least 8/10 in two consecutive rounds. Preserve the server-template, htmx, Stimulus, and self-hosted Phosphor architecture.

## Acceptance Criteria

Current user follow-up: both Codex Sol and Claude Opus independently award all six visual-only categories at least9/10 in each actual light/dark scheme (12 scores/model), in TWO consecutive JOINT rounds on unchanged application code. Relevant current browser and Go checks pass; actual before/after screenshots, scores, task IDs and coverage limits are saved; main remains clean/on main. The earlier8/10 milestones remain completed history and do not satisfy this target.

Earlier completed follow-up: rebase onto latest origin/main, evaluate actual
prefers-color-scheme light/dark, and repeat the council with six visual-only
categories. Each model must give all twelve light/dark scores at least 8 in two
consecutive joint rounds. Old-base workflow scores do not satisfy this follow-up.


## Notes

**2026-11-04T07:27:55Z**

Completed four independent Codex Sol / Claude Opus review rounds. Both models scored all six categories 8/10 in rounds 3 and 4 on frozen application commit 006b14ef. Full Go short suite passed (API418.4s), 459 Node tests passed, 9 E2E tests passed, broad Chromium matrix passed (81.1s). Saved 36 authenticated final screenshots plus baseline and intermediate comparisons. Evidence: docs/reviews/Exh-zrjo/summary.md and scores.json. Preview isolated on t3code:18280. Main worktree remains clean.

**2026-11-04T08:58:33Z**

Rebased onto origin/main 44def51a and continued with a separate light/dark visual rubric. The old completion note describes historical workflow reviews. Visual R9 exposed missing light syntax highlighting; R10 exposed stock focused-selection fills overriding the app theme. Both are corrected in 7e96d783. All 12 E2E tests pass (23.5s), including actual selections in Paste and the artifact editor at 1280/390/320 through light/dark/light; 74 current captures are saved. Sol R11 is reviewing; Opus hit its five-hour usage limit and resets at 09:50 UTC. A self-removing continuation trigger is scheduled for 09:51 UTC on November 4. Joint passing streak remains zero; ticket remains in_progress until two joint passes. Current evidence and handoff: docs/reviews/Exh-zrjo/visual-summary.md and visual-resume.md.

**2026-11-04T09:11:55Z**

Sol round 11 completed: every visual category is 8/10 in both light and dark on application 7e96d783. Independently verified actual focused selections in Paste and existing source at 1280/390/320; minimum sampled selected-token contrast 4.523:1 light / 4.696:1 dark. Full current Go short suite passed (API408.314s, store174.280s), all 12 E2E tests passed, and 74 captures are committed. Joint streak remains zero pending the deferred Opus round-11 phase after quota reset; a fresh joint confirmation round is still required. Scheduled continuation 09:51 UTC Nov4 deletes its trigger before resuming. Keep current application frozen.

**2026-11-04T11:23:12Z**

The temporary quota-resume trigger was deleted after firing; no recurring automation remains. R11 Opus exposed opaque active-line paint hiding selections, and R12 Opus exposed lime repeated-word hints plus tiny native find controls. Candidate 30bdaa5d now themes all standard editor chrome and preserves visible selection. All 14 E2E tests pass (42.7s), focused Chromium passes (15.380s), and 104 actual light/dark captures are saved. R13 Opus awards all twelve scores 8; Sol is still running, so joint streak remains zero pending its verdict. Application stays frozen for independent review. Evidence: docs/reviews/Exh-zrjo/visual-summary.md.

**2026-11-04T11:27:56Z**

R13 is the first qualifying joint pass on application 30bdaa5d: both Sol and Opus independently award all twelve visual light/dark scores 8. All required checks pass, including confirmed full Go exit 0. Fresh R14 confirmation tasks are running for both models on the unchanged application; ticket stays in_progress until the second consecutive joint pass. Scores, reports and actual task IDs: docs/reviews/Exh-zrjo/visual-scores.json.

**2026-11-04T11:40:45Z**

R14 Opus scores Composition and spacing 7.5 in light and dark after finding a preexisting active Shared with me desktop rail wrap. Parent confirmed row53.3px vs44px and a9.3px shift below it. The existing GallerySpaces browser assertion now reproduces the issue; it previously checked width only. Sol is still reviewing the unchanged served build. Joint streak resets to zero; production fix waits until that independent review ends.

**2026-11-04T11:51:01Z**

R14 Sol independently scored all twelve categories8, but Opus composition7.5 in both themes makes the joint round fail. Fixed desktop rail alignment in application01109602 with one CSS declaration (240px), preserving phone tabs/natural enlarged-text wrapping. Strengthened existing browser regression first failed old UI, then navigation/empty/tab/enlarged-field group passed44.657s; filters/scroll/readability passed21.234s; all14 E2E passed1.8m;110 captures saved. Actual navigation before/after shown. Fresh independent R15 Sol+Opus tasks running; streak0. Full current Go short run pending. Evidence: docs/reviews/Exh-zrjo/visual-scores.json.

**2026-11-04T12:08:04Z**

R15 is a joint pass on unchanged application01109602: both models award all twelve light/dark scores8. Fresh R16 Sol is running. R16 Opus failed without scores at its five-hour quota; detailed activity says resets14:50UTC Nov4. One-use continuation scheduled14:51UTC, implemented as a temporary daily trigger whose FIRST action is deleting itself. Only missing Opus phase will be retried; no duplicate Sol review. Streak1, confirmation incomplete, ticket remains in_progress. All current validation passes. See docs/reviews/Exh-zrjo/round-16-opus-unavailable.md and visual-scores.json.

**2026-11-04T12:16:11Z**

Sol R16 independently confirms all twelve visual light/dark scores8 on unchanged application01109602. Its report and terminal result are saved; do not duplicate that phase. Only Opus confirmation is missing after its14:50UTC quota reset. One-use continuation remains scheduled14:51UTC and deletes its temporary entry FIRST. Streak remains1; ticket stays in_progress. All current validation passes.

**2026-11-04T14:53:34Z**

The14:51UTC continuation deleted its temporary daily trigger FIRST; list_scheduled_tasks confirmed no remaining automation. Application source is unchanged from01109602, matching completed Sol R16. Fresh post-reset Opus confirmation is running under a new retained task ID, with the original no-score failure preserved. Sol current-round report/scores were withheld. No validation reruns are needed; all checks already pass. Streak1 pending actual independent Opus scores.

**2026-11-04T15:05:21Z**

Completed the rebased visual-only follow-up. Both Codex Sol and Claude Opus independently scored all six categories8/10 in light and dark in consecutive rounds15 and16 on unchanged application0110960247e5fdece7f88699c85afb865824f8e6 (origin/main44def51a). All48 actual report scores were verified. Opus final confirmation was a fresh independent post-quota execution, with Sol same-round results withheld; original no-score failures remain recorded. Build,468 Node tests on unchanged scripts, editor lint,14 E2E tests, focused Chromium44.657s+21.234s and full Go short suite (exit0, API439.355s/store197.975s) pass.110 authenticated captures and actual before/after comparisons are saved. Both temporary scheduler entries were deleted; no recurring automation remains. Outcome: docs/reviews/Exh-zrjo/visual-summary.md; scores and task IDs: visual-scores.json; validation: visual-validation.md. Preview http://t3code:18280. Feature branch retained, main clean/on main, no merge or PR.

**2026-11-04T15:33:14Z**

User raised the target to9/10 after the completed8/10 milestone. Continue in the same isolated feature worktree with the same six visual-only categories, independent light/dark scores, and two consecutive joint Sol+Opus passes at9+. Preserve the completed8/10 evidence as history. New progress: docs/reviews/Exh-zrjo/nine-summary.md and nine-scores.json. No recurring automation.

**2026-11-04T16:48:18Z**

Target9 work continues on frozen application a65d7c6b/assetsb1ef638450cc. R17 both below9; R18 Sol rated typography/harmony9 but found Share fieldset scrolling/clipping failure, independently confirmed by parent. Opus R18 failed before scores at actual five-hour quota, reset19:50UTC Nov4. Fixed regular-block scroll container around semantic fieldsets; actual scrolling/paint/footer/lower-disclosure regression fails old binary and passes new. All15 E2E pass44.3s; native modal group passes6.292s;468 Node/editor lint carry forward unchanged JS.156 actual light/dark captures verified with zero page errors/overflow. Fresh Sol R19 running; no joint9 pass assumed. Full current Go short running. One-use19:51UTC continuation uses temporary daily trigger that deletes itself FIRST, then only missing Opus review on current frozen source if eligible; do not duplicate Sol or expose same-round peer scores. Main clean/on main. Current evidence: docs/reviews/Exh-zrjo/nine-summary.md, nine-scores.json, nine-validation.md and nine-resume.md. Actual before/after shown and saved.

**2026-11-04T17:01:18Z**

R19 Sol independently gave five categories9/10 in both schemes and composition8.5 for108.453125px staggered Versions timestamps. Opus R19 was never launched before quota reset; no stale review will be filled. Application8504bc51/assets33556024040a now shares intrinsic desktop history columns, retaining phone reflow and one-line View/Restore. Strengthened existing regression fails actual old UI then passes current1280/768 bothschemes plus320 enlarged text/viewer. All15 current E2E pass43.5s. Actual history before/after shown and saved. Fresh Sol R20 running; missing Opus R20 deferred to actual19:50UTC quota reset, one-use19:51 continuation reads current checkpoint and deletes its own temporary daily trigger FIRST. Streak0. Full Go current runs detached with durable status after prior signal143 interruption;156 current captures refreshing. See nine-resume.md/nine-scores.json; do not duplicate active/completed Sol or expose its same-round scores to Opus.

**2026-11-04T17:07:47Z**

Sol R20 completed independently on unchanged8504bc51/assets33556024040a: all twelve visual light/dark scores9. It freshly verified history shared columns, real Share scrolling/paint at all four widths/bothschemes, native source chrome/completion and representative broader surfaces; limits saved. Full current Go short now confirmed PASS exit0 in431.7s with applicationUnchanged=true; all15 E2E pass43.5s;156 current captures verified. Keep source frozen. Opus20 has not run, so joint streak remains0/2.19:51UTC one-use continuation deletes its temporary daily trigger FIRST, launches ONLY missing independent Opus20 (same-round Sol report/scores withheld), then requires a fresh joint confirmation if it passes. Never duplicate Sol20 or claim target9 completed from this one-model result. Evidence: nine-summary.md, nine-scores.json, nine-validation.md, nine-resume.md.

**2026-11-04T19:52:44Z**

19:51UTC continuation deleted its temporary daily trigger FIRST; no scheduled automation remains. Source still8504bc51/assets33556024040a, unchanged from completed Sol20. Fresh independent post-reset Opus20 is running with Sol same-round report/scores and parent aggregates withheld. Actual taskId node:delegated-task:command%3Amcp%3A9913d16e-75eb-46de-9b1d-e9986db72056%3Adelegate-task%3AExh-zrjo-r20-opus-visual-nine-after-reset-2026-11-04 retained in nine-scores.json; safe supplied brief saved. No duplicate Sol, builds, suites or captures. All checks already pass; joint streak0 pending Opus, then a fresh joint confirmation is required if it passes. Keep application frozen.

**2026-11-04T20:27:45Z**

R20 terminal joint failure: Sol all twelve9, Opus typography/composition/component consistency8.5 in light/dark. Current application74544552 repairs posture popover readability/viewport placement and shared helper/color/filter details. Actual before/after shown/saved nine-posture-comparison.html. Both fresh independent R21 tasks running, each current peer output withheld; task IDs saved in nine-scores.json. Build,468 Node tests,16 E2E59.9s and focused Chromium20.531s pass. Current full Go and188 captures running. Joint target9 streak0; ticket remains in_progress. No recurring automation remains. Main stays clean/on main.

**2026-11-04T20:47:46Z**

R21 BOTH finished below9: Sol identified cramped Stored state/assets explanations and zero paragraph spacing; Opus identified Share opening scrolled351px on320×640. Current application17baf930 fixes both and minor helper/Tag-height/posture-target mismatches. Actual before/after shown/saved nine-opening-comparison.html. Build,468 Node16.435s,17 E2E1.1m and focused Chromium15.307s pass. New opening regression first fails old351, catches hidden-reset reopen633, then passes32 mouse/touch first/reopen states. Fresh independent R22 both running; actual task IDs saved. Current fullGo/220 captures running. R21 fullGo confirmedexit0/456.95s preserved as historical. Streak0; no recurring automation; main clean/on main.

**2026-11-04T21:11:32Z**

R22 BOTH below9 on remaining editor action grammar; previous fixes verified. Current5f1021e3 pairs transfer controls/help, gives Save priority and Add secondary, quiet outlines for Erase/asset Delete,44px phone panel actions, square/right-aligned inspector utilities and Widget help gap. Actual desktop/phone light/dark before/after shown/saved nine-editor-actions-comparison.html.18 E2E1.3m on layout; final quiet classes focused pass3.5s; Go Chromium19.993s;468 Node/lint carry unchangedJS. Fresh BOTH R23 independent tasks running; IDs saved, source frozen. Full current Go/228 captures running. Prior17baf930 fullGo confirmedexit0/454.29s and220 captures preserved as historical. Streak0; no recurring automation; main clean/on main.

**2026-11-04T21:22:03Z**

R23 Opus failed BEFORE scores at actual five-hour limit, reset00:50UTC Nov5 (epoch1793839800). One-use continuation scheduled00:51UTC Nov5 via a temporary daily trigger that FIRST deletes its exact ID; dedupe eligible Sol and retry only missing fresh Opus phase, no invented scores. Sol23 still running; source5f1021e3/c948e619826d frozen. Original capture terminated143 after177 PNG/no manifest; fresh detached retry passed276.1s,228 verified actualschemes/assets/errors[] and incomplete output cleaned after proof. FullGo exit1/466.88s has one stale assertion still requiring old filled Erase; after Sol terminal update explicit expectation to quiet outline, focused+full rerun, no production UI change. Streak0, ticket in_progress. Current checkpoint nine-resume.md.

**2026-11-04T21:26:56Z**

Sol R23 COMPLETED independently with all twelve scores9 on unchanged runtime application5f1021e3/c948e619826d. Retain it; no duplicate Sol phase. Joint streak remains0 because Opus failed without scores before its00:50UTC Nov5 reset; one-use00:51 continuation launches only missing fresh Opus phase, with same-round Sol result withheld. After Sol terminal, validation-only commitffdbbaa5 updates the stale filled-Erase assertion to explicit quiet-outline expectation; focused test passes0.263s. Full Go confirmation runner77627 now running with production-source/test-only distinction recorded; no app rebuild or UI change. Capture clarification: original wrapper143 child later finished228-manifest; confirmed retryexit0 remains final evidence, duplicate original228 PNGs removed after validation. Current checkpoint/safe Opus brief saved.

**2026-11-05T00:55:52Z**

Nov5 00:51 continuation FIRST permanently deleted its temporary daily trigger; list_scheduled_tasks confirmed[]. Runtime5f1021e3/assetsc948e619826d unchanged except known validation-only test. Full Go confirmation now verified exit0/409.34s (API395.914/store166.248), applicationUnchangedtrue; all current checks pass. Sol23 all twelve9 retained without duplication. Fresh independent post-reset Opus23 running under actual taskId node:delegated-task:command%3Amcp%3Abb622e97-24f4-48e5-90ab-d51f24f155e2%3Adelegate-task%3AExh-zrjo-r23-opus-visual-nine-after-reset-2026-11-05. Same-round Sol scores/report and parent aggregates withheld; both original failed/new actual attempts saved. Streak0 pending actual Opus, then fresh BOTH confirmation if joint pass. No recurring automation or source change.

**2026-11-05T01:19:19Z**

Fresh post-reset Opus23 completed below9: component consistency/responsive8.5 bothschemes, despite Solall9; no joint pass. Fixed tag action slots32desktop/44phone,44 Add property and full phone tag primary in329c50aa/assetsbc0938a74a48. Broader role probe caught boolean label18px; now44; asset utilities share square slot. Actual before/after shown/saved nine-tag-controls-comparison.html. Extended tools regression fails old20/18, then passes5.2s, native Tag Edit/Cancel/focus and Move/Cancel/no persistence. Full18 E2E1.3m and focused Chromium19.103s pass; Node/lint carry unchangedJS. Fresh independent BOTH24 running, actual IDs saved, source frozen; current fullGo/236 captures running. Streak0, no timers, main clean/on main.

**2026-11-05T01:26:45Z**

R24 Opus completed independently below9: typography/emphasis/component consistency8.5 in both schemes after finding Network & permissions uses12/700 headings,16/400 capability labels and filled Add versus shared secondary actions. Parent confirmed live1280/320 bothschemes and saved BEFORE screenshots; no source edit while Sol remains independently active. Current fullGo confirmedPASSexit0/482.6s/applicationUnchangedtrue,236 actual-scheme capturesPASSexit0/313.98s/currentassets/errors[]. All current checks pass. Streak0, ticket in_progress. Next source repair waits for Sol terminal; no duplicate review or recurring automation.

**2026-11-05T01:40:52Z**

R24 BOTH terminal: Solall12=9; Opus typography/emphasis/component consistency8.5 bothschemes makes jointFAIL. Current applicationd30dbbe9/assetse29fcce0e122 aligns Network & permissions groupheads/captions/secondary Add+Allow, persistent origin label/full example, server-rendered empty helper, phone origin/action grouping and first-line caret. Actual before/after shown/saved nine-permissions-comparison.html. Extended existing regression first fails old12/700 heading then passes shared roles/all4widths/actualschemes/draft htmx empty transitions/no saved permission change. Full18 E2E confirmedexit0/78.53s; native draft/navigation/permission-save/modal group confirmedexit0/24.32s (package21.790). Initial wrappers143/port collision retained; verified orphan scratch server cleaned, no preview impact or port-reuse bypass. Fresh BOTH25 independent reviews active, actual IDs retained in nine-scores.json, same-round peers withheld. Current fullGo/244 captures pending, source frozen. Streak0, no recurring automation, main clean/onmain.

**2026-11-05T01:47:02Z**

R25 Opus independently COMPLETED on unchangedd30dbbe9/e29fcce0e122: all twelve visual light/dark scores9. It verifies Network & permissions shared grammar plus a local unsaved origin row removed back to All changes saved; exact sampled limits retained. Sol25 still active, no duplicate or peer-score leak; joint streak remains0 pending both. Current244 authenticated captures confirmedexit0/312.98s/applicationUnchangedtrue, all PNGs/manifest244/currentassets/actualschemes/errors[]. Current fullGo pending, browser checks already pass; source frozen. No recurring automation.

**2026-11-05T01:48:29Z**

Current fullGo completed exit1/451.04s/applicationUnchangedtrue with exactly two stale gallery_test.go template assertions: Blocked still expects the old text-sm/muted h3 override, External links still expects a spacer span without field-label. Failed log/status retained. No browser regression observed; current18 E2E/native browser/244 captures pass. Wait Sol25 terminal, then update those explicit expectations/comments and run affected tests plus full Go confirmation; no rendered UI change or duplicate eligible review required. Opus25 all12=9 remains independent; joint streak0 pending Sol.

**2026-11-05T01:54:41Z**

R25 is the FIRST qualifying joint9 pass: BOTH Sol+Opus independently all twelve visual light/dark ratings9 on unchangedd30dbbe9/e29fcce0e122. Fresh BOTH26 confirmation running with unique actual task IDs in nine-scores.json, all prior context through25 supplied, current peers/parent aggregates withheld. Streak1/2, no completion claim. After BOTH25 terminal, validation-only commita787bd5e updates two stale gallery_test.go expectations to explicit shared h3/field-label roles; focused tests pass0.028s, no production UI/asset/API/behavior change. Full standard Go confirmation detachedPID108262 pending durable receipt; source proof excludes ONLY exact test file. Current18 E2E/native browser/244 captures already pass, no redundant repeats. No recurring automation, main remains clean/onmain.

**2026-11-05T02:03:56Z**

R26 Opus independently confirms all twelve light/dark ratings9 on unchangedd30dbbe9/e29fcce0e122; exact coverage/minor suggestions preserved. Sol26 still active, no same-round leak or duplicate; joint streak remains1/2 pending actual verdict. Full current Go confirmation now PASSexit0/423.55s/applicationUnchangedtrue/exact validation-only gallery_test.go scope verified. Canonical current log/status saved, R24 prior canonical archived, original R25 failure retained. ALL required checks pass:18 E2E78.53s, native browser24.32s, focused expectations0.028s,468 Node/lint carry unchangedJS,244 verified actual-scheme captures312.98s. No rebuild/source change or repeated passing suites. Only Sol26 outcome remains before potential second joint pass/closure. No recurring automation.

**2026-11-05T02:12:22Z**

COMPLETED target9. BOTH Codex Sol+Claude Opus independently award every fixed visual category9 in actual light/dark in TWO consecutive JOINT rounds25/26 on unchangedd30dbbe92f9f5535a33f59be25f7c9f9c4be4511/assetse29fcce0e122. All48 original report-table ratings verified against ledger; actual task IDs/statuses and coverage retained. Sol26 accidental historical aggregate Git diff audited: compared HEADa787 predates26 tasks/outcomes, no same-round peer result exposed; procedural deviation retained. All checks pass: build,18 E2E78.53s,native browser24.32s,expectations0.028s,full Go423.55s(API410.584/store182.020),468 Node/lint carry unchangedJS,244 authenticated real-scheme captures312.98s. Only later source diff is test-only gallery_test.go expectation updatea787; runtime/assets unchanged. Failed/incomplete attempts preserved, actual before/after comparisons shown/saved. Final scheduler listempty; no recurring automation. Final evidence docs/reviews/Exh-zrjo/nine-summary.md,nine-validation.md,nine-scores.json,nine-final-rating-verification.json. Close on feature, main stays clean/onmain, no merge/pushmain/PR.

**2026-11-05T02:37:06Z**

User reported concrete post-council misses with screenshots: selected preset check badge touches/mismatches the rounded swatch border; editable tag pills have awkward spacing; phone header Save is an unexplained checkmark with no persistent outcome feedback. Continue in same feature worktree, preserve earlier25/26 ratings as historical, fix these details and review the corrected candidate honestly. Main remains untouched/clean.

**2026-11-05T03:12:41Z**

User screenshot follow-up implemented on947758e0/b2069bfa62d2: inset/concentric selected swatch check, compact tag label with separate32/44 controls, explicit mobile Save with persistent sticky Saved/Unsaved/Saving/Review and visible issue/retry feedback. Save stays in editor; View separate. Real native workflow caught stale Review returning after resolved approval; reset fixed, regressionpasses18.45s. Final468 Nodepass16.743s,20 E2Eexit0/96.67s,native header/tag/navigation/permission/lifecycle/icon group exit0/31.53s.244 current captures confirmedexit0/315.28s/currentassets/actualmedia/errors[];14 current real Save-state captures and6+6 beforeafter saved/shown. FullGo still pending. Fresh Sol27 active; Opus27 failed BEFORE scores at actual five-hour quota, reset05:50UTC Nov5. One-use continuation scheduled05:51UTC Nov5 via temporary daily trigger that FIRST deletes exact task ID, then reads current checkpoint/dedupes and resumes only eligible missing phase. No score invented; new follow-up streak0/2. Current six visual +focused Save UX category each light/dark (14/model), two joint passes required. Historical25/26 milestone remains separate. Evidence followup-summary.md/followup-validation.md/followup-scores.json/followup-resume.md. Main clean/onmain, no merge/pushmain/PR.

**2026-11-05T03:14:36Z**

Current fullGo now CONFIRMED PASSexit0/573.41s/applicationUnchangedtrue on947758e0. All required local checks pass, including20 E2E96.67s,native combined31.53s,468 Node16.743s,244 captures315.28s and14 actual Save-state images. Full current log/status retained; no passing-suite reruns/rebuild. Sol27 still active, Opus27 failed without scores at actual quota; temporary one-use05:51UTC Nov5 continuation remains scheduled and FIRST deletes itself, then dedupes current state and runs only eligible missing reviewer. New follow-up9 milestone incomplete, streak0; original d30 ratings historical. Main clean/onmain.

**2026-11-05T03:20:41Z**

User requested reapplying all UI work stacked on PR273 (feature/Exh-77m9/gallery-tabs). Fetched current headab0135c9; same main44def51a is its ancestor, so restack directly onto its three commits. Preserve Starred/Search/Shared/Activity and new behavior. Sol27 completed all14=9 on old947758e0, Opus27 no-score quota; R27 now superseded by requested base change, not counted for stacked interface. Save report/history and backup branch before rebase; fresh BOTH nextunused28 after integrated validation. Timer checkpoint will be updated to current stack, never fill obsolete27. Main remains untouched/clean.
