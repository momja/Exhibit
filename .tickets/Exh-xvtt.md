---
id: Exh-xvtt
status: in_progress
deps: []
links: []
created: 2026-10-20T06:24:35Z
type: epic
priority: 1
assignee: Max Omdal
---
# Portable artifact bundles and private account import

Replace the default single-document download with an extensible versioned ZIP containing the complete current artifact: pages, widget, owner state, assets and portable metadata. Keep an explicit HTML-only choice. Add a private admin-token-only import into a target account, reusing PR #184 archive validation and staging patterns. Stack implementation on PR #184; delegate components and run review-agent before opening stacked PRs.

## Acceptance Criteria

Default export is a documented versioned ZIP with a pages array; HTML remains available. Detail and gallery downloads ask in an accessible modal. Complete bundles round-trip into a new artifact in an existing account atomically with fresh identities and bounded validation. Import has no public UI and requires an explicit admin token. Tests, screenshots, independent reviews, and stacked PRs are complete.

