---
id: av-b17a
status: open
deps: []
links: [av-ghvs]
created: 2026-08-12T02:51:46Z
type: bug
priority: 1
assignee: Max Omdal
tags: [ingest, snapshot, security]
---
# refetch skips the snapshot vendorer and seeds the allowlist from a scan

Two defects in `refetchArtifact` (internal/api/artifacts.go), both divergences from the ingest path beside it.

1. It never runs the snapshot vendorer — it overwrites the blob with the raw fetched bytes and re-scans. So 'Update from source' on a snapshotted artifact silently de-vendors it: a self-contained file becomes one full of live external references again, and after av-ghvs it also drops the inlined runtime assets, breaking a tool that worked a moment earlier.

2. It writes `network_allowlist: scanner.Scan(...)` straight into the store, seeding the allowlist directly from a scan. That contradicts the spec §6.2 invariant — approval is always explicit, the scan never seeds — which the comments on the create and PATCH paths in the same file assert and uphold. A refetch can therefore silently grant origins the user never approved.

(2) is the security-relevant half and is worth fixing on its own.

## Acceptance Criteria

- refetch runs the same snapshot pipeline as ingest, so a vendored artifact stays vendored across an update from source.
- refetch never writes the allowlist; new origins surface as a footprint for explicit approval, matching create and PATCH. The refetch response returns the same NetworkFootprint contract the create/PATCH paths return (rather than a bare artifact), so newly observed origins actually reach the approval flow's UI.
- A test asserts a refetch of a snapshotted artifact leaves the allowlist untouched and re-vendors the body.

