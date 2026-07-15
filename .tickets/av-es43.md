---
id: av-es43
status: in_progress
deps: []
links: []
created: 2026-07-14T23:17:39Z
type: bug
priority: 1
assignee: Max Omdal
---
# Search clear button shows when empty + height mismatch

The search clear ('x') button in the gallery search bar has two issues:

1. **Visibility**: The button is visible even when the search input is empty. The DOM shows `hidden=""` on the button, but the CSS `display:inline-flex` from `.btn` may override the hidden attribute, or the `syncClear()` JS isn't correctly hiding it on initial load.
2. **Height**: The clear button height (~23px per the screenshot measurement) doesn't match the search bar height (~35px). The input uses `padding:9px 12px` while the button inherits `.btn-sm` (`padding:5px 12px;font-size:13px`) plus `.search-clear` (`padding:5px 10px`). Buttons in the search row should match the input's visual height.

## Acceptance Criteria

1. The 'x' clear button is only visible when the search input has text (including on page load with a `?q=...` param).
2. The 'x' button is the same visual height as the search input.

