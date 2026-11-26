---
id: Exh-wmw7
status: open
deps: []
links: []
created: 2026-11-26T20:51:13Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [a11y, editor, palette]
---
# Light-mode code selection color drops syntax text below 4.5:1 contrast

The Playwright journey e2e/specs/ui-appearance.spec.ts › 'source syntax stays differentiated and readable in both color schemes' fails on origin/main (0fa708a5). With source selected in CodeMirror in light mode, syntax text over the selection background rgb(220, 213, 193) measures about 4.0-4.3:1 (for example '<!doctype html>' 4.24, 'section' 4.03), below the 4.5:1 the journey requires; the first failure is reported at 1280px light. The color is --color-code-selection: #dcd5c1 in web/gallery/tokens.css, set by #308 (beca1645, brand refresh). Found while fixing Exh-1zg6 (PR #314), where the owner chose to track it separately. It fails identically with that PR's change stashed. Fixing it is a palette decision: pick a selection tone that keeps every syntax color at or above 4.5:1 in light mode and still reads as a selection on the paper ground.

## Acceptance Criteria

- The source syntax journey passes at 1280, 390 and 320px in light and dark.
- The chosen selection color is approved as part of the paper/dusk palette.
- No syntax color loses contrast against the unselected editor background.

