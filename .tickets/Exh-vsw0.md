---
id: Exh-vsw0
status: open
deps: [Exh-90b2, Exh-qhvv]
links: []
created: 2026-11-15T02:03:46Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-od5a
tags: [brand, ui]
---
# Sign-in descent animation

Animate the sign-in page as a camera descending from the sky: sky, clouds and hill layers move at different rates into the final composition (mock: about 3.6s, cubic-bezier(.5,0,.15,1)).

Plays once: on the first visit and after sign-out; otherwise the page shows the final frame. No skip control: the sign-in form is in place and usable from the first frame, so only the scenery moves and nothing waits on the animation. With prefers-reduced-motion: reduce, show the final frame with no motion.

## Acceptance Criteria

Animation plays on first visit and after sign-out only; form never moves, fades in late or loses focus; motion ends within 5s (WCAG 2.2.2 needs no pause control below that); reduced-motion users see the static final frame; no layout shift.

