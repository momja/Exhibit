---
id: Exh-sqny
status: in_progress
deps: [Exh-7hh4]
links: []
created: 2026-10-31T17:02:34Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate direct-open confirmation to the shared modal

directOpenPrompt in internal/api/templates/direct_open.tmpl and web/gallery/direct-open.js manually manage overlay visibility, background inertness, focus containment and Escape priority. Use the shared modal controller for both artifact viewer and agent preview, including capability-triggered opens.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve viewer/artifact-scoped remembered choice, storage-unavailable fallback, current artifact URL after preview swaps, blank-tab claim in the click gesture, opener severance, popup-blocked retry, stale-request guard, cancelled-placeholder cleanup, and exactly-once resolution. Nested capability prompts retain correct topmost Escape/focus behavior. Do not remember cancelled or failed opens. Remove redundant manual inert/focus-trap mechanics and consolidate remaining obsolete overlay styles after all preceding migrations.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

