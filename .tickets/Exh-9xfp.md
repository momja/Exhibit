---
id: Exh-9xfp
status: in_progress
deps: [Exh-p3ah]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate sharing dialog to the shared modal

sharePanel in internal/api/templates/partials.tmpl and web/gallery/share.js implement a custom overlay, focus restoration, dismissal, and busy guards. Compose sharing with the shared native-dialog controller and shared modal styles.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve immediate-save people/link/state-mode controls, copy/embed controls, request locking through htmx refresh, recipient input and IME behavior, disclosure/scroll state, and error/retry handling. Escape, backdrop and Done share the saving guard; a pending request cannot be dismissed. Preserve the usable desktop/mobile and short-window layouts and focus the visible Share trigger on dismissal.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

