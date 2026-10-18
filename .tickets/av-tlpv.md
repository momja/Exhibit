---
id: av-tlpv
status: closed
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

A narrow, stacked sharing editor connects people, public link permissions, and saved-data behavior. Static glyph cards explain separate data per person and shared data for everyone. Public links explicitly expose the owner's existing saved data read-only, in either mode. A collapsed switching disclosure names which stored data each person will see.

Changes save immediately through the existing API routes, followed by htmx updates of the server-rendered sharing fragment. Enter submits people using the native form on desktop and mobile. Link access displays the generated URL as soon as it is enabled. Done dismisses the modal, with save feedback inline beside it. The iframe embed controls start collapsed.

The guest list has its own viewport-proportional scroll area. Short windows scroll the overlay outside the dialog rather than the modal body.

## Acceptance Criteria

- Shared-data cards explicitly state that named recipients can view and update the same data.
- Public-link exposure and read-only behavior are visible before activation.
- People, public link, revocations, and data-mode changes persist immediately and refresh through htmx.
- Enter adds people on desktop and mobile without an Add button.
- Enabling public access immediately displays a working link.
- Done dismisses the modal; save confirmation appears on the same line.
- Failed mutations report their outcome and allow retry.
- Large guest lists scroll independently; short windows scroll outside the dialog.
- The iframe embed controls start collapsed.
- Static glyphs remain on both mode cards; switching details start collapsed.
- The panel works on desktop and narrow phone viewports.
