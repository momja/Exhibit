---
id: Exh-9j7e
status: closed
deps: [Exh-johq]
links: []
created: 2026-10-20T02:25:12Z
type: feature
priority: 2
assignee: Max Omdal
---
# Duplicate artifacts from the gallery action menu

Add a Duplicate action and modal with editable title, copy share settings, and copy data options. Clone through the API, stay on the gallery, and update its server-rendered cards with htmx without a full page reload.

## Acceptance Criteria

Duplicates preserve artifact source and supporting content; title and optional share/data copying work; owner boundaries enforced; gallery refreshes using htmx; modal handles failures.


## Notes

**2026-10-20T02:41:34Z**

Implemented gallery Duplicate modal, owner-scoped atomic API/store clone, optional sharing and owner data, independent remapped assets, quota accounting and idempotent retries. Documented behavior in docs/artifact-duplication.md. Focused API/store checks and registered client suites passed; parent will complete integration/full-suite/browser verification.

**2026-10-20T02:51:11Z**

Integrated on feature/Exh-johq/gallery-actions-integration. Browser verified unchecked options produce empty private copies; checked options preserve owner data/share mode with fresh public IDs; filtered galleries reset and show the new artifact through htmx; performance.timeOrigin and a JS sentinel prove no document reload. Simulated a successful commit followed by lost acknowledgement and confirmed retry creates exactly one copy. Escape, focus restoration, light/dark styling and phone-width layout checked; screenshots updated. Added duplicate route to request-body-size boundary inventory; targeted boundary and generated-icon checks pass after rebuilding all assets.

**2026-10-20T02:53:44Z**

Final validation passed: make assets; go build ./cmd/server; go test ./... (API 312.264s, store 218.002s); browser interaction checks and screenshots. Both feature and main worktrees are clean; main remains on main.

**2026-10-20T04:44:54Z**

Fixed both review findings: duplicating a refetched artifact now follows owner-scoped stored URLs into historical assets, recursively includes textual dependencies, rewrites fresh identities, and preserves the current runtime manifest without copying unused history. Dialog padding clicks preserve edits while actual backdrop clicks dismiss. Regression coverage passed for historical dependency cycles, quota boundaries, source deletion, owner/artifact boundaries, all gallery script suites, and render asset/version tests. Built the server and verified real browser padding/backdrop clicks and focus restoration.
