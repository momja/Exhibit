---
id: av-cjkw
status: open
deps: []
links: []
created: 2026-08-04T04:19:11Z
type: epic
priority: 2
assignee: Max Omdal
tags: [ui, mobile, pwa, frontend, epic]
---
# Mobile PWA polish — home-screen install, standalone chrome, no pinch-zoom

Make the app-origin gallery UI installable and feel native when added to a mobile home screen (iOS Safari primarily, Android Chrome as a secondary beneficiary of the same manifest). Three concrete asks: (1) the existing compiled-in logo becomes the home-screen icon, (2) the browser chrome (Safari's URL bar/toolbar) is hidden when launched from the home screen via standalone display mode, (3) pinch-to-zoom is disabled on the app's own pages so it behaves like a native app rather than a webpage. Scope is the app origin only — gallery, detail, edit, new, notfound. The render origin (RENDER_ORIGIN, where artifact bodies execute per docs/architecture.md §3.2) is explicitly out of scope: artifacts are visitor-authored documents, and their own viewport/zoom behavior is theirs to control, not ours to override.

## Design

Standard web PWA primitives, no framework: a manifest.json served from the app origin, PNG icons rendered at build time from the existing compiled-in SVG logo (internal/api/logo.go), apple-* meta tags for iOS (which does not read manifest.json display mode the way Android does), and a viewport meta tag change (maximum-scale=1, user-scalable=no) applied only to the html/template pages under internal/api/templates/ that serve the app shell — never to the render surface's served artifact documents. Icon generation should live in the existing Node/esbuild asset-build pipeline (docs/build_assets.md) alongside the Phosphor icon vendoring, not as a new build system.

## Acceptance Criteria

1. A manifest.json is served from the app origin with name, short_name, icons (192, 512, and maskable variants), theme_color, background_color, display: standalone, and start_url. 2. iOS 'Add to Home Screen' uses the app logo as the home-screen icon (apple-touch-icon present at the sizes iOS expects). 3. Launching from the iOS home screen opens without Safari's URL bar/toolbar (apple-mobile-web-app-capable + status-bar-style set; display: standalone honored on Android). 4. Pinch-to-zoom is disabled on app-origin pages (gallery, detail, edit, new, notfound) via the viewport meta tag. 5. The render origin's artifact documents are unmodified — no manifest, no apple-* tags, no viewport changes there. 6. Icons are generated from the single existing logo source (internal/api/logo.go), not a separately maintained asset, so the two never drift. 7. Verified on an actual iOS Safari 'Add to Home Screen' — home screen icon, standalone launch (no chrome), and no-pinch-zoom all confirmed on-device or via simulator screenshot.

