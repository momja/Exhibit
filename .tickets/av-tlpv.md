---
id: av-tlpv
status: open
deps: []
links: [av-7k7b]
created: 2026-09-12T04:34:14Z
type: feature
priority: 2
assignee: Max Omdal
tags: [sharing, ui]
---
# Confirm before a share_state_mode change orphans a grantee's data

Switching artifacts.share_state_mode changes which artifact_state rows a viewer's ID resolves to (StatePrincipal in internal/store/store.go) with no data migration between the two: 'shared' resolves every viewer to the owner's rows, 'own' resolves each viewer to their own. Flipping shared -> own does not delete anything, but every grantee's next visit reads their own (empty) row instead of the owner's board they were writing to — it reads as their progress vanishing. The owner's share panel (web/gallery/share.js setStateMode) fires this on a plain fetch with no confirmation, unlike the adjacent 'Replace link' action in the same file, which already uses confirm() with explicit consequence copy ('Anyone still holding the old link loses access').

## Design

Mirror the existing 'Replace link' pattern in web/gallery/share.js: gate setStateMode behind a confirm() when switching away from 'shared' and share_grant_count > 0, wording the consequence in the same voice as newShareBadgeView's detail text (internal/api/gallery.go ~580) e.g. "N people will stop seeing the shared board — each reverts to their own data." No confirmation needed switching own -> shared (nothing is orphaned, though a grantee's own data becomes shadowed by the owner's — worth a lighter note, not a confirm). No server-side change: PATCH share_state_mode already validates and applies immediately: this is purely a client-side guardrail.

## Acceptance Criteria

- Switching share_state_mode from 'shared' to 'own' while share_grant_count > 0 shows a confirm() naming the grant count and the consequence before the PATCH fires.
- Cancelling the confirm leaves the mode and the <select> unchanged.
- Switching own -> shared, or switching modes with zero grants, fires no confirmation (matches today's behavior).
- web/gallery/share.test.mjs covers both the confirm-and-proceed and cancel paths.

