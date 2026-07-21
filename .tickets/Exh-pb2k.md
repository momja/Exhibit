---
id: Exh-pb2k
status: in_progress
deps: []
links: []
created: 2026-07-21T05:37:14Z
type: bug
priority: 2
assignee: Max Omdal
tags: [ui, css]
---
# FOUC / white-flash between page navigations

Exhibit's gallery is a classic MPA (internal/api/templates/*.tmpl, docs/technical_stack.md §9) — gallery/detail/edit are separate full-document server renders, no client router. On every navigation between them (e.g. clicking an artifact card, Edit, ← Gallery) the browser unloads the current document synchronously and paints a blank default-white frame before the next document's render-blocking CSS is ready, even though every page's stylesheet chain (phosphor -> tokens.css -> components.css -> page.css) is already correctly render-blocking in <head>. Because the app's own page background (#f0f0f0, set in tokens.css/index.css/detail.css/edit.css body rules) is not pure white, this default-UA-white gap between two gray pages reads as a visible flash on every navigation.

## Acceptance Criteria

Cross-document navigations between gallery/detail/edit opt in to the CSS View Transitions API (Chrome 126+) so the browser keeps the outgoing page's rendered snapshot on screen and cross-fades to the incoming page instead of showing a blank white frame. Enabled globally via a single '@view-transition { navigation: auto; }' rule in web/gallery/tokens.css (loaded first, on every page) rather than per-page, since it is a document-level opt-in. Pure progressive enhancement: browsers without View Transitions support ignore the at-rule and navigation is unchanged from today.

