---
id: Exh-d5ku
status: closed
deps: [Exh-plvh, Exh-bv8l]
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Logo pigments: recolour both logo variants, icons and OG card

Recolour the flat and hanging logos in painting pigments: blue #2f6fc4, red #d9432b, yellow #f2b63c, white #f6efdc, ink #1f2a1c. Regenerate PWA icons and the logo OG card. Decide whether internal/artifacticon favicons move to the same pigments (recommended).

## Acceptance Criteria

Both variants, favicons, PWA icons and the OG card use the pigments; artifact favicon decision recorded.


## Notes

**2026-11-16T23:38:08Z**

Pigments applied to logo.svg, logo-hanging.svg and internal/color/brand.go (new BrandWhite #f6efdc, BrandInk #1f2a1c); TestLogoArtworkUsesBrandPigments pins them. Flat mark on dusk: ink frame is near-invisible against #14182e but the red/yellow/blue blocks and cream cell hold the silhouette (screenshot logos-512-dark, header-dark), so no dark lightening for the flat mark; hanging wire/nail keep #b4bac4 in dark. Manifest theme_color and background_color now paper #f1e9d6 (not blue): a manifest cannot follow the scheme, paper is the ground the standalone app opens on and the splash/status bar then match the page instead of a stray blue bar; PWA icon canvases and the og-default card use the same paper. Artifact favicons (internal/artifacticon) moved to the pigments as recommended: untagged uses red/blue/yellow, background ink #1f2a1c (was #111), white panel cream; so the tab icon is the same family as the logo. Tag colours are unchanged user data. Tests: go test of internal/color, artifacticon and api Logo/Manifest/Favicon/PWA/Palette pass.
