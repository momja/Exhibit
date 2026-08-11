---
id: av-3qmf
status: closed
deps: []
links: [av-8zqr, av-s9ti]
created: 2026-08-15T16:40:00Z
type: bug
priority: 2
assignee: Max Omdal
parent: av-cjkw
tags: [ui, mobile, pwa, frontend, a11y]
---
# Focusing a field on mobile zooms the page in and leaves it there

Tap the search box (or any field) on an iPhone and Safari zooms the page in. It does not zoom back out when the field is blurred, so the page stays wider than the screen and the button beside the field — the one you were reaching for — is off it.

This is not the pinch gesture and has nothing to do with the home screen. WebKit zooms whenever it focuses a form control whose text is smaller than **16px**, on the assumption the field is too small to type into. Every field in this app was 12–14px, so every field triggered it.

The fix is the type scale, not the viewport:

- `tokens.css` gains `--field-font-size` (14px) and `--field-code-font-size` (12px, for the monospace source fields), and one `@media (pointer:coarse)` block raises both to 16px.
- Every `input` / `select` / `textarea` / `.cm-editor` rule across the gallery sheets sizes itself from those tokens instead of a literal px value, plus an element-level floor in `components.css` for controls no rule names — an input with no `font-size` inherits the UA's ~13.33px and would zoom just the same.
- The media query keys on pointer type, not width: a narrow desktop window has the same breakpoints and no on-screen keyboard, and a tablet in landscape is wide and still touched.

Because 16px is the exact threshold WebKit uses, removing the *reason* for the zoom leaves zooming itself completely intact — no `user-scalable=no`, no gesture cancelling, no WCAG 1.4.4 exposure, and pinch-to-zoom still works everywhere for everyone.

**Supersedes [[av-8zqr]]**, which attacked this by disabling pinch-zoom in the installed app and then owed a text-size control to make up for it. That was solving the wrong problem: the complaint was never "the pinch fires by accident", it was "focusing a field zooms me in and strands the button". Both are withdrawn.

Measured in Chromium with `hasTouch` emulation (`pointer:coarse` matches, as on a phone): search box, edit title, CodeMirror, agent composer, and paste editor all compute to 16px on touch and stay 14/12px on desktop.

## Acceptance Criteria

1. On a touch device every form control — including both CodeMirror instances — computes to at least 16px. 2. Desktop sizing is unchanged (14px controls, 12px code). 3. Nothing disables or constrains zoom: no `user-scalable`, no `maximum-scale`, no gesture handlers anywhere in the app. 4. New controls size themselves from `--field-font-size` / `--field-code-font-size` rather than a literal px value — enforced by review, not by a test. 5. Manual check on a real iPhone: tapping the search field does not zoom the page, and the clear button stays reachable.
