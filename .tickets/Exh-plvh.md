---
id: Exh-plvh
status: closed
deps: []
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Palette tokens: paper/dusk ground, ink accent, slate links

Redefine tokens.css for the new palette, keeping the existing token names so page sheets need no colour literals.

Light: ground #f1e9d6, panel #fbf7ec, subtle #f5efdf, field #fffdf6, hairline #e0d6bd, ink #1f2a1c, muted around #5a6453.
Dark (single prefers-color-scheme block): ground #14182e, panel #1d2240, subtle/field #181c36, hairline #2c3258, ink #efe8d6, muted around #a09b8d, on-accent #10142a.
Accent (--brand-blue and tints): ink #1f2a1c (hover #000), dark cream #efe8d6.
New link token: slate #2e5482 (dark #9db6d6), used only for inline text links.
Danger from poppy (#b5331f-ish), warn from sun; pick a success colour that does not read as an accent.
Card shadow: 0 1px 0 rgba(60,45,10,.10), 0 0 0 1px rgba(60,45,10,.07); hover 0 3px 10px rgba(60,45,10,.14), 0 0 0 1px rgba(60,45,10,.09). Dark equivalents in the mock.
Re-theme Clerk's embedded panels through clerk-appearance.js to match.

## Acceptance Criteria

Every text/background token pair passes WCAG AA in light and dark (record the check). No colour literals added to page sheets. Clerk panels match. PR attaches screenshots of gallery, detail, edit and settings in both modes.


## Notes

**2026-11-16T16:22:21Z**

Palette set in tokens.css (paper/dusk, ink accent, slate --color-link for inline links only; danger poppy, warn sun, success muted green; card shadow tokens applied, card hover now uses --shadow-card-hover not an accent ring). Clerk re-themed via clerk-appearance.js. WCAG AA check: internal/api/palette_contrast_test.go (go test ./internal/api -run TestPaletteContrastAA -v) parses tokens.css and checks 4.5:1 for all text pairs, 3:1 for field-border and warn icon, in light and dark; all pass. Lowest text ratio is above 4.5; lowest non-text is dark field-border on panel 3.42. Not changed: tiles/grain, fonts, logo, manifest theme_color (still BrandBlue).
