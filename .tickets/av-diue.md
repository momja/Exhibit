---
id: av-diue
status: in_progress
deps: []
links: []
created: 2026-09-28T06:05:45Z
type: task
priority: 2
assignee: Max Omdal
tags: [perf, fonts]
---
# Ship Phosphor subset + font-display swap (from av-xfld spike)

Promote spike experiment B to production: subset the vendored Phosphor regular weight to used classes and rewrite font-display block->swap in web/icons/build.mjs. Measured: woff2 147KB->5.8KB, css 78KB->4.9KB raw. Keep TestPhosphorIconAssetsServed green; add a template-coverage guard test.

