---
id: Exh-vfks
status: in_progress
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

