---
id: av-s9ti
status: closed
deps: []
links: []
created: 2026-08-04T04:19:31Z
type: task
priority: 3
assignee: Max Omdal
parent: av-cjkw
tags: [ui, mobile, pwa, frontend]
---
# Disable pinch-to-zoom on app-origin pages

~~Update the viewport meta tag on the app-origin page templates (gallery, detail, edit, new, notfound) to maximum-scale=1, user-scalable=no so the installed app doesn't rubber-band/zoom like a webpage.~~ **Reverted (PR #89 review):** `maximum-scale=1,user-scalable=no` fails WCAG 1.4.4 (Resize Text) — it blocks pinch-zoom for low-vision users with no alternate text-scaling affordance, and iOS 10+ ignores the directive anyway, so it doesn't even reliably deliver the no-pinch behavior it was meant to add. The viewport meta tag is left as `width=device-width,initial-scale=1` on all five app-origin templates. Do not touch the render surface's own document — artifacts are visitor-authored files and get to set (or not set) their own viewport; overriding it there would violate the 'it's just a file' ownership principle in architecture.md §1.

## Acceptance Criteria

Superseded by the accessibility revert above — pinch-to-zoom is intentionally left enabled on app-origin pages. Original criteria (kept for record): 1. viewport meta on all app-origin templates reads width=device-width,initial-scale=1,maximum-scale=1,user-scalable=no. 2. The render surface's artifact document head is untouched — confirmed, no viewport change leaked into internal/api (render surface) templates. 3. Manual check: pinch gesture on an app-origin page (e.g. the gallery grid) on iOS Safari does not zoom; pinch gesture inside a rendered artifact iframe still behaves however that artifact defines.

