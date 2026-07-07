---
id: av-ijew
status: closed
deps: []
links: []
created: 2026-07-06T23:09:48Z
type: chore
priority: 2
assignee: Max Omdal
---
# Sync /docs with shipped closed-ticket features

Doc audit against closed tickets found drift: (1) stale artifact-viewer names/references after Exhibit rename 034ced0 (architecture.md:3-4, technical_stack.md:3+283, README:147/153, stale screenshot); (2) PRD 4.4 schema missing artifacts.source_url + tags.color/uniqueness (migrations 002-004); (3) URL-paste ingest, POST /api/artifacts/:id/refetch, DELETE /api/artifacts/:id undocumented (exhibit-6hh/mie/uek/63q); (4) gallery documented as templ+htmx+Alpine+Tailwind but implemented as hand-rolled server-rendered Go HTML; (5) FTS5 documented as source+title+tags but indexes title only; (6) iframe clipboard Permissions-Policy delegation (av-ys8g) undocumented.


## Notes

**2026-07-06T23:38:52Z**

Doc sync applied on branch feature/av-ijew/docs-sync (commit c9c3bea, Supacode worktree). 7 files: architecture.md, technical_stack.md, product_requirement_doc.md, build_assets.md, README.md, regenerated exhibit_screenshot.png, removed dead web/templates/gallery.templ. FTS coverage gap split out as av-b6o9. Not pushed/merged — awaiting review.

**2026-07-07T01:09:11Z**

PR opened: https://github.com/momja/Exhibit/pull/28

**2026-07-07T01:59:41Z**

PR #28 merged (9bb6488). Review comments addressed in 8a9adee: future/ticket refs removed from docs, docs/security.md added.
