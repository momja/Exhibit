---
id: Exh-wjqh
status: closed
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


## Notes

**2026-11-16T16:21:09Z**

Fonts via shared fontsHead partial. Schibsted wght 400..800; Fraunces pinned opsz72 wght600 SOFT100 WONK0. App origin sets no style/font CSP, so no CSP change; render origin untouched. Verified live load and blocked fallback with Playwright.

**2026-11-16T17:01:25Z**

Round 1 fixes: Clerk/CodeMirror/connections use --font-sans; Fraunces is a pinned static instance so font-variation-settings removed (fontsHead URL is the single source); body font-family consolidated in components.css; tests cover all pages.

**2026-11-17T16:25:30Z**

Decision change (user override of 'no self-hosting'): fonts are now self-hosted via web/fonts (fontsource OFL packages, Fraunces static instance 72/600/100/0 via fontTools instancer). Reason: security audit - Google stylesheet is render-blocking (85s blank pages on black-holed networks) and sends visitor IP/hostname to Google (GDPR).
