---
id: av-e3sj
status: open
deps: [av-r0dk]
links: []
created: 2026-08-16T16:31:31Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-mdc5
tags: [ui, gallery, security]
---
# Link bridge first-request confirmation modal

First-request confirmation for the link bridge (epic av-mdc5; the child backend ticket ships the gate + pendingLink hook — this ticket renders the decision UI and performs the grant).

**Modal** (internal/api/templates/detail.tmpl, mirroring dl-modal/clip-modal): title "Allow opening links?" with the approved copy:

> You are opening a link to a site shown here. Exhibit cannot verify the safety of external sites. Make sure you trust this destination before allowing links. You can revoke this at any time from the toolbar.

Rendered as: You are opening a link to <code id="link-host">example.com</code>. Exhibit cannot verify the safety of external sites. Make sure you trust this destination before allowing links. You can revoke this at any time from the toolbar. The destination hostname is filled from pendingLink.hostname. Buttons Block / Allow links (Phosphor icon).

**Host logic** (web/gallery/detail.js): on __avNavigate when !linksApproved → populate #link-host, show the modal, keep the pending URL; Allow → setCapabilityApproved('links_approved', true, 'links') then window.open(pendingUrl, '_blank', 'noopener'); Block / Escape / overlay click → drop the URL, artifact keeps running. Denial persists nothing (mirrors downloads: denial drops, approval persists). Subsequent links open without a prompt — the grant is per-artifact, which is what "before allowing links" states.

## Acceptance Criteria

- First external-link click on an unapproved artifact shows the modal with the exact copy above and the destination hostname; Block drops it, Allow opens the tab and persists the grant server-side.
- Second link opens directly (no prompt) within the same session and after a reload (grant read from the bootstrap).
- The modal is accessible (role=dialog, aria-modal, aria-labelledby) and dismissible by Escape and overlay click, matching the existing modals.
- Tests: detail-page template asserts the copy and hostname span; the approval flow is covered at the api level mirroring clipboard_test.go.

