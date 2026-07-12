---
id: Exh-un03
status: open
deps: []
links: []
created: 2026-07-11T05:02:09Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-avau
---
# Static build manifest: schema + config loading

Define the manifest file format consumed by the new static-build subcommand: output directory, base URL/path the static site will be served from, and site title/tagline. Per epic Design, this is build configuration only — not a per-artifact allow-list; scope is whole-library export (every artifact in the instance is built). Add a loader that parses and validates the manifest (clear errors on missing/malformed fields) as a standalone package other build-mode code can depend on.

## Acceptance Criteria

A documented manifest format (e.g. YAML) exists with output dir, base URL/path, and site title/tagline fields. A Go package loads and validates a manifest file, returning clear errors for missing required fields or malformed values. Unit tests cover valid and invalid manifests.

