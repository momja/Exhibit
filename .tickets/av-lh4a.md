---
id: av-lh4a
status: closed
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


## Notes

**2026-11-16T17:48:36Z**

Widget saves now select and swap only the server-rendered tile, preserving the artifact iframe; artifact/state saves still replace the pane. Chromium regression covers first/repeated widget saves, an initially unbound preview, independent stamps, and an open version viewer. Verified rebuilt assets, go vet ./..., focused AgentPreview|Widget tests and node --check.

**2026-11-16T18:10:29Z**

Independent Pi review requested changes on 4335ea2e: fixed synthetic htmx event target in gallery test; historical-viewer test now counts both refresh event types after SSE acknowledgement; widget requests use this:replace independently of pane requests, with a delayed-response overlapping-save regression. Focused AgentPreview|Widget|GalleryPageScriptSuite tests (including Chromium), rebuilt assets, go vet and JS syntax checks passed. Fresh review required before merge.

**2026-11-16T18:38:10Z**

Second Pi review found the independent widget request queue could lose the tile when a pre-widget pane response arrived late. Replaced tile-owned requests with a persistent sibling htmx source, a shared queue-all ordering for both refresh types, and out-of-band selection of the current widget element. Added TestAgentPreviewStalePaneBeforeWidgetSaveBrowser from the reviewer reproducer, using SSE/DOM acknowledgements rather than sleeps. Earlier independent-queue rationale is superseded; fresh review remains required.
