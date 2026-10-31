---
id: Exh-xw54
status: in_progress
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

