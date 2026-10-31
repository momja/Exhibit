---
id: Exh-7hh4
status: in_progress
deps: [Exh-w432]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate location permission dialog to the shared modal

geo-modal in internal/api/templates/detail.tmpl and its handlers in web/gallery/detail.js implement a custom location approval overlay. Adopt the shared native modal controller.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve owner-only geolocation approval, validated frame/request identity, denial replies, permission persistence, and the top-level direct-open behavior. Keep location/security copy and Permissions-Policy unchanged. Escape, backdrop and Block settle the pending request; stale callbacks, cancellation, failed approval and popup blocking do not grant or open unexpectedly. Initial/return focus and nested direct-open interaction work on desktop and phone.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

