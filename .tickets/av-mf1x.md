---
id: av-mf1x
status: closed
deps: [av-4oa1]
links: [av-isb3, av-41se, exhibit-fr7]
created: 2026-07-20T03:43:19Z
type: bug
priority: 3
assignee: Max Omdal
tags: [gallery, frontend, security]
---
# Capability badge goes stale after a runtime origin approval

On the artifact viewer, approving an origin from the runtime network prompt (exhibit-fr7) writes the allow decision and transparently reloads the iframe so the new CSP takes effect — but the toolbar's capability cluster/badge (av-isb3) is server-rendered from the artifact's allowlist at page load, so it keeps reading e.g. 'Sandboxed' (0 origins) until the user reloads the page. The artifact's actual posture and the badge describing it disagree, which is the one place that must not drift. Same class of staleness applies to the popover's origin list (av-41se). Reproduced during exhibit-fr7 verification; the fix was deliberately deferred rather than hand-rolling a second, JS-side copy of the badge's rendering logic.

## Design

Do not rebuild the badge in page JS — that duplicates the capabilityCluster/capabilityPopover partials' logic in a second language and is exactly the drift av-4oa1 exists to avoid. Once partial re-render lands, have the prompt's Allow handler (web/gallery/detail.js) re-fetch the capability cluster fragment for the artifact and swap it, alongside the iframe reload it already performs. A full window.location.reload() is the cheap alternative but throws away the transparent-reload property exhibit-fr7 was built for (and the artifact's in-frame scroll/UI state).

## Acceptance Criteria

After approving an origin from the runtime prompt, the toolbar badge and its popover reflect the new allowlist without a manual page reload, and the iframe reload stays transparent.


## Notes

**2026-09-27T20:01:13Z**

Implemented via htmx (av-4oa1's mechanism, delivered by av-6m3e): new GET /partials/capability-cluster fragment rendering the same capabilityCluster partial; detail toolbar wraps the cluster in an htmx target on exhibit:capabilities-changed; network-prompt.js Allow handler fires the event before the transparent frame reload. Recipients read the fragment minus the Manage link; strangers 404. Follow-up (same staleness class, out of scope): the first-use capability-bridge approvals in detail.js (dl/clip/link/media Allow) also widen posture without refreshing the badge — one event fire each once covered by tests. Incidental: fixed pre-existing stale renderEditPage call in gallery_test.go:568 (failed package build on main).

**2026-09-27T20:19:32Z**

Scope correction (2026-09-27): the hx-status 4xx/5xx guards from the previous note are REVERTED — feature/av-3cdp/htmx-v4 owns that ground and takes the opposite direction (noSwap meta pin to 401/5xx; 404s swap a user-facing fragmentError). A 4xx guard here would have suppressed those error swaps. Trial merge against origin/feature/av-3cdp/htmx-v4 is conflict-free. Remaining follow-up after av-3cdp lands: one line in capabilityClusterPartial — answer fragmentNotFound instead of plain-text 404, like the other /partials/* routes (comment in code marks it). Also note: main carries a second 'mark in progress' commit (a3c7682) duplicating this branch's 1430266; expect a trivial ticket-file conflict at PR time (keep closed).
