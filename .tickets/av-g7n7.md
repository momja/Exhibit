---
id: av-g7n7
status: closed
deps: []
links: []
created: 2026-07-22T04:11:11Z
type: feature
priority: 2
assignee: Max Omdal
---
# Viewer mobile: compact header + actions bottom-sheet

The artifact viewer (detail.tmpl) header + .toolbar is a single flex row that wraps on narrow screens into a multi-line wall, leaving the artifact iframe almost no vertical space. Redesign the viewer for mobile (<=640px) so the artifact dominates: collapse to a single compact header row (back + truncated title + kebab), demote the date, and move every secondary action (Open in new tab, Edit, Modify with agent, capability/allowlist rows, Downloads, Clipboard, Update from source) into a bottom-sheet actions menu triggered by the kebab.

## Design

Mostly a @media (max-width:640px) block in web/gallery/detail.css plus a small toggle in detail.js. Reuse the 640px breakpoint (precedent: web/gallery/edit.css:55). Restyle the existing .toolbar in place into a fixed bottom sheet (transform translateY toggled by an .open class + a scrim div); hide .toolbar-sep on mobile; add a kebab button in the header shown only under the media query. The one wrinkle: the capability cluster (partials.tmpl capabilityCluster) renders an inline chip with a hover popover positioned top:100%/right:0 that flies off-screen in a tap sheet — render its Network/Downloads/Clipboard rows inline/static within the sheet on mobile. Also switch height:100vh -> 100dvh in the mobile query so the sheet/actions aren't hidden under the mobile URL bar. Desktop layout must be unchanged.

## Acceptance Criteria

At <=640px: header is a single non-wrapping row (back, ellipsized title, kebab); the artifact iframe fills the remaining viewport; tapping the kebab opens a bottom sheet listing all former toolbar actions plus expanded capability rows; scrim dismisses it. Desktop (>640px) renders identically to today. detail.js stays node --check clean; run build (web/gallery build.mjs) so assets embed; verify existing Go tests that assert on detail CSS/markup still pass. Manual mobile-viewport verification screenshot attached to the PR.

