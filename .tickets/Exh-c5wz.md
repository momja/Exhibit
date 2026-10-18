---
id: Exh-c5wz
status: open
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-c2hu
tags: [versions, state, history]
---
# Bind agent context and history rendering to the accepted request

Remaining #150–#152 findings: a follow-up replaces the active turn's selection file even if screened out; attaching to a live conversation omits the reread notice; history navigation can change the displayed transcript during resume; persisted messages overlap replayed SSE output. Two integration assertions also observe DB writes before expected tool calls are recorded.

## Design

Keep immutable selection context associated with the accepted prompt being consumed. Synchronize a live continuation reread notice and retain it across guardrail refusals. Tie resume completion to its selected conversation identity and serialize/invalidate navigation. Establish a persisted-snapshot/event cursor boundary to prevent duplicate transcript output. Fix recorder tests to wait on their observable expected tool calls rather than inferring them from an earlier DB write. The get_selection label reported on #150 is already supplied by #151; verify before changing it.

## Acceptance Criteria

Queued/blocked follow-ups cannot change another turn's selection. Live attachment after an external edit rereads current source on its first accepted prompt. Delayed resume plus navigation cannot show another conversation; attaching does not repeat completed replies/tools. Repeated focused integration runs eliminate the known recorder-observation race.


## Notes

**2026-10-18T23:13:17Z**

Release hardening fixes the two recorder-observation test races from #150: source and selected-element context tests now wait for the exact expected tool-call sequence after the save becomes visible. The product context, selection-isolation, live-reread and cursor/navigation items remain open.
