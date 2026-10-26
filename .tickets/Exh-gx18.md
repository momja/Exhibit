---
id: Exh-gx18
status: in_progress
deps: [Exh-2c2e]
links: []
created: 2026-10-26T01:02:13Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-qeuv
tags: [gallery, frontend]
---
# Gallery filters: tags and collections in the rail and a phone filter sheet, active filters as chips

ListOptions already supports Tags and Collections but no gallery exposes them. Add the filter axis that B2 keeps separate from spaces.

- URL params ?tag=<name> and ?collection=<id> (repeatable, AND semantics matching ListArtifacts), carried through search, paging (NextURL/RetryURL) and HX-Replace-Url. Applies to My library and Shared by me. Shared with me uses the owner filter from the previous story instead (tags are the owner's), surfaced through the same UI.
- Desktop: 'Collections' and 'Tags' sections in the left rail under the spaces (tag color dots per the mockup), toggling a filter is a link/htmx request that replaces the grid from offset zero, sharing the existing hx-sync scope with search.
- Phone: a Filter button beside search opens a bottom sheet (Stimulus controller registered in web/gallery/controllers.js; reuse modal/focus conventions from docs/ui-components.md) listing the same options; Escape/backdrop close; focus returns to the trigger.
- Active filters render as removable chips above the grid (server-rendered in the grid partial so htmx keeps them in sync). 'Clear all' when more than one.
- The rail filter sections must render even when the spaces nav is hidden (a solo user still has tags) — coordinate with the spaces partial so the rail exists when either has content; if the library has no tags or collections, no rail at all (unchanged page).
- Tests: Go tests for param parsing/validation (unknown tag -> ignored or 400, pick one and document), paging preserves filters; Node or browser test for the sheet controller; Chromium test at phone width. Update screenshots and docs/architecture.md §3.5 gallery section for spaces + filters. Closing this story also closes av-0k5q and the epic.

## Acceptance Criteria

- Tag/collection filtering works on desktop rail and phone sheet, with chips.
- Filters survive search and paging.
- No rail or sheet for a library with no tags, collections or shares.
- Docs and screenshots updated; av-0k5q and the epic closed on the branch.

