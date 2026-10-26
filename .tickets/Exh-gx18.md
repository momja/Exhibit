---
id: Exh-gx18
status: closed
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


## Notes

**2026-10-26T02:38:26Z**

Decisions: gallery tags and collection IDs are repeatable AND filters; unknown, empty, or foreign choices return 400 and duplicates are removed. Existing API name-filter OR behavior is preserved via opt-in MatchAllFilters and CollectionIDs store options. Incoming shares expose owner filters through the same UI. Successful first-page htmx responses re-render chips, hidden search parameters and both option lists; request settings explicitly inherit in htmx 4. The phone sheet uses a registered Stimulus controller wrapping native dialog focus containment/dismissal and returning focus to Filter. Migration 045 adds reverse membership indexes after EXPLAIN confirmed scans; plan tests cover plain/search and shared-only variants. No-option full-page HTML matches base exactly for empty and five-card libraries.

**2026-10-26T02:54:36Z**

Verification: go build ./... and go vet ./... pass. The full go test ./... suite passes with the repository-pinned Pi 1.3.0, Node page-script suite, and CHROMIUM_TEST_BINARY set to Chromium 1248 (1,103 API cases; API package 369.957s). Phone and desktop filter browser cases also passed three consecutive targeted runs. Browser waits explicitly return Boolean values and wait for asynchronous dialog focus restoration. make lint matches all 28 base diagnostics exactly (15 errcheck, 2 goimports, 1 ineffassign, 8 staticcheck, 2 unused), with no new findings. Shot-scraper captures refreshed for 1280x900 desktop and 390x844 phone sheet; the screenshot build-tag test reproduces them.
