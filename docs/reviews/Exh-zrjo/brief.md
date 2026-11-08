# UI council brief

Improve Exhibit's core app interface for consistency and excellent everyday usability. Work is isolated on `feature/Exh-zrjo/ui-council`; no production data or services are used.

## Fixed review scope

Library and shared spaces, search and filter empty states, card actions and duplicate dialog, add-artifact methods and validation, artifact editor and viewer, Settings. Inspect desktop (1280px), phone (390px), narrow phone (320px), light and dark. Agent/provider connectivity and experimental connector configuration are outside this pass; do not score absent credentials as an interface defect. Artifacts/widgets are user-authored content: judge the surrounding app, not the sample tools' own designs.

## Scoring rubric

Score each category independently 0–10, with evidence:

1. Visual hierarchy: readable type, spacing, clear primary actions and page context.
2. Component consistency: shared headers, controls, surfaces, icons and interaction conventions.
3. Navigation/discoverability: find tools, reach actions, switch spaces, return from subpages.
4. Workflow feedback: clear state, validation, errors, empty states, pending/success and recovery.
5. Accessibility: semantic names, keyboard/focus, contrast, target sizes, text resizing.
6. Responsive behavior: useful phone layouts, overflow safety, touch targets, light/dark behavior.

8 = polished, usable, no substantial unresolved problem in the category. 9 = excellent with minor refinements possible. 10 = exceptional. Be critical; a target is not a reason to inflate scores. List concrete blocking findings with route/selector and a suggested fix. Explicitly distinguish observed behavior from source-code inference or untested states.

Completion requires both Codex Sol and Claude Opus to score **every category >=8 in two consecutive rounds**, with regression checks passing. New findings reset the pass streak if any score falls below 8.

## Review mechanics

Each round is a fresh independent review of a frozen build. Reviewers do not edit application code or fixture data. They may write their own assigned Markdown review report. Use saved screenshots plus source inspection and/or your own browser tab for hands-on checks. Do not operate another reviewer's tab. The same scope and rubric are retained each round; changes and findings from prior rounds are supplied as context.

## Isolated fixture

App: http://t3code:18280 ; render: http://t3code:18281
Fixture login: admin / changeme (scratch data only).
Run Log: /artifacts/461183aa-ca77-41f8-a4c0-6bbbe189b332 ; append /edit for editor.
Screenshot auth (do not commit): /tmp/exhibit-zrjo-auth.json
Capture workflow: shot-scraper (repository convention).
