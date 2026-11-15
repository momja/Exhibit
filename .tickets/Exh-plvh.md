---
id: Exh-plvh
status: open
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

Every text/background token pair passes WCAG AA in light and dark (record the check). No colour literals added to page sheets. Clerk panels match. Screenshots of gallery, detail, edit, settings in both modes.

