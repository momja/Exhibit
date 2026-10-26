---
id: Exh-2c2e
status: closed
deps: [Exh-u9b1]
links: []
created: 2026-10-26T01:02:13Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-qeuv
tags: [gallery, frontend, css]
---
# B2 spaces navigation: left rail on desktop, bottom tab bar on phones

The navigation chosen in av-0k5q (B2).

- One Go partial 'gallerySpaces' (components or gallerygrid template) listing My library / Shared with me / Shared by me with Phosphor icons (ph-books, ph-tray-arrow-down or ph-users, ph-share-network — pick and add to web/icons/icons.txt) and the active space marked aria-current=page. Plain links, no JS: each space is its own page from the previous story.
- Two layouts from the same markup in index.css: desktop/wide — a left rail beside the grid (~180px, labels + counts); narrow — a fixed bottom tab bar (icons + short labels, safe-area-inset-bottom padding, grid gets bottom padding so the last row is not hidden). Breakpoint by width; tab bar targets >=44px. Dark mode from tokens only (no literal colors).
- Render the nav only when SpaceCounts says a second space has content (shared-with > 0 or shared-by > 0). A library with neither renders exactly today's page — assert this in a test.
- Show counts next to Shared with me / Shared by me in the rail (not in the tab bar).
- Keyboard: links in order, visible focus. Screen reader: <nav aria-label="Library spaces">.
- Browser test (existing Chromium helper) at a phone viewport and a desktop viewport: correct layout shown, active state, navigation works. Update screenshots.

## Acceptance Criteria

- One partial, two CSS layouts; no layout JS.
- Hidden when only My library has content.
- Phone: bottom tab bar, content not obscured; desktop: left rail.
- Tests and screenshots updated.


## Notes

**2026-10-26T02:22:51Z**

Implementation decisions: one gallerySpaces partial with a 208px desktop rail, switching to fixed bottom tabs below 768px. The slightly wider rail keeps full labels beside two-digit counts. Phone labels: My library, With me, By me; full names remain available to assistive technology. Icons: ph-books, ph-tray-arrow-down, ph-share-network. Counts are per viewer and independent of search; both zero preserves the original page. Count lookup errors use the generic 500 response. The aside leaves room for future filters; no filters, layout JS, or migration.

**2026-10-26T02:24:54Z**

Verification: make assets, go build ./..., go vet ./..., and go test ./... -count=1 -timeout=20m all pass with CHROMIUM_TEST_BINARY set and the repository-pinned Pi 1.3.0 on PATH. The Go suite includes the Node page-script suite and Chromium browser tests; new cases cover 390x844 and 1280x900 layouts, navigation, focus, dark mode, safe-area spacing, and last-card clearance. make lint reports the same 28 findings as base 6ccab5540b463400441bec8cbe1340bb9498be92 (15 errcheck, 2 goimports, 1 ineffassign, 8 staticcheck, 2 unused), with no introduced findings. Complete normalized no-share HTML matches the base branch for empty and five-card libraries. Phone and desktop screenshots regenerated with shot-scraper; the screenshot build-tag test reproduces them.
