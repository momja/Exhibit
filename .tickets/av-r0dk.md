---
id: av-r0dk
status: open
deps: []
links: []
created: 2026-08-16T16:31:30Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-mdc5
tags: [render, security, backend]
---
# Nav bridge: preamble interception + host open, gated on links_approved

Backend half of the link navigation bridge (epic av-mdc5). The sandboxed frame cannot navigate anywhere useful: target=_blank links are dropped (no allow-popups) and no-target links would replace the iframe with a framed-refusal error page. The frame preamble must intercept external link activations and hand them to the host frame, which owns the navigation — mirroring the download (av-ryby) and clipboard (av-hll6) bridges.

**Frame shim** (internal/render/render.go, inside bridgeScript — framed-only, widgets already excluded):
- In the existing capture-phase document click listener, after the blob:/data: download check: resolve closest('a') to an absolute URL (anchor.href); if the protocol is http(s) and the origin differs from the document's own (skip #hash-only, javascript:, mailto:, relative same-document links), e.preventDefault() (no stopPropagation — same contract as the download bridge) and window.parent.postMessage({__avNavigate: true, artifactId: ARTIFACT_ID, url: href}, API_ORIGIN).
- Only the URL crosses the boundary — a pointer to content the artifact already displays, not a capability grant.

**Server**: links_approved boolean column on artifacts (migration alongside the existing downloads/clipboard columns), accepted by PATCH /api/artifacts/:id exactly like downloads_approved — including rejecting non-bool values with a 400 (internal/api/downloads_test.go / clipboard_test.go are the pattern). Never seeds from scans, never affects the CSP.

**Host** (web/gallery/detail.js): listener for __avNavigate validating e.source === frame.contentWindow, d.artifactId === ID, and an http(s) scheme; if linksApproved → window.open(url, '_blank', 'noopener') (transient activation from the click covers the async roundtrip); if not → store as pendingLink {url, hostname} and do not open (child 3 renders the confirmation; this ticket ships the gate and the hook, not the modal).

**Docs**: security.md §4 and architecture.md §6 gain the link-bridge paragraph (sandbox stays the wall; bridge is UX not enforcement; right-click open-in-new-tab already exists, so the bridge adds gesture convenience, not new capability).

## Acceptance Criteria

- A target=_blank http(s) link in an artifact with links_approved=true opens in a new tab from the host; the artifact frame does not navigate.
- Unapproved artifact: clicking a link opens no window and does not navigate the frame (gate holds; pendingLink hook populated for child 3).
- PATCH /api/artifacts/:id {"links_approved": true|false} round-trips; non-bool is a 400.
- The sandbox attribute is unchanged (allow-scripts allow-forms); no allow-popups / allow-top-navigation tokens anywhere.
- Widget renders never intercept (bridgeScript absent from the widget preamble — extend the existing assertion).
- Tests: render_test.go preamble interception (mirror the download-bridge tests), api-level PATCH test (mirror downloads_test.go), detail-page bootstrap asserts linksApproved.

