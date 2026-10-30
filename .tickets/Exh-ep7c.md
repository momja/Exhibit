---
id: Exh-ep7c
status: in_progress
deps: []
links: []
created: 2026-10-30T03:26:47Z
type: bug
priority: 2
assignee: Max Omdal
---
# Keep the Settings fallback link hidden beside the Clerk avatar

The gallery header shows Settings beside the signed-in Clerk avatar even though Settings belongs inside its account menu. Reproduce visibility using the actual account partial and production CSS, keep recovery available only after Clerk fails, and cover the closed/open menu behavior in a browser.

## Acceptance Criteria

Signed-in header shows only the avatar when collapsed; Settings is available in its account menu; an actual Clerk loading failure can still reveal the recovery link.

