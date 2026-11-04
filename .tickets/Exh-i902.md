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

An artifact that calls window.exhibit.ai leaves the service without it. The exported file should say so rather than break silently, and the portable ZIP should be able to run inference locally with keys the person enters there, the same pattern connectors use.

## Design

- .html export: include a minimal window.exhibit.ai stand-in whose available() is empty and whose methods reject with 'unavailable', showing nothing in the artifact's DOM. The export dialog says this tool uses AI and won't use it offline.
- ZIP export: the local relay gains the inference route with the same request shape, so the artifact runs unchanged. Each kind works only if the person configures a provider for it locally (never the instance's credential, never the viewer's stored one).
- No credential of any kind is written into either export.

## Acceptance Criteria

- An exported .html of the example artifact loads, available() is empty and calls reject with 'unavailable'.
- The ZIP runner completes complete() and decide() calls against locally configured mock providers, and reports unavailable for an unconfigured kind.
- A test asserts neither export contains a key or the instance's inference configuration.
