---
id: Exh-gynk
status: in_progress
deps: [Exh-uxyy]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate gallery filter sheet to the shared modal

galleryFilterSheet in internal/api/templates/gallerygrid.tmpl uses native dialog but web/gallery/filter-sheet-controller.js duplicates show/close, Escape, backdrop, and focus-restoration mechanics. Reuse the shared modal behavior while retaining the sheet presentation.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Phone Filters and Done/close controls work across library spaces and multiple gallery instances. Retain bottom-sheet sizing and safe-area treatment, stable dialog root, server-rendered option swaps, search/filter parameters and htmx targets. Escape/backdrop dismissal and return focus to the correct Filter button use the shared behavior. Remove obsolete filter-sheet dialog mechanics.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

