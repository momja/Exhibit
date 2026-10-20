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
