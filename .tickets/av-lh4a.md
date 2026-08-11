---
id: av-lh4a
status: open
deps: []
links: [av-ghvs]
created: 2026-08-12T02:51:46Z
type: bug
priority: 2
assignee: Max Omdal
tags: [agent, htmx, performance]
---
# Widget save re-downloads the entire artifact document

`internal/api/agentui.go` mints one cache-busting stamp per render and applies it to both frames — `FrameURL` and `widget.URL`. `exhibit_widget_saved` (web/gallery/agent.js) then calls the same `refreshPreview()` the artifact save uses, so the artifact iframe gets a fresh stamp and fully reloads even though its body did not change. The handler's own comment says so: 'The artifact body is unchanged.'

Cost is proportional to artifact size and is paid on every widget save. With a vendored wasm artifact (av-ghvs) that is ~16 MB re-transferred, uncompressed, to refresh a tile.

Note `exhibit_state_changed` is correctly implemented and must keep reloading the artifact frame: state is inlined into the document at render, so the frame genuinely is stale.

## Acceptance Criteria

- The artifact frame and the widget frame carry independent cache-busting stamps.
- A widget save refreshes only the tile; the artifact frame's src is unchanged and it does not reload.
- An artifact save and a state change both still reload the artifact frame.
- A test covers the two stamps moving independently.

