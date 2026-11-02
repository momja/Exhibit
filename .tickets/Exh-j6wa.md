---
id: Exh-j6wa
status: closed
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

## Notes

**2026-11-02T01:20:21Z**

Implemented in bug/Exh-j6wa/clerk-button-placeholder on the isolated worktree based on a1dddeb0. The shared accountControl now server-renders a non-interactive circular placeholder in the existing 32px header-control token footprint. Clerk's trigger uses the same token. Loading continues through deferred startup, permission reload, and asynchronous button rendering; only an enabled supported Clerk trigger reveals the inert mount. Live loading/ready/error status, anchored Try again/Settings recovery, bounded permission/render waits, and disconnect cleanup preserve neighboring geometry. The deferred client/session-renewal path and current menu entries remain unchanged. Non-Clerk markup keeps its ordinary link without a reserved slot.

Validation completed:
- make assets: passed; final gallery assets rebuilt with node web/gallery/build.mjs.
- From web/gallery: node --experimental-vm-modules --test *.test.mjs: 458 passed, 0 failed/skipped.
- go test ./internal/api -run 'Clerk|AccountControl|AppHeader|EveryAppPageUsesSharedHeader|GallerySpaceNavigation|PagesReferenceOnlyVersionedAssets|GalleryPageScriptSuite' -count=1 -v: passed, 24 top-level tests, 103.096s; no skips. Real Chromium compared header/neighbor positions and sizes at 1280/768/767/390/320px in light/dark on gallery and Settings. Verified idle/reload/asynchronous/disabled-button stages, load/reload/session errors, actual 15-second reload/render timeouts, keyboard recovery, stale reload/render callbacks, unmount/reconnect, current account/Profile/Settings/deletion behavior, persistent htmx space navigation/history, and non-Clerk headers.
- go test -tags screenshot ./internal/api -run '^TestCaptureClerkPlaceholder$' -count=1 -v: passed, 12.925s; shot-scraper regenerated six loading/ready/error desktop/phone screenshots in docs/screenshots/Exh-j6wa. Component docs explain the lifecycle, screenshot command, and SDK boundary. New browser suites are required by the CI skip guard.
- node --check e2e/check-browser-tests.mjs and git diff --check: passed.

Integration/limitations: browser timing, lifecycle, and screenshots use a local SDK double with the production loader/templates/styles/Stimulus/htmx. The ready icon is simulated; live hosted Clerk UI has not been validated. Readiness relies on Clerk's supported cl-userButtonTrigger class; verify it and the hosted menu when upgrading Clerk. Full-repository go test ./... was not rerun; the earlier restart-cancelled run remains incomplete and is not a pass. No PR, merge, or push requested/performed. Parent main remains clean at the ticket-only base commit.
