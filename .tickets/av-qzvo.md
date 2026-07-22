---
id: av-qzvo
status: open
deps: []
links: []
created: 2026-07-22T04:49:21Z
type: feature
priority: 2
assignee: Max Omdal
tags: [mobile, pwa, ui]
---
# Support 'Add to Home Screen' PWA mode

When the app is launched from an iOS/Android home-screen icon (standalone/PWA display mode), suppress the browser search/address bar and disable pinch-to-zoom so the app feels like a native app rather than a web page.

## Design

Detect standalone mode via the 'display-mode: standalone' media query (and/or navigator.standalone on iOS). When standalone, apply layout/CSS adjustments so no browser chrome/search bar affects the viewport. Disable pinch-to-zoom by setting the viewport meta tag's user-scalable=no / maximum-scale=1 (only when running standalone, not for the regular browser-tab experience, to avoid harming accessibility for normal web visitors). Add a web app manifest (display: standalone, icons, theme-color) so 'Add to Home Screen' produces a proper standalone launch.

## Acceptance Criteria

- Adding the site to the home screen on iOS Safari and Android Chrome launches it in standalone display mode (no address/search bar).
- Pinch-to-zoom is disabled when running in standalone/home-screen mode.
- Normal browser-tab visits (not installed) are unaffected: address bar and pinch-to-zoom behave as before.
- A web app manifest is present with correct icons and theme-color.

