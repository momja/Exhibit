---
id: av-mdc5
status: open
deps: []
links: []
created: 2026-08-16T16:31:12Z
type: epic
priority: 2
assignee: Max Omdal
tags: [render, security]
---
# Epic: External link navigation — host-mediated link bridge with first-use approval

External links are dead inside the artifact frame today. The render iframe is sandbox="allow-scripts allow-forms", so a target="_blank" link is an auxiliary navigation: without allow-popups the click is silently dropped (right-click → Open in new tab still works because browser chrome is not subject to the sandbox). A plain no-target link would navigate the iframe itself, replacing the artifact with an external page that usually refuses framing (X-Frame-Options / frame-ancestors) — an error page where the tool used to be.

Decision: a **navigation bridge** — the third sibling of the download (av-ryby) and clipboard (av-hll6) capability bridges — NOT new sandbox tokens. allow-popups + allow-popups-to-escape-sandbox would (1) not fix no-target links eating the iframe, (2) grant artifacts the all-or-nothing ability to pop windows on any click anywhere in their UI, and (3) violate the documented principle that the sandbox stays the wall and bridges are UX, not enforcement (security.md §4). The bridge intercepts the common vector — anchor activations — and anything it misses stays sandbox-blocked, exactly like downloads.

Shape (mirrors the existing bridges end to end):
- **Frame** (bridgeScript in internal/render/render.go, framed-only): capture-phase click listener, after the blob:/data: download check, resolves closest('a') to an absolute URL; external http(s) links postMessage {__avNavigate, artifactId, url} pinned to API_ORIGIN and preventDefault (no stopPropagation). Widgets already omit bridgeScript, so tiles can never navigate.
- **Host** (web/gallery/detail.js): validates e.source, scheme, and artifact; when links_approved opens the URL in a new tab via window.open(url, '_blank', 'noopener') — the click's transient activation (~5 s) covers the postMessage roundtrip. Unapproved → stashed for the first-request confirmation (child 3).
- **Approval** is per-artifact, first-use, server-side (links_approved, PATCH through the single write path), revocable from the edit page (child 2). Denial just drops the navigation, mirroring downloads.
- No CSP/allowlist interaction: the popup is its own top-level document governed by the target site's CSP.
- Top-level renders and shares already navigate natively and need nothing (bridgeScript installs only when framed).

Children (all base on child 1's branch, per the TICKETS.md merge-branch note for dependent issues):
1. Backend bridge: preamble interception + host open + links_approved schema/PATCH, gated (unapproved → no window). Includes the security.md/architecture.md paragraphs.
2. Edit-page toggle: third capability row (Ask first / Always allow) + capability popover row + view models — the revocable surface.
3. First-request confirmation modal on the detail page with the exact approved copy.

Out of scope: wrapping window.open inside the frame (artifacts calling it directly stay sandbox-blocked; follow-up if demand appears), form submissions, non-http(s) schemes.

