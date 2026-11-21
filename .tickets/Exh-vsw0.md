---
id: Exh-vsw0
status: closed
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


## Notes

**2026-11-17T16:13:18Z**

Descent is CSS-only and server-decided (scriptless page kept). Cookie exhibit_descent_seen=1 (host-only, HttpOnly, Lax, Secure by APP_ORIGIN) set on first sign-in render, cleared at logout; Clerk afterSignOutUrl now /auth/logout so menu sign-out re-arms it (expiry does not). Layers are CSS image-set backgrounds under prefers-reduced-motion:no-preference: reduced-motion and repeat visits download none. Worst-case text contrast over the whole descent (p99 bg, no shadow, 150ms steps, 1280/1920/390): light headline 3.90/4.46/5.49, tagline 4.49/4.58/5.43, wordmark 4.71/4.82/5.56; dusk 10.1-11.9. Layers-only final frame vs static composite: mean abs diff 1.2-1.9/255 desktop (codec noise, geometry exact), 2.8-3.3 phone (layers 1600w vs 1170w crop). Clerk hosted flow not exercised locally.
