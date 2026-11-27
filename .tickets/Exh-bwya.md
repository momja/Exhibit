---
id: Exh-bwya
status: in_progress
deps: []
links: [Exh-od5a]
created: 2026-11-20T16:09:54Z
type: bug
priority: 3
assignee: Max Omdal
tags: [ui, login, ios]
---
# Sign-in painting stops at the iOS Safari toolbar

On iOS Safari 26 (iPhone, dark mode) the hosted sign-in page's painting ends at the top of the floating bottom toolbar. The strip around the toolbar is filled with one flat colour (black in dark mode, because the hosted page's `contain:paint` on `<body>` stops the body background reaching the canvas). Reported during the Exh-od5a brand-refresh demo with screenshots.

Tried on feature/Exh-od5a/brand-refresh and reverted, none of which made the painting continue under the toolbar on a real device:
- dropping `contain:paint` and giving the root a `--painting-ground` colour sampled from the art's bottom edge (the strip took the colour, the painting still stopped at the bar);
- stretching the phone scene to `max(100%,100lvh)`;
- `viewport-fit=cover` on the sign-in pages with safe-area padding and a fit calculation above the bottom inset;
- fading the painting's last 56px into the ground colour.

Community reports on Safari 26 say it ends the viewport above its floating toolbar and fills the strip with a colour sampled from fixed elements near the edge, or the html/body background (https://1ar.io/updates/safari-26-liquid-glass-web/, https://jahir.dev/blog/safari-toolbar). Content that scrolls may show through the glass; that is untested here and would make the sign-in page scroll and need the Clerk card's fit-to-viewport logic (clerk-controller.js fitLoginToViewport) reworked.

## Acceptance Criteria

On a physical iPhone running iOS Safari 26, in light and dark mode, the sign-in page shows no hard edge or black band at the toolbar; the header and card stay clear of it; desktop layout and the card's fit-to-viewport behaviour are unchanged.


## Notes

**2026-11-27T16:44:32Z**

Candidate: allow the hosted phone document to scroll beyond its viewport-height body and extend the painted scene into that space, while retaining the body height used by Clerk fit. Safari 26 toolbar compositing remains unverified; physical iPhone testing in light and dark is required before acceptance.
