---
id: Exh-6fz9
status: closed
deps: [Exh-k8p6]
links: [Exh-k8p6, Exh-vfks]
created: 2026-10-19T04:42:37Z
type: feature
priority: 2
assignee: Max Omdal
---
# Export stored artifact assets as a ZIP from the edit page

Add Export assets to the Stored assets panel alongside localStorage JSON export. Download all current stored assets in one ZIP, with original bytes and a manifest mapping archive paths to source URLs and content types. Use existing owner-scoped authentication. Fail cleanly if any payload is unreadable.

## Acceptance Criteria

Owner can export a valid ZIP containing every current asset and a JSON manifest. Empty asset sets produce a manifest-only ZIP. Other owners and recipients cannot export assets. Missing payloads report an error without a partial archive. Verify browser download and mobile layout.


## Notes

**2026-10-19T04:52:36Z**

Added owner-only GET /api/artifacts/:id/assets/export and Export assets ZIP in the edit page. Archive contains raw current assets and manifest.json, preserves binary bytes, separates duplicate filenames, and fails before download for unreadable blobs. Shared download helper preserves localStorage export behavior. make assets, go build ./cmd/server, twelve export browser-script tests, targeted access/route tests, and final go test ./... all pass. Browser ZIP bytes and manifest matched saved assets; desktop/mobile screenshots recorded. Custom gallery widgets still have no dedicated download action.
