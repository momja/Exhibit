---
id: Exh-90b2
status: closed
deps: [Exh-plvh, Exh-wjqh, Exh-d5ku]
links: [Exh-s7kn]
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Painted sign-in page (static)

Restyle /auth/login (login.tmpl, clerk.tmpl, login.css) as the painted front door: day painting in light mode, dusk in dark; flat logo in the header; Fraunces headline and tagline; rounded sign-in box (10px corners, soft shadow, ink button) for the local form and the hosted Clerk widget alike; grain over the painting; phone layout with sky gradient and the painting at the bottom; responsive images.

Scope is the sign-in page only, not a public page for signed-out visitors. Headline copy may overlap Exh-s7kn (onboarding copy); agree wording there. Can start with the mock paintings until the production art lands.

## Acceptance Criteria

Local and Clerk login both styled in light and dark on desktop and phone; fields usable on first paint; text contrast passes AA over the painting; screenshots attached to the PR.


## Notes

**2026-11-17T07:29:30Z**

Contrast (cream text #fbf7ec, measured on rendered screenshots with text hidden): headline/tagline/wordmark sit on a translucent scrim chip (--painting-scrim, navy .72 light / .68 dark) so AA does not depend on the pixels behind (needed for Exh-vsw0, which moves the sky under the text). Over the real painting: light 8.2-9.1:1, dark 14.5-15.6:1 across 1280/1920/390/375 widths. Worst case with pure white behind the chip: 5.53:1 light, 6.48:1 dark (tagline needs 4.5, large text 3). Without the chip, the first attempt measured only 3.0-3.5:1 in light, so a gradient scrim tuned to the art was rejected. Headline copy 'A shelf for small tools.' kept from mock; overlaps Exh-s7kn onboarding copy (not edited). Italic 'small' dropped: only one static upright Fraunces instance is loaded (Exh-wjqh).

**2026-11-17T07:52:43Z**

Rework (chips removed, per design review): text sits bare on the sky with a feathered radial shade (--painting-shade, scene ::after) plus soft text-shadow; headline large text, tagline Schibsted 600 19px and wordmark 20px bold treated as large text (need 3:1). Worst case measured WITHOUT text-shadow, p99 background luminance under each text line, with the day/dusk sky layer swept over its full 832px descent offset range (11 steps, desktop 1280x800 and 1920x1080): light headline 4.01, tagline 4.36, wordmark 4.71 (1280) / 4.54, 4.66, 4.80 (1920); dusk 9.26-13.43. Minimum is always the at-rest offset. At-rest composites incl. phone crops: light 4.00-5.57, dusk 10.4-12.7. Supersedes the chip numbers (5.53/6.48 over white). Clouds/hill layers only rise from below the text zone so are not behind it; phone sky layer not swept (phone uses the composite crop). Italic 'small': fontsHead now requests Fraunces ital,opsz,wght,SOFT,WONK static italic instance; verified document.fonts shows 'Fraunces italic loaded' with font-synthesis none.
