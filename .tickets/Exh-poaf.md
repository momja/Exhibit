---
id: Exh-poaf
status: closed
deps: []
links: []
created: 2026-10-20T06:24:35Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-xvtt
---
# Versioned artifact bundle format and complete ZIP export

Define the portable manifest and page list; export source, optional widget, owner state, asset bytes and metadata. Default /export to ZIP; retain ?format=html. Bound and validate reads and fail before download headers. Reuse the existing asset export representation. Document format and test complete exports.


## Notes

**2026-10-20T06:43:27Z**

Implemented owner-only default ZIP export with editable source, portable page/widget HTML, owner state, original assets and readable manifest. Retains reachable historical assets without shadowing current runtime source URLs. Preserves the shared schema and coordinates cfg-based upload/document limits plus Stored asset budgets with the private importer. Explicit format=html retains the legacy state-free download. Focused bundle and legacy export tests pass; package regression suites are running.
