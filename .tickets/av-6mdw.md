---
id: av-6mdw
status: in_progress
deps: []
links: []
created: 2026-09-29T04:51:21Z
type: feature
priority: 3
assignee: Max Omdal
tags: [ui, gallery]
---
# Press / to focus gallery search, with a key hint in the search bar

The gallery's search box is the page's main control, but the only way to reach it is the mouse or tabbing past the header links. Most sites with a library-style search (GitHub, YouTube, MDN) answer the `/` key by focusing search, and show a small `/` key cap inside the box so people learn the shortcut without being told.

Add both to the gallery page:

- Pressing `/` anywhere on the page focuses the search input and selects any existing query, so typing replaces it. The slash is not typed into the box.
- The shortcut stays out of the way when the keystroke is headed somewhere else: a slash typed into any input, textarea, select or contenteditable is left alone, and Ctrl/Cmd/Alt+/ are not intercepted.
- A key-cap hint (`/`) sits at the right edge of the search box while it is empty and unfocused. It hides once the box has focus or text (the clear button takes that spot), and on touch devices, where there is no physical keyboard to press it on.
- The input advertises the shortcut to assistive tech with `aria-keyshortcuts="/"`; the visual hint is `aria-hidden`.

## Acceptance Criteria

- `/` on the gallery page focuses #search-input and selects its text; the slash does not land in the box.
- `/` typed into another field, or with Ctrl/Cmd/Alt held, is not intercepted.
- The hint shows on an empty, unfocused search box and hides on focus, with a query, and under pointer:coarse.
- A node page-script test covers the shortcut; the Go gallery test asserts the hint and aria-keyshortcuts are rendered.

