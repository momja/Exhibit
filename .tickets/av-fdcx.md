---
id: av-fdcx
status: open
deps: [av-emh4]
links: []
created: 2026-08-04T04:19:31Z
type: task
priority: 2
assignee: Max Omdal
parent: av-cjkw
tags: [ui, mobile, pwa, frontend]
---
# Serve web app manifest + apple-* home-screen meta tags

Add a manifest.json (name, short_name, icons from [[icon asset ticket]], theme_color, background_color, display: standalone, start_url: '/') served from the app origin, linked via <link rel=manifest> in the shared page head. iOS Safari does not act on the manifest's display mode, so also add the apple-mobile-web-app-capable, apple-mobile-web-app-status-bar-style, apple-mobile-web-app-title, and apple-touch-icon meta/link tags to the same head include used by gallery.tmpl, detail.tmpl, edit.tmpl, new.tmpl, and notfound.tmpl. This is app-origin only — do not touch the render surface's document template.

## Acceptance Criteria

1. GET /manifest.json (app origin) returns valid manifest JSON with icons pointing at the generated PNGs. 2. Every app-origin html/template page's <head> links the manifest and carries the apple-* home-screen tags. 3. Adding to home screen on iOS Safari shows the Exhibit icon and launches standalone (no URL bar/toolbar). 4. Android Chrome's 'Install app' prompt (if triggered) uses the same manifest and icon set.

