---
id: Exh-16ds
status: in_progress
deps: []
links: []
created: 2026-10-20T05:26:17Z
type: feature
priority: 2
assignee: Max Omdal
tags: [ui, frontend, components]
---
# Add Stimulus foundation for reusable gallery components

Adopt self-hosted Stimulus as the gallery behavior layer, with explicit native module registration and documented component conventions. Migrate existing gallery action menus as the first reusable controller; leave modal migration to the upcoming story.

## Acceptance Criteria

Pin and embed the Stimulus ES module and license; preserve the no-bundler gallery build; initialize controllers once; gallery menus retain their actions, keyboard behavior, focus, and htmx lifecycle; add real-browser regression coverage for multiple instances, swaps, and cleanup; document how a new modal controller is registered and composed with Go partials and htmx.

