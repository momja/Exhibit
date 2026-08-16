---
id: av-d2xf
status: closed
deps: [av-r0dk]
links: []
created: 2026-08-16T16:31:31Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-mdc5
tags: [ui, gallery]
---
# Edit page: external-links capability toggle (revocable)

Edit-page half of the link bridge grant (epic av-mdc5): the first-use grant for external links must be visible and revocable, matching downloads and clipboard.

- internal/api/templates/edit.tmpl — the Capabilities column gains a third row: **External links** with the same Ask first / Always allow <select> the Downloads and Clipboard rows use; wire it to the same PATCH path those rows use (see the dl-select/clip-select handling in web/gallery/edit.js). The bootstrap gains let linksApproved = ... from the view.
- internal/api/gallery.go — the edit-page construction site of capabilityView gains LinksApproved, fed from the new links_approved column (the detail-site view ships with the backend ticket).
- internal/api/templates/partials.tmpl — capabilityPopover: add an "External links — Can open links in new tabs" row (Phosphor icon, e.g. ph-arrow-square-out), and fold LinksApproved into the $sandboxed "Fully contained" condition so the copy stays truthful.
- The detail modal's copy already points at the toolbar; no new navigation needed — the edit page's existing #security-panel anchor is the destination.

## Acceptance Criteria

- The edit page shows the External links select; toggling PATCHes links_approved through the single write path and reflects on the detail page after reload.
- The capability popover lists the links grant when active and never claims "Fully contained" while it is set.
- gallery_test.go assertions mirror the existing downloads/clipboard edit-page tests.

