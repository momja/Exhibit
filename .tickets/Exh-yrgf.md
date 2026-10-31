---
id: Exh-yrgf
status: in_progress
deps: [Exh-b98w]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate editor unsaved-changes dialog to the shared modal

editorLeaveDialog in internal/api/templates/editnavigation.tmpl is a native dialog opened/closed directly by web/gallery/unsaved-changes-controller.js. Compose the existing editor-session coordinator with shared modal presentation and lifecycle; the coordinator retains navigation and draft decisions.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Keep editing, discard, and save-and-leave retain their behavior, including partial saves, edits typed during saves, pending-save waits, failed saves, pageshow/Back restoration, and confirmed restore/delete exit bypass. Browser beforeunload protection remains native. Preserve existing dismissal policy and prevent dismissal during a save. Keep navigation links and focus restoration correct; shared mechanics must not cause duplicate prompts or navigate after cancellation.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

