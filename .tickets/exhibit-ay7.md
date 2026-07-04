---
id: exhibit-ay7
status: closed
deps: []
links: []
created: 2026-07-02T06:45:56Z
type: feature
priority: 2
assignee: Max Omdal
parent: exhibit-3yo
---
# Render artifact body with CodeMirror on /artifacts/<id>/edit

Replace the plain textarea for the artifact 'body' field on the /artifacts/<id>/edit page with a CodeMirror 6 editor, per tech-stack §5. CodeMirror renders the source (syntax-highlighted, editable HTML/CSS/JS) while the sandboxed iframe renders the running tool — keep the two roles separate. Bundle CodeMirror as an esbuild-built island (go:embed served asset, no Node in production) and mount it over the body field, submitting the editor contents back through the existing edit form / single-write-path API. Pull in @codemirror/lang-html (plus lang-javascript/lang-css as needed), @codemirror/commands, @codemirror/search, and a theme.

## Design

Approach: CodeMirror 6 as a client-side island mounted on the existing body <textarea>, syncing content back to the form on change/submit so the API write path is unchanged. Modules: codemirror meta-package or @codemirror/state + @codemirror/view; @codemirror/lang-html, @codemirror/lang-javascript, @codemirror/lang-css; @codemirror/commands, @codemirror/search; a theme (e.g. @codemirror/theme-one-dark). Bundle with esbuild into an embedded static asset served by the Go binary. Related: exhibit-2hl (surface newly-detected origins after a body edit) consumes edited body content.


