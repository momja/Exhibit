---
id: nw-g5pl
status: closed
deps: [nw-7t3k]
links: []
created: 2026-09-27T05:10:00Z
type: chore
priority: 3
---
# Split internal/api/gallery.go into one file per page
`gallery.go` holds the handlers and view models for every artifact page (index, new, detail, edit, 404) plus shared helpers, ~960 lines. Later pages already have their own files (`admin.go`, `profile.go`, `agentui.go`).

Move each page's handler, view model and render function into its own file, keeping shared page helpers (render URL minting, tag/widget/capability views) in one place. Pure move: no behaviour change.

Stacked on nw-7t3k.
