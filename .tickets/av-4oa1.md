---
id: av-4oa1
status: open
deps: []
links: []
created: 2026-07-20T03:43:03Z
type: feature
priority: 3
assignee: Max Omdal
tags: [draft, gallery, frontend]
---
# Partial re-render for server-rendered chrome (htmx or equivalent)

The gallery's pages are full server renders (epi-q0u2: stdlib html/template + static assets). Anything that changes server-side state after load — a capability approval, an origin decision, a tag edit — leaves the rendered chrome describing the old state until the user reloads the whole page. Today the only options are a full reload (loses iframe/editor state) or hand-rolling DOM updates in page JS that duplicate the template's logic in a second language. Adopt a partial re-render mechanism so a handler can return one server-rendered fragment and the page can swap it in: htmx is the obvious candidate (no build step, no framework, fits the 'one Go binary, no frontend framework' stance), but the decision is open — a small fetch+swap helper over existing template partials may be enough.

## Design

Evaluate htmx vs a ~20-line fetch-and-swap helper before committing; the deciding question is whether we need htmx's attribute vocabulary (triggers, targets, swaps, out-of-band) or just 'GET this fragment, replace this node'. Whatever wins must: (1) serve fragments from the same template partials the full page render uses, so there is exactly one definition of each component's markup; (2) be self-hosted on the app origin like every other asset (no CDN — see technical_stack.md §9 for the icon precedent); (3) not require a Node build step beyond what scripts/build-assets.sh already does. Natural first consumers: the capability cluster/badge partial and the security panel's origin rows.

## Acceptance Criteria

A server-rendered fragment can be re-fetched and swapped into a live page without a full reload, reusing the same template partial as the initial render. At least one real consumer is converted. The mechanism is documented in technical_stack.md §9 and architecture.md §3.5.

