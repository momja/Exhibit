---
id: Exh-j6wa
status: in_progress
deps: []
links: [Exh-xw54]
created: 2026-10-31T21:36:17Z
type: bug
priority: 2
assignee: Codex subagent
tags: [ui, auth, gallery, css]
---
# Reserve space for the Clerk profile button while it loads

The hosted Clerk profile button starts from an empty mount container. Because Clerk is intentionally deferred until after page load, inserting the button later changes the header layout. Render a placeholder immediately and reserve the final button footprint so adjacent controls stay in place while Clerk loads.

## Design

Start with the gallery account mount in internal/api/templates/gallery.tmpl, web/gallery/clerk-controller.js, and shared header/component CSS. Match the real button's dimensions and the existing light/dark design tokens. The server renders the placeholder; use CSS and the existing Stimulus lifecycle to reveal the loaded button without building UI markup in JS. Keep loading/error handling accessible and avoid a fake focusable account action. Preserve Clerk's deferred startup and session renewal. Non-Clerk mode must not gain an empty slot. Keep compatible with the persistent-header navigation ticket and ongoing .tickets/Exh-arf2.md / .tickets/Exh-r6a0.md work.

## Acceptance Criteria

- The first rendered HTML reserves the Clerk button's final dimensions and shows an appropriate loading placeholder.
- Loading the real button does not move or resize adjacent header controls on desktop or mobile, including throttled/slow Clerk loading.
- Loading, success, and failure states remain accessible; errors remain visible and usable without making the header jump. The placeholder is not an extra keyboard stop and does not obscure the loaded control.
- Existing deferred loading, account behavior, and non-Clerk layouts remain correct; no third-party icons or new blocking startup work are introduced.
- Verify before/after header geometry in a real browser with delayed Clerk startup, and update desktop/mobile screenshots.
