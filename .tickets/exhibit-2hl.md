---
id: exhibit-2hl
status: closed
deps: [exhibit-x87]
links: []
created: 2026-07-01T05:24:02Z
type: feature
priority: 3
parent: exhibit-3yo
---
# Surface newly-detected origins for approval after a body edit

After the exhibit-i0k security fix, editing an artifact body (PATCH with a new body) no longer auto-adds newly scanned origins to the allowlist — approval must be explicit. But the edit page (renderEditPage in internal/api/gallery.go) gives no in-UI approval path for origins the edited body introduces; the user only finds out via the manual '+ Add origin' control on the detail page (or the future runtime prompt). Surface the re-scanned footprint on save and offer the same approval UI the ingest flow now uses.

## Design

updateArtifact could return the re-scanned footprint in its response; the edit-page JS then reuses the ingest showApproval() flow to let the user approve newly-detected origins into network_allowlist via PATCH.



## Notes

**2026-10-19T19:50:26Z**

Audited on 2026-10-19 against origin/main 550499c. This requirement is already implemented; the open status was stale.

- [Exh-4021](Exh-4021.md) shipped the body-edit re-scan and approval flow in commit cef31e9, merged as 13fa43d on 2026-07-10.
- internal/api/artifacts.go updateArtifact compares the old/new body's network footprint and returns network_footprint plus footprint_changed without granting scanned origins.
- web/gallery/edit.js save() shows the returned footprint after a successful body save; approveOrigins() writes explicitly selected origins through PATCH while preserving existing approvals. Previously blocked origins are labelled and start unchecked; declining adds no approval.
- internal/api/gallery.go renderEditPage also surfaces referenced, undecided origins in the security panel. The dependency [exhibit-x87](exhibit-x87.md) is closed and its relational decision model is in use.

Validation against the audited revision: targeted API/scanner/store Go tests passed (167 cases including subtests); the 13 edit-page JavaScript tests passed; an additional script exercised save-to-approval, explicit approval, preserved existing grants, unchecked blocked origins, declining approval, unchanged footprints, and failed saves. The CodeMirror bundle built successfully.

No additional implementation is required for this ticket.
