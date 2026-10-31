---
id: Exh-p3ah
status: in_progress
deps: []
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate tag editor to the shared modal

Tag editing in the artifact editor uses editTagModal in internal/api/templates/partials.tmpl and its own overlay, Escape/backdrop listeners, and ID-based opening/closing in web/gallery/tags.js. Adopt the existing shared native-dialog controller used by Duplicate. This first stack layer may make small shared-modal improvements required by the migrations.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

The Tags panel pencil opens the correct tag with its current name/color. Rename, recolor, global delete, API errors, and exhibit:tags-changed htmx refresh still work. Reopening resets the correct feature state. Return focus to the invoking pencil or a deliberate visible fallback when refresh replaces it. Reuse shared modal CSS and keep the dialog outside replaced form regions.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

