---
id: av-hwx2
status: open
deps: [av-41se, av-p0a1]
links: [av-isb3, av-p0a1, av-41se]
created: 2026-07-18T15:15:41Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-jafp
tags: [ui, viewer]
---
# Viewer: read-only posture + Manage link

The artifact viewer/detail page edits the allowlist inline today (detail.tmpl #al-display + web/gallery/detail.js addOrigin/saveAllowlist). With management moved to the Edit page (av-p0a1), the viewer becomes read-only: it shows the capability cluster (the av-isb3 badge component) plus a "Manage in allowlist settings ->" link to Edit. Confirmed direction: the viewer before/after was reviewed and approved ("the viewer changes are great, we will roll with that").

## Mockups

Paper file 01KWQV7Y6G82HDZJ5CKPJTSJ7Y:
- "Artifact Edit — allowlist moved here (option)" — see its "WHAT CHANGES IN THE VIEWER" before/after strip (busy inline editor -> read-only cluster + Manage link).
- Cluster + popover behaviour: "av-isb3 — B · Capability row + tooltip".

## Design

- Remove the inline allowlist editor and add-origin control from detail.tmpl and detail.js.
- Show the read-only capability cluster (reuse the av-isb3 / av-41se component) in the viewer toolbar; it opens the av-41se popover on hover/focus/tap.
- Add "Manage in allowlist settings ->" linking to /artifacts/:id/edit (the av-p0a1 security section).
- The viewer no longer PATCHes network_allowlist — it only reads.

## Acceptance Criteria

- The viewer/detail page has no inline allowlist editor or add-origin control.
- The viewer shows the read-only cluster and a working Manage link to the Edit page.
- The av-41se popover opens from the viewer cluster.
- No allowlist mutation originates from the viewer; management happens only on Edit.

Depends on the capability cluster (av-isb3), the popover (av-41se), and the Edit-page destination (av-p0a1).

## Complexity

M
