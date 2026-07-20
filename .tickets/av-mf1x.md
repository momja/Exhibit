---
id: av-mf1x
status: open
deps: [av-4oa1]
links: [av-isb3, av-41se, exhibit-fr7]
created: 2026-07-20T03:43:19Z
type: bug
priority: 3
assignee: Max Omdal
---
# Capability badge goes stale after a runtime origin approval

On the artifact viewer, approving an origin from the runtime network prompt (exhibit-fr7) writes the allow decision and transparently reloads the iframe so the new CSP takes effect — but the toolbar's capability cluster/badge (av-isb3) is server-rendered from the artifact's allowlist at page load, so it keeps reading e.g. 'Sandboxed' (0 origins) until the user reloads the page. The artifact's actual posture and the badge describing it disagree, which is the one place that must not drift. Same class of staleness applies to the popover's origin list (av-41se). Reproduced during exhibit-fr7 verification; the fix was deliberately deferred rather than hand-rolling a second, JS-side copy of the badge's rendering logic.

## Design

Do not rebuild the badge in page JS — that duplicates the capabilityCluster/capabilityPopover partials' logic in a second language and is exactly the drift av-4oa1 exists to avoid. Once partial re-render lands, have the prompt's Allow handler (web/gallery/detail.js) re-fetch the capability cluster fragment for the artifact and swap it, alongside the iframe reload it already performs. A full window.location.reload() is the cheap alternative but throws away the transparent-reload property exhibit-fr7 was built for (and the artifact's in-frame scroll/UI state).

## Acceptance Criteria

After approving an origin from the runtime prompt, the toolbar badge and its popover reflect the new allowlist without a manual page reload, and the iframe reload stays transparent.

