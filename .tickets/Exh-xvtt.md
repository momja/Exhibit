---
id: Exh-xvtt
status: closed
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

## Notes

**2026-10-20T09:03:56Z**

Implemented the versioned page-based bundle contract, complete ZIP exporter, shared native ZIP/HTML download dialog and private dedicated-admin-token importer atop PR #184. Components were implemented by isolated subagents. Full-diff and security reviews were addressed; final parser and focus-race rereviews both report no findings. Focused API/export tests, offline Chromium JSON/image execution, cross-instance/account restoration and five repeated download browser runs pass. Desktop/mobile screenshots and operator/schema documentation are included. Full-suite publication validation is recorded in the stacked PRs.
