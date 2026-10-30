---
id: Exh-61ix
status: closed
deps: []
links: []
created: 2026-10-30T04:40:47Z
type: feature
priority: 2
assignee: Max Omdal
tags: [experiment, agent, render]
---
# Experimental component kit: pinned, CSS-only, inlined at render

Experiment toward cross-artifact UI cohesion. A built-in, versioned component kit (v1 is CSS only: design tokens plus component classes over native HTML). An artifact opts in and pins a version with <meta name="exhibit-kit" content="v1">; render and export replace the marker with the exact kit version inlined, so the stored body stays authorable and the exported file stays self-contained. Kits are immutable once released: a change is a new version, never an edit, so an old artifact never restyles silently. The agent gets list_kit / get_kit_component tools and prompt guidance to build with the kit.

## Design

No schema change. internal/kit embeds kit/<version>/{tokens.css,components.css,manifest.json}. kit.Inline(body) rewrites markers. Applied in render (artifact, share, version view, widget) and export. GET /api/kit and /api/kit/{version}/parts/{name} read-only, reachable by agent credentials (global, non-owner data). Multi-part source and per-library kits are later steps.

## Acceptance Criteria

Artifact with the marker renders and exports with v1 CSS inlined; unknown version is left as-is and harmless; agent can list and read the kit; prompt tells the agent to use it; docs describe the format and the immutability rule.


## Notes

**2026-10-30T08:27:09Z**

Built on feature/Exh-61ix/component-kit. Marker is <meta name="exhibit-kit" content="v1"> (nothing fetches a meta, so the footprint scanner ignores it). Kit rules live in @layer exhibit-kit so artifact CSS always wins. Released versions are hash-pinned by TestReleasedKitsAreImmutable. Tools: list_kit, get_kit_component. Docs: docs/component-kit.md.
