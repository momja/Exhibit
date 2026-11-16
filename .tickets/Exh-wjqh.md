---
id: Exh-wjqh
status: in_progress
deps: []
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Fonts: Schibsted Grotesk and Fraunces from Google Fonts

Load Schibsted Grotesk (text) and Fraunces (one headline per page) from Google Fonts; no self-hosting. Add font tokens to tokens.css and apply Schibsted to body text. Fraunces with WONK 0, SOFT 100, opsz 72 for page titles only.

Request only the axes and weight ranges used, with display=swap. Fallbacks: system-ui for Schibsted, Georgia for Fraunces, so offline or blocked instances still render.

Note in technical_stack.md §1 that fonts deliberately come from a CDN, unlike icons and htmx.

## Acceptance Criteria

Fonts load on app pages and the sign-in page; text renders acceptably with the fonts blocked; technical_stack.md updated.

