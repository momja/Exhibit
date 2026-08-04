---
id: av-5ztj
status: in_progress
deps: []
links: []
created: 2026-08-04T05:12:56Z
type: bug
priority: 1
assignee: Max Omdal
---
# Docker asset build fails: pwa-icons build.mjs can't find logo.svg

Deployment build failed: RUN sh scripts/build-assets.sh in the Dockerfile's assets stage exits with ENOENT for /app/internal/api/logo.svg. The web/pwa-icons workspace (added in 8751ec3) reads internal/api/logo.svg to rasterize PWA icons, but the assets build stage only COPYs web/ and scripts/ into its build context, never internal/api/logo.svg.

## Acceptance Criteria

docker compose -f compose.yml build succeeds; web/pwa-icons/build.mjs finds internal/api/logo.svg in the assets stage.

