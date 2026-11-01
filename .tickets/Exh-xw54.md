---
id: Exh-xw54
status: closed
deps: []
links: [Exh-j6wa]
created: 2026-10-31T21:36:17Z
type: bug
priority: 2
assignee: Codex subagent
tags: [gallery, frontend, perf, auth]
---
# Switch library spaces without reloading the app shell

Switching My library, Shared with me, and Shared by me currently follows ordinary anchors and replaces the whole document. Clerk is deduplicated only within one document, so every tab change repeats deferred initialization and the user-data refresh before the profile button returns. Keep the app header and the mounted Clerk button alive while switching library spaces through server-rendered htmx fragments.

## Design

Start with internal/api/templates/galleryspaces.tmpl, gallery.tmpl, gallerygrid.tmpl, internal/api/galleryspaces.go, sharedgallery.go, and web/gallery/gallery.js. Define reusable Go template regions shared by full pages and fragments. Keep the account controller outside replaced regions; update the search form, filters, gallery, active-space state, counts, page title, and destination-specific permissions consistently. Use Stimulus lifecycle for reusable behavior and native ES modules; do not build fragment markup in JS. Ensure request races and history restoration cannot apply a previous space's results or permissions to the current space. Preserve normal href navigation as a fallback. The Clerk placeholder is a separate ticket. Related ongoing header/account work is tracked in .tickets/Exh-arf2.md and .tickets/Exh-r6a0.md; avoid broad header or account refactors.

## Acceptance Criteria

- Switching among all three library spaces preserves the document, header, and Clerk mount; no additional Clerk initialization, user reload, or button remount is caused by a space switch.
- Each space has the correct title, active navigation, counts, search/filter endpoints, results, empty state, and read-only versus owner controls. Search, filters, pagination, widgets, and duplication continue working after repeated switches.
- URLs remain directly loadable and bookmarkable; browser Back/Forward restores a coherent space and query/filter state. Ordinary modified clicks and navigation without JS retain their normal behavior.
- Fast switching and outstanding search/pagination requests cannot replace the destination with stale results. Authentication failure continues through the existing safe session-expiry path.
- Focused backend and real-browser checks cover navigation without document/account remounts, history, and post-switch interactions. Update relevant documentation and desktop/mobile screenshots.


## Notes

**2026-10-31T22:02:22Z**

Implemented htmx navigation for all three library spaces using shared gallerySpaceContent and search-region partials. The stable gallery shell/header/Clerk root survive space switches and history traversal. Registered gallery/gallery-space controllers coordinate stale requests and release search listeners, timers, widget observers, duplication dialogs and return-link behavior. api.js reads the active server-rendered read-only decision, so an incoming-space bootstrap does not prevent later owner duplication.

Validation: make assets; go test ./... (all pass); focused gallery/shared/action-menu/compression regression suite (pass); Node search/duplication/widget suites (24 pass); go vet ./internal/api (pass); git diff --check (pass). Real Chromium desktop/phone checks cover exact document/header/account/button identity with Clerk load=1, reload=1, mount=1, unmount=0, search/filter/pagination/history, owner duplication from a read-only initial load, widget disposal/remount, delayed responses, rapid switches, failures, modified clicks and native links with app scripts blocked. The real Clerk loader/controller uses a local SDK double; no live hosted account was needed. shot-scraper desktop/phone captures are in docs/screenshots/Exh-xw54 and documented in architecture/ui-components.

Integration: Exh-arf2 and Exh-r6a0 must keep the header/account control outside #gallery-content and #gallery-search-region; only the heading/search/filter trigger belongs in the search region. Clerk/account implementation and the separate Exh-j6wa placeholder ticket were left unchanged. No main edits, merge, push or PR.

**2026-11-01T17:35:45Z**

PR integration: rebased onto remote main at 7d84543, retaining the shared appHeader/account menu and shared Duplicate modal. The gallery heading/search/filter trigger now form the shared header's swappable region; the account control stays outside both swap targets. Duplication listeners follow space lifecycle while preserving the shared modal event contract. Desktop/mobile screenshots refreshed against this integrated layout.

**2026-11-01T17:38:28Z**

Post-rebase checks passed: make assets; go vet ./internal/api; screenshot capture for TestCaptureGallerySpaceNavigation; all six Playwright application journeys. Full Go test suite is running against the integrated branch.
