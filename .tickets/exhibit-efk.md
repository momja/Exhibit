---
id: exhibit-efk
status: closed
deps: [exhibit-w9y]
links: []
created: 2026-05-31T22:30:56Z
type: feature
priority: 1
---
# Web gallery UI: templ + htmx + Tailwind

Gallery page showing artifact grid, search, tag/collection filters. Upload via drag-drop or paste. Server-rendered partials via htmx. Detail view with CodeMirror source + iframe renderer.



## Notes

**2026-07-07T01:28:55Z**

Retrospective (2026-07-06, av-ijew doc audit): this ticket's FEATURE mostly shipped, but never with the stack in the title.

- templ/htmx/Tailwind were never implemented: a-h/templ never appeared in go.mod in any commit, 'templ generate' was never run (no *_templ.go ever committed), htmx/Alpine never appear in any code commit.
- The only trace was web/templates/gallery.templ — dead scaffolding from the initial v1 commit (34c0e99), never referenced by the build. It loaded htmx/Alpine from third-party CDNs, which would violate the later self-hosted/no-CDN rule. Deleted in PR #28 (feature/av-ijew/docs-sync).
- 34c0e99 shipped only the API. The actual gallery landed in f076bdc as hand-written server-rendered Go string HTML (internal/api/gallery.go), which is still the approach today. Docs updated in PR #28 to describe this; per project decision, adopting a template engine/framework would be a new story.

What shipped vs the ticket body: gallery grid + search YES; paste upload YES (drag-drop NO — no drop handler); detail view with CodeMirror + iframe YES (CodeMirror later, exhibit-ay7); htmx partials NO (full-page renders instead, deliberate); tag/collection FILTERS NO — pills display/manage tags (exhibit-tww) but nothing filters the grid; README claims GET /api/artifacts?tags=&collections= but the store-layer filter is unverified.

Real remaining gaps if wanted: drag-drop upload, tag/collection filtering (verify API filter support first).
