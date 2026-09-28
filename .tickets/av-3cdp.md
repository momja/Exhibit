---
id: av-3cdp
status: closed
deps: []
links: []
created: 2026-09-27T19:53:01Z
type: task
priority: 2
assignee: Max Omdal
tags: [frontend, htmx]
---
# Update to htmx v4

Migrate the vendored htmx asset (web/htmx) from 2.x to 4.0 and adapt our usage: renamed htmx:afterSwap event, error-response swap behavior for fragment 404s, custom exhibit:* trigger names vs the : metaCharacter, and any upgrade-checker findings. Docs (technical_stack.md section 9) updated to match.


## Notes

**2026-09-27T19:58:20Z**

Migrated to htmx 4.0.0 on branch feature/av-3cdp/htmx-v4 (pushed). htmx's own upgrade-checker is clean. internal/api suite passes (incl. new noSwap meta assertion); gallery node suite 86/86. Drive-by: fixed stale renderEditPage call that had the internal/api test package unbuildable on main.

**2026-09-27T20:08:40Z**

Follow-up on same branch: replace silent fragment 404s with a user-facing error fragment (narrow noSwap pin to 401/5xx so 404s swap). Same PR.

**2026-09-27T20:11:54Z**

Fragment 404s now swap a user-facing error notice (fragmentError partial + .frag-error); noSwap narrowed to 401/5xx. Full internal/api suite + gallery 86/86 pass.

**2026-09-28T04:30:52Z**

Removing noSwap entirely: generic 500 fragment + HX-Refresh on 401, then delete the meta pin.

**2026-09-28T04:33:31Z**

noSwap removed entirely: fragmentServerError (generic 500 fragment), HX-Refresh on fragment 401s, meta tags deleted. Full api suite green.
