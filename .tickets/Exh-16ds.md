---
id: Exh-16ds
status: closed
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


## Notes

**2026-10-20T05:39:56Z**

Implemented pinned app-origin Stimulus ES module/license, explicit controller registration, a shared actionMenu partial with link/event helpers, and migration of gallery Edit/Duplicate menus. Added real Chromium coverage for different menus, escaped data, keyboard/focus, htmx swaps/appends, disconnect/reconnect, and duplication-dialog focus. Documented registration and the upcoming modal seam; captured desktop/mobile screenshots. Validation: make assets; focused integration and Node suites; go test ./...; manual authenticated duplication and htmx refresh.
