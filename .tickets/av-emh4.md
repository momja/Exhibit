---
id: av-emh4
status: open
deps: []
links: []
created: 2026-08-04T04:19:31Z
type: task
priority: 2
assignee: Max Omdal
parent: av-cjkw
tags: [ui, mobile, pwa, build]
---
# Generate PWA icon assets from the compiled-in logo

internal/api/logo.go compiles in the Exhibit brand mark as an SVG (exhibitLogoSVG) and a base64 data: URI (exhibitLogoDataURI) already used as the favicon. Home-screen icons need real raster PNGs at fixed sizes (iOS apple-touch-icon wants 180x180 minimum; the web manifest wants 192x192 and 512x512, plus a 512x512 'maskable' variant with safe-area padding for Android's adaptive-icon mask). Render these from the same SVG source at build time so there is exactly one place the logo is defined.

## Acceptance Criteria

1. icons rendered from internal/api/logo.go's SVG (or a shared source it and the icon step both consume — no hand-drawn duplicate). 2. Sizes produced: 180x180 (apple-touch-icon), 192x192 and 512x512 (manifest 'any' purpose), 512x512 maskable (logo padded to fit within the safe circle, per web.dev/maskable-icon). 3. Generation runs as part of the existing Node/esbuild asset pipeline (docs/build_assets.md) and outputs into internal/api/assets/ for go:embed, consistent with how Phosphor icons and htmx are vendored. 4. Generated PNGs are not committed to git (matches the existing 'Node-built assets ... not committed' policy in technical_stack.md §13).

