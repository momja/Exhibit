---
id: Exh-uxyy
status: in_progress
deps: [Exh-9xfp]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate agent API key dialog to the shared modal

The BYOK-only key-modal in internal/api/templates/agent.tmpl is controlled by openKeyModal/closeKeyModal and inline handlers in web/gallery/agent.js. Replace its custom overlay mechanics with the shared native dialog.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve BYOK/platform conditional rendering, provider/model/thinking selections, encrypted-key masked replacement rules, save/delete API semantics, opening brief continuation, and server error messages. Add a labelled dialog with predictable focus, consistent dismissal and pending-request protection. Do not expose key values in logs, screenshots, or test artifacts.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

