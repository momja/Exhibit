# Exhibit visual council: target 9/10

The user raised the goal: **"Now climb to a 9/10."** Continue the existing UI
council in the same feature worktree. The completed8/10 milestone is historical,
not a passing result for this new target.

## Fixed scope and scoring

Original brief: iterate Exhibit's UI for consistency and stellar UI/UX, with
independent Codex Sol and Claude Opus scoring each round. Show actual before/after
screenshots. Preserve the latest-main rebase and evaluate actual
prefers-color-scheme light and dark. This phase remains UI-only.

Keep the six categories, each scored independently in light and dark,0–10:

1. Typography: scale, weight, hierarchy, reading comfort and consistent labels.
2. Composition and spacing: alignment, rhythm, density and deliberate balance.
3. Color harmony: coherent palette, surfaces and purposeful color.
4. Contrast and emphasis: legible text/states and clear action hierarchy.
5. Component visual consistency: shared fields, buttons, cards, menus, dialogs,
   and native source-editor chrome.
6. Responsive visual polish: coherent1280/768/390/320px compositions, not merely
   absence of overflow.

8 remains polished/coherent with no substantial issue. **9 means excellent:**
intentional, harmonious and carefully finished on material surfaces, with only
rare trivial refinements.10 is exceptional. A9 utility interface need not add
ornament, new features or a different brand. Do not inflate scores because the
user raised the target; rate the weakest material surface, not an average.

Completion requires BOTH models to award all twelve light/dark scores>=9 in
TWO consecutive joint rounds on unchanged application code. All8 scores from
rounds15/16 do not count. Incomplete/provider-limited reviews are not passes.

## Baseline and scope

Starting application0110960247e5fdece7f88699c85afb865824f8e6, source version
150ec936001f, base origin/main44def51a. It scored8 in every category from both
models in two rounds. Existing110 captures in visual-round-15/ are the actual
unchanged baseline. Preserve existing public/shared workflows and upstream
Search, fixed tabs, scroll chrome and outside-press dismissal.

App-owned Library/shared spaces, global Search, Add methods, Settings,
editor/source/history/tags, viewer chrome/popovers, card menus, Duplicate and
Share are in scope. Arbitrary user-authored tools/widgets and credential-dependent
agent execution are excluded. Do not judge a light user tool body as app dark UI.

Known opportunities from the8/10 reviews: dense Share explanation and compact
secondary actions; inset Revoke; narrow Edit tag footer/strong Delete; mismatched
labels/checkbox sizes in Duplicate; uneven placeholders and clipped empty Paste
prompt/tag selector; source completion alignment, panel Close styling and unused
Find space; left-aligned full-width Return to Library; weight mismatch in Settings;
weak light Add-method selection; card-menu vertical alignment; Search empty-copy
orphan and small Clear action; generous desktop rail at768. Reassess rather than
assuming fixing these alone automatically earns9.

## Review method

Every round uses fresh delegate_task calls, shared isolation, async review role,
unique stable execution request IDs, and retained actual task IDs. Include the
original brief, ALL earlier findings/responses and unresolved objections. Do not
continue through childThreadId or top-level threads. Reviewers must not read
same-round peer reports or parent aggregate/checkpoint files.

Use own T3 tabs, explicit light/dark emulation with matchMedia proof and actual
rendered observations at1280/768/390/320. Be clear about sampled vs exhaustive
coverage, source inference and screenshots. Do not substitute tests for scores.
Read-only except the assigned report; no valid saves/creates/deletes/grants,
fixture changes or user-content edits. Temporary unsaved specimens must be cleared.

Preview http://t3code:18280, render18281, scratch admin/changeme. Run Log
/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332 and /edit. Existing tests and
capture workflow remain the regression foundation. No app edits while reviews
run. Keep main clean/on main; no merge or PR requested.
