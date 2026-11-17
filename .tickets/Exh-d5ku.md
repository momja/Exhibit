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

**2026-11-17T02:35:05Z**

Round-1 corrections. The earlier claim that the manifest paper makes the status bar match the page was wrong in dark mode: pwaHead now carries two theme-color metas (paper #f1e9d6 for light, dusk #14182e for dark), pinned with the manifest and web/pwa-icons/build.mjs to --color-ground by TestPaperAndDuskFollowGroundTokens. Flat mark frame stroke thickened 1.006 -> 1.5 (hanging wire stroke 1.5 too, legs on the new outer top edge y 46.797; viewBoxes widened to fit; 404 pivot still 48.3% 4.2%): at 16/18px the cream cell now keeps a visible edge on paper and a visible frame on dusk (small-mark-before/after.png). Kept. Artifact favicon docs now say cream, not white; the old blue remains only in artifact fixtures (mockllm, samples).
