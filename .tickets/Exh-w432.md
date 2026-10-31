---
id: Exh-w432
status: in_progress
deps: [Exh-d0np]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate camera and microphone dialog to the shared modal

media-modal in internal/api/templates/detail.tmpl and its handlers in web/gallery/detail.js use a custom overlay. This is one implementation for both camera and microphone; migrate it as one ticket.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve the exact requested device set and corresponding approval flags, owner/recipient and stale-request gates, unchanged security/Permissions-Policy semantics, and the safe Block initial focus. Allow and open still goes through the shared direct-open flow; cancellation must settle the intended request and return focus. Late saves must not reopen a cancelled request, double-open tabs, or grant an unrequested device. Preserve nested-dialog behavior and popup blocking recovery.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

