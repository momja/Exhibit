# Visual-only council: rebased follow-up

User correction: rebase onto latest main and inspect actual `prefers-color-scheme: dark`. The council, rather than the user, should identify the visual problems. This pass concerns typography, composition and color; it does not use the previous workflow-heavy rubric.

Base: `origin/main` at `44def51a`. Only this task's four commits were replayed; unrelated local ticket commits were excluded. The rebased pre-polish application is `a65a7cfe`. Preserve upstream mobile Search, fixed tabs, scrolling chrome and outside-press dismissal behavior.

## Fixed visual categories

For each category, record a light score and a dark score, independently, from 0–10:

1. Typography: scale, weight, hierarchy and reading comfort.
2. Composition and spacing: alignment, rhythm, density and balance.
3. Color harmony: coherent palette and appropriate color use.
4. Contrast and emphasis: legibility, action hierarchy and restrained boundaries.
5. Component visual consistency: shared buttons, fields, cards, menus, dialogs and source editor.
6. Responsive visual polish: 1280px desktop, 390px phone and 320px narrow layouts.

8 means polished and coherent with no substantial visual issue. Score material weak surfaces honestly rather than averaging them away. Completion requires **both models to give every category in each theme at least 8 in two consecutive rounds**. Previous scores do not count under this new rubric/base.

## Scope and evidence

Library, shared spaces, global Search (blank/results/miss), Add methods, Settings, editor/source/history/tags, viewer toolbar/popovers, card menu, duplicate and share dialog. Judge app-owned designs; user-authored widget/tool content is outside this pass. Provider-dependent agent execution and experimental connectors are excluded. Existing behavioral checks still run as regressions, without affecting the visual rubric.

Reviewers must explicitly emulate dark/light, confirm the media query, and inspect rendered pages. The capture script now asserts the browser media query and includes desktop dark, Search and open UI overlays. Record route/selector/theme and observed vs source-inferred findings. Distinguish usability defects from aesthetic refinements.

Scratch app: http://t3code:18280 ; render port 18281. Login admin/changeme. Run Log: `/artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332` and `/edit`. Health/Reading/Utilities tags, Daily tools collection, incoming/outgoing shares and version history are present. No production data is used.

Reviewers use separate tabs and may cancel dialogs or submit invalid forms. No valid saves, creates, deletes or sharing changes. Each reviewer writes only its own assigned report.
