---
id: Exh-ep7c
status: closed
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


## Notes

**2026-10-30T03:32:25Z**

Reproduced with the production header template and CSS: recovery.hidden was true, computed display was flex, and the status was empty. The gallery header anchor display rule overrode the browser hidden default. Added a scoped hidden rule to account-control. Browser coverage now checks the collapsed header, Settings inside the open menu, Settings-page header visibility, and recovery after a simulated Clerk outage. The regression failed before the CSS fix and go test ./internal/api -run Test(Clerk|GalleryHeader) passes after it; asset build and git diff --check also pass.
