---
id: Exh-90b2
status: open
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

