---
id: Exh-hl64
status: closed
deps: []
links: []
created: 2026-10-27T20:09:04Z
type: bug
priority: 1
assignee: Max Omdal
tags: [perf, mobile]
---
# Embedded static assets are served without cache validators or lifetimes

internal/api/assets.go serves /assets/* with http.FileServer over embed.FS. Embedded files have a zero modtime, so responses carry no Last-Modified, no ETag and no Cache-Control: the browser re-downloads every stylesheet, script, htmx, Stimulus and the icon font on every launch. Fix: give assets validators and a cache lifetime, and measure repeat-visit transfer and timing before and after under mobile throttling.

