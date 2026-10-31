---
id: Exh-1grc
status: in_progress
deps: [Exh-yrgf]
links: []
created: 2026-10-31T17:02:33Z
type: task
priority: 2
assignee: Codex
tags: [ui, components, modal-migration]
---
# Migrate runtime network permission dialog to the shared modal

networkPrompt in internal/api/templates/partials.tmpl and web/gallery/network-prompt.js are shared by detail and agent pages, but implement custom overlay/focus/dismissal mechanics separately from the modal controller. Migrate both consumers together.

## Design

Follow docs/ui-components.md and the latest existing modal-controller.js contract: Go partials own markup and shared CSS owns modal presentation; Stimulus owns browser lifecycle and dismissal. Feature code retains data, API operations, change events, approval/cancellation decisions, and feature-specific guards. Use scoped targets, app-origin native ES modules, and existing Phosphor assets. Extend the small shared interface only for concrete requirements; avoid a flag-heavy framework. Keep htmx-rendered markup server-owned. Preserve intentional per-feature behavior rather than forcing every feature into the same save/dismiss policy. Built-in browser confirm/prompt/beforeunload UI and provider-owned Clerk modals are outside these app-owned dialog migrations.

## Acceptance Criteria

Preserve owner-only decisions, validated message sources, queued origins, safe initial focus on Block once, per-origin allow/block endpoints, transient dismissal semantics, error reporting and capability refresh events. Escape/backdrop advance the queue exactly once; late writes cannot consume the next request. Allow reloads the correct current frame and clears stale queued reports. Handle simultaneous/stacked prompts without Escape closing unrelated dialogs. Convert substantially rewritten script to an imported native module and update both consumers.

The dialog uses shared native modal behavior instead of duplicating focus, Escape, backdrop, and lifecycle mechanics. Real-browser checks cover open/close, keyboard focus and restoration, desktop/phone layout, relevant save/error paths and htmx removal/reconnection. Existing affected Go/Node suites pass; add meaningful regression cases for the migrated behavior and retain native-dialog coverage rather than only mocking it. Update component documentation and current screenshots for visible changes. Close this ticket on its own feature branch before opening its stacked PR; never develop on, merge to, or push main.

