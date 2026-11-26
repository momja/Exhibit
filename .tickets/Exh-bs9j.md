---
id: Exh-bs9j
status: open
deps: []
links: []
created: 2026-11-26T01:37:07Z
type: chore
priority: 2
assignee: Max Omdal
tags: [mobile, ios, qa, accessibility]
---
# Test artifact behavior and zoom UI on iOS with large accessibility text enabled

Manual QA pass on a physical iOS device with the largest Dynamic Type size enabled (Settings > Accessibility > Display & Text Size > Larger Text, slider at maximum). Verify that artifacts still behave correctly and that the zoom UI remains usable and legible: opening and navigating artifacts, the artifact viewer chrome, zoom controls and their labels at maximum text size, layout at the smallest and largest supported viewports, and no clipped or overlapping controls or horizontal overflow. Confirm the artifact surface itself is unaffected by the host page's text-size setting. Note that this is a verification chore, not a redesign: file separate feature/bug tickets for any defect found rather than fixing it here.

## Acceptance Criteria

Each checklist item is exercised on a real iOS device (not only the simulator) at maximum Dynamic Type and recorded as pass or fail in a note on this ticket. Screenshots capture the zoom UI and an open artifact at maximum text size, saved outside the checkout. Any failure has a linked follow-up ticket with reproduction steps, device/iOS version and dynamic type size. Ticket is closed once the results and follow-up links are recorded.

