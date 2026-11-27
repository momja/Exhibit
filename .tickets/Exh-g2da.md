---
id: Exh-g2da
status: closed
deps: []
links: []
created: 2026-11-27T21:05:02Z
type: chore
priority: 2
assignee: Max Omdal
---
# Refresh README screenshots and brand mark

README.md still shows the pre-brand-refresh logo (design_files/exhibit_logo.svg: hanger, saturated yellow) and a single October screenshot of the old gallery. Replace both with the current production mark (internal/api/logo.svg) and recent captures of the shipped UI, stored under docs/screenshots/readme/ and referenced from README.md.

## Acceptance Criteria

Rendered README images resolve; no dead relative paths.


## Notes

**2026-11-27T21:06:04Z**

README.md now uses internal/api/logo.svg (the flat mark logo.go documents as the brand mark; design_files/exhibit_logo.svg stays as the source artwork) and four recent captures under docs/screenshots/readme/: Starred gallery light and dark, artifact detail, public share. Captures come from the exhibit-ui-map crawl at f4a8d32; git diff --stat f4a8d32..HEAD over internal/api/templates, web/gallery and web/hero-art is empty, so they show HEAD's UI. Removed docs/screenshots/exhibit_screenshot.png (only reference was README). Rendered README.md through the GitHub markdown API with context=momja/Exhibit and screenshotted it: all four images load, side-by-side pairs stay on one row.
