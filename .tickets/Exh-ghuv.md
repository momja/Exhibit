---
id: Exh-ghuv
status: in_progress
deps: []
links: []
created: 2026-10-18T16:55:44Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, detail]
---
# Collapse the artifact detail page's two-row header into compact chrome

On desktop the artifact detail page (/artifacts/:id) stacks two full-width bars above the sandboxed frame: the app header (back link, title, created date) and a toolbar row (Open in new tab, Edit, Modify with agent, Download file, sandbox posture cluster, Manage in allowlist settings, Share, Update from source). Together they take ~95px of vertical space on a page whose primary interface is the artifact itself. Mobile already solved this with a one-row header plus a bottom-sheet toolbar (av-g7n7); desktop still has the two rows.

## Design

Propose five interface options on the paper canvas, pick one, implement it in detail.tmpl/detail.css/detail.js. The toolbar DOM must stay the single definition of the action list (the mobile sheet restyles the same markup). Owner-only controls stay gated on .ReadOnly; the capability cluster stays an htmx target (av-mf1x).

## Acceptance Criteria

Desktop detail page chrome is a single bar no taller than --header-height (40px). Every existing action remains reachable for an owner, and a recipient sees the same reduced set as today. Mobile bottom-sheet behaviour unchanged. Page-script tests and Go template tests pass. Screenshots in docs updated.


## Notes

**2026-10-18T17:31:27Z**

Decision (2026-10-18): option 4, the floating pill, extended to picture-in-picture behaviour. Artifact full-bleed. The pill has a grip as its only drag area, snaps to the four corners using a projected throw, and can be flicked past the left/right edge to stash as a slim edge tab (hover reveals it; click or drag restores it). Position is remembered per browser. Rationale: content first, so the artifact is not a second-class object embedded in Exhibit, and the pill can later become an agent tool palette. This supersedes the 40px single-bar acceptance criterion. Open question: where banners (capability warning, state changed) live, proposed as toasts anchored to the pill plus a dot on the stashed tab.

**2026-10-18T22:21:37Z**

Prototype review decisions (demo: paper canvas frame 07-pip-demo):
- Pill order: back button, divider, grip, title, divider, action buttons, ⋮ menu. The grip is mid-pill, so its hover shape is an 8px rounded rect; the back button at the end is a full circle, concentric with the outline.
- No placeholder slots or edge highlights while dragging. Release springs to the nearest corner, using the throw's velocity to pick it.
- Stashing is positional only: it happens when the pointer is released within 20px of the left or right window edge. Velocity never stashes. While in that zone the pill fades to 55% opacity as the only cue.
- The drag lift scales about the grab point (transform-origin = pointer offset, position via the translate property), so the pill stays under the pointer.
- The ⋮ menu and the info card hang off their own trigger and open toward the screen centre.
- No toasts. Placement changes are announced to screen readers only, through an aria-live region.
- Sandbox posture is option D: the title is a button with a posture glyph (grey shield when contained, accent globe + origin count with grants) and a caret. It opens an info card with the created date, the Sandbox posture receipt (same rows as capabilityPopover), Sharing (owner only) and Manage security settings (owner only). The separate posture button is gone, and the created date and Manage allowlist link leave the ⋮ menu. The card stays one server-rendered partial that htmx re-renders on exhibit:capabilities-changed.
- Banners (capability warning, state changed) still need a home; toasts are ruled out.
