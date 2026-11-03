---
id: Exh-t9bv
status: closed
deps: []
links: []
created: 2026-11-03T04:00:50Z
type: bug
priority: 1
assignee: Max Omdal
tags: [gallery, frontend, mobile]
---
# Phone space tabs ride up and leave a blank band when iOS overscrolls the gallery

On an iPhone (home-screen app), scrolling to the bottom of a gallery space and pulling past it moves the Library / Shared with me / Shared by me tab bar up with the content, leaving an empty band of page background below it (screenshot from 2026-11-03, Shared by me).

Cause: the phone tabs are position:sticky; bottom:0 inside .gallery-layout. A sticky box cannot leave its containing block, so once the visible area extends past the layout's end (iOS rubber-band overscroll), the bar stays at the end of the content instead of the bottom of the screen.

## Acceptance Criteria

- Phone tabs stay pinned to the bottom of the viewport while the page overscrolls, whatever the content height.
- Content keeps clearance equal to the tabs' rendered height (wrapped or enlarged labels, home-indicator inset); short galleries gain no spurious scroll range.
- Desktop rail unchanged.
- Browser tests cover a viewport extending past the content end.


## Notes

**2026-11-03T04:26:01Z**

Fixed: phone tabs are position:fixed; space-tabs Stimulus controller observes the bar's border box (the safe-area inset is padding) and publishes --space-tabs-height for the layout's clearance. Regression check ends the content above the viewport bottom, the shape iOS rubber-banding produces.
