---
id: epi-d3b6
status: open
deps: []
links: []
created: 2026-07-05T19:14:41Z
type: chore
priority: 2
assignee: Max Omdal
tags: [build, deploy, tech-debt]
---
# Build vendored frontend assets (Phosphor icons, CodeMirror) at deploy time instead of committing them

internal/api/assets/phosphor/{LICENSE,Phosphor.woff2,regular.css} and internal/api/assets/editor.js are npm-fetched/bundled (web/icons, web/editor) then committed to git so go:embed has something to serve and go build needs no Node. Raised in PR #18 review (https://github.com/momja/Exhibit/pull/18#discussion_r3525485054): don't like committing built/fetched output — make asset-fetching part of the build/deploy process instead, not tracked in git. Still requires no internet egress or runtime dependencies for the running server; the Node step just moves to build time on every deploy, which is an accepted tradeoff.

## Acceptance Criteria

Add a Node build stage to the Dockerfile (docs/technical_stack.md §12 already describes this: 'build with the Go toolchain, and Node only to bundle CodeMirror/gallery JS') that runs 'make assets' (or the npm installs directly) into internal/api/assets/* before the Go build stage copies/embeds them. Remove internal/api/assets/editor.js and internal/api/assets/phosphor/{LICENSE,Phosphor.woff2,regular.css} from git tracking and add them to .gitignore. Update the Makefile/.gitignore comments and docs/technical_stack.md (which currently say 'the bundled output is committed') to describe the new build-time-fetch behavior. Make 'make build' depend on 'make assets' so a local dev build still works in one command. Confirm 'docker build' produces a working image with icons/editor intact, and that a bare 'go build' without 'make assets' first fails clearly (missing embed) rather than silently serving stale assets.

