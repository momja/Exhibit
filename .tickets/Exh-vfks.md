---
id: Exh-vfks
status: closed
deps: []
links: [Exh-6fz9, Exh-k8p6]
created: 2026-10-20T03:01:26Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, storage]
---
# Import stored state JSON and stored asset ZIP exports

Add import controls symmetrical to the Stored state JSON and Stored assets ZIP exports. Imports replace the corresponding existing data. Confirm erasure only when that data exists, preserve exported strings and asset bytes, validate before replacement, and leave existing data intact on failure.

## Acceptance Criteria

Both panels accept their exported format. Existing corresponding data triggers explicit erasure confirmation; empty data imports without confirmation. Cancel and invalid files preserve existing data. Replacement is atomic, owner-scoped, and refreshes the panel. Cover empty imports, binary assets, malformed files, access control, and responsive layout.


## Notes

**2026-10-20T03:21:26Z**

Implemented Import JSON and Import assets ZIP with validation before replacement, transaction-level confirmation guards, owner-only access, quota-aware asset staging and cleanup, and partial panel refresh. Import/export regression tests, server build, and desktop/mobile browser checks pass; full repository test run is in progress. Screenshots are in docs/screenshots/Exh-vfks/.

**2026-10-20T03:24:29Z**

Final verification complete: make assets, the final server build, import/export regression checks, and go test ./... all pass. Browser checks verified empty imports without prompts, populated-data warnings, cancellation, accepted replacements, and asset delete/import swaps at desktop and 390px mobile widths. PR: https://github.com/momja/Exhibit/pull/184.

**2026-10-20T05:11:17Z**

User clarification: asset imports are always complete replacements of the current asset set. Correct quota accounting to evaluate the final swapped set, accept valid whole exported archives with repeated source URLs, and verify omitted assets are retired atomically. No merge or partial-import mode.

**2026-10-20T05:19:59Z**

Corrected complete asset replacement accounting: quota now measures the final swapped set and retains charges for body, shared, and historical references. Valid duplicate-source exports round-trip. Regression tests prove omitted assets disappear, unused payloads are reclaimed, empty manifests clear the set, and failed inserts restore the entire original set. Also fixed the reviewed state refresh race with sequencing of GET responses. Validation: go test ./... passed, 14 import/state JS tests passed, final server build passed, and live browser verified cancellation, A,B to B,C swaps, swapping back, and delayed pre-import responses. Live environment runs the updated binary on port 18110.
