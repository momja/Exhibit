---
id: Exh-i902
status: open
deps: [Exh-zbwk]
links: []
created: 2026-11-04T05:37:11Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, export, experimental]
---
# Exports: explain missing inference in .html exports and run it locally from the ZIP

An artifact that calls window.exhibit.ai leaves the service without it. The exported file should say so rather than break silently, and the portable ZIP should be able to run inference locally with a key the person enters there, the same pattern connectors use.

## Design

- .html export: include a minimal window.exhibit.ai stand-in whose calls reject with 'unavailable' and which shows nothing in the artifact's DOM. The export dialog names the fact that this tool uses AI and that it won't work offline.
- ZIP export: the local relay gains an inference route backed by a locally entered provider key (never the instance's, never the viewer's stored one), using the same request shape so the artifact runs unchanged.
- No credential of any kind is written into either export.

## Acceptance Criteria

- An exported .html of the example artifact loads and its calls reject with 'unavailable'.
- The ZIP runner completes a decide() call against a locally configured mock provider.
- A test asserts neither export contains a key or the instance's inference configuration.

