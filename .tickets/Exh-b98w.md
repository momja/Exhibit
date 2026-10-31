---
id: Exh-b98w
status: in_progress
deps: [Exh-gynk]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate admin entitlement editor to the shared modal

The entitlement-dialog in internal/api/templates/admin.tmpl uses showModal/close directly in web/gallery/settings.js, with its own styling and dismissal paths. Adopt the shared modal without changing entitlement semantics.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Opening any account row loads that account and returns focus to its own trigger. Preserve plan/reference and exact nonnegative whole-byte parsing: blank means instance default, malformed text must never silently clear a limit. Preserve authorization and server failures. Saving and dismissal have a coherent pending guard; validation failures retain the entered fields. Use a native ES module for substantially rewritten page code.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

