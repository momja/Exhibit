---
id: Exh-d0np
status: in_progress
deps: [Exh-3917]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate external link permission dialog to the shared modal

link-modal in internal/api/templates/detail.tmpl and the external-link bridge in web/gallery/detail.js own overlay dismissal, a pending-allow guard and iframe focus restoration. Adopt shared modal mechanics.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve destination hostname/copy, validated source and URL scheme, owner-only approval, persisted grant, and noopener external navigation. Block/Escape/backdrop cancel without persisting denial. All dismissal paths honor pending approval; a stale response cannot open another destination. Return focus to the live artifact frame as appropriate, and preserve popup/user-gesture behavior.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

