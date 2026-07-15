---
id: epi-q0u2
status: in_progress
deps: []
links: []
created: 2026-07-05T18:06:34Z
type: chore
priority: 2
assignee: Max Omdal
tags: [refactor, gallery, tech-debt]
---
# Extract gallery.go's inline HTML/CSS/JS into template files

internal/api/gallery.go builds every page (gallery index, artifact detail, edit view, and now the tag modals) as Go string concatenation — HTML, CSS, and inline <script> JS all interleaved in one ~900-line file. It's unreadable and every new UI feature (tags, modals) makes it worse. Raised in PR #18 review (https://github.com/momja/Exhibit/pull/18#discussion_r3525323237).

## Acceptance Criteria

Research spike first: decide the Go templating approach (there is an existing but unused templ scaffold at web/templates/gallery.templ from the v1 scaffold — decide whether to resurrect templ, or use html/template, or something else) before touching gallery.go. Then split gallery.go's HTML into template files, its CSS into a static stylesheet, and its inline <script> blocks into static JS served as assets — matching the docs/technical_stack.md §9 pattern already used for CodeMirror/editor.js. No behavior change; existing gallery_test.go assertions should keep passing.


## Notes

**2026-07-15T04:45:51Z**

Research spike decided stdlib html/template (templ scaffold was already deleted; stdlib = no codegen, no deps, contextual auto-escaping). Implemented on chore/epi-q0u2/extract-gallery-templates — PR https://github.com/momja/Exhibit/pull/49. Templates in internal/api/templates/, CSS/JS in the new web/gallery asset workspace served under /assets/gallery/. Page-content test assertions for CSS/JS follow the code into the static assets.
