---
id: Exh-bb37
status: closed
deps: []
links: []
created: 2026-11-03T05:03:26Z
type: bug
priority: 2
assignee: Max Omdal
tags: [ui, mobile, components]
---
# Dismissing a popup menu also activates what was tapped

Tapping outside an open action menu (gallery card ⋮, detail pill) closes the menu but the same tap also activates whatever was under it: on a phone the only empty space is usually another card, so dismissing the Edit/Duplicate menu opens an artifact. The Clerk account menu (UserButton popover) behaves the same way.

Native menus on iOS, Android and desktop treat the press that dismisses a menu as consumed: it closes the menu and does nothing else.

## Acceptance Criteria

- A tap/click outside an open action menu closes it and does not activate the element under the pointer (card link, another button, a nav tab).
- A scroll gesture starting outside dismisses the menu without leaving a later click swallowed.
- The same holds for the Clerk account menu popover.
- Pressing inside the menu, Escape, Tab and focus moving away keep their current behavior.
- Browser test covers the card-link case.

