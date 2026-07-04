---
id: exhibit-2hl
status: open
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


