---
id: av-tux9
status: closed
deps: []
links: [av-p3gj]
created: 2026-07-15T06:21:33Z
type: bug
priority: 3
assignee: Max Omdal
tags: [ui, gallery, security]
---
# Gallery detail: escape allowlist origins in client-side renderBadges()

The artifact detail page's client-side renderBadges() (web/gallery/detail.js) rebuilds the allowlist toolbar badges after addOrigin()/saveAllowlist() by concatenating each origin straight into innerHTML: allowlist.map(o => '<code>' + o + '</code> '). The origin is not HTML-escaped, so an origin string containing markup (typed into the 'Add origin' input, or already-approved from a crafted allowlist inlined by the server) is injected raw into the app-origin DOM and executes — a self-XSS on the app origin. Scanner/allowlist origins can legitimately contain a literal double-quote and other metacharacters, so this is a real sink, not just theoretical. Pre-existing (predates the epi-q0u2 template extraction); surfaced during that PR's review. Note the server-side render already auto-escapes via html/template's allowlistBadges partial, so this client path is now the lone unescaped origin sink. detail.js has no esc() helper today (index.js/edit.js do).

## Design

Escape each origin before it reaches innerHTML. Either (a) add the same esc() helper index.js/edit.js already use and wrap the origin: '<code>' + esc(o) + '</code>', or (b) build each badge via document.createElement('code') + textContent = o and append, avoiding innerHTML entirely (preferred — no escaping to get wrong). Keep the 'none' muted-span branch. Matches spec §6.2 transparency + the project's rule to HTML-escape scanned origins in any app-origin UI.

## Acceptance Criteria

Adding an origin like https://x"><img src=x onerror=alert(1)> via the toolbar renders it as inert text inside a <code> badge (no script execution, no DOM injection). Existing add/save/revoke allowlist behavior and the 'none' empty state are unchanged. A test (Go asserting served detail.js escapes, or an e2e badge test) covers the injection case.

