---
id: av-6m3e
status: open
deps: [av-4oa1]
links: []
created: 2026-07-30T16:15:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, frontend, htmx]
---
# Agent preview: htmx partial re-render of the iframe on artifact save

The agent chat page rebuilds its preview pane in JS: showArtifact() in web/gallery/agent.js hand-sets the title, the Open/Details links, the empty-state visibility, the snippet button's disabled state, and a cache-busted iframe src, duplicating markup that agent.tmpl already defines. Replace that with an htmx partial re-render: after the agent's update_artifact (and create_artifact) tool call lands, the server hook that already fires — Session.noteArtifactSaved in internal/agent/agent.go:462, which broadcasts the synthetic exhibit_artifact_saved SSE event — becomes the htmx trigger, and the page swaps in a server-rendered preview fragment instead of mutating the DOM by hand. This is the agent-surface consumer of the partial re-render mechanism tracked by av-4oa1.

## Design

Mechanism: htmx + its SSE extension, self-hosted on the app origin (no CDN — same rule as Phosphor, technical_stack.md §9), vendored into internal/api/assets/ by scripts/build-assets.sh. Decide with av-4oa1 whether htmx's SSE extension drives this directly (sse-swap on the exhibit_artifact_saved event) or the existing EventSource in agent.js calls htmx.ajax() on that event — the second keeps one SSE connection and the existing event switch, which already does more than swap (chat messages, tool chips, footprint notes).

Fragment: extract the preview pane's markup from agent.tmpl into a partial (preview-bar title, Open/Details links, snippet button, empty state, iframe with its cache-busting ?r= stamp) rendered both by the full agent page and by a new app-origin fragment handler (e.g. GET /partials/agent-preview?artifact=<id>), so there is exactly one definition. Route sits with the other page routes in internal/api/api.go (unauthenticated, like /agent itself); no API-shape change.

Sharp edges to solve, not discover:
- agent.js caches frameEl once at load and the state bridge + snippet postMessage handlers compare e.source against frameEl.contentWindow. Swapping the iframe node out invalidates that reference. Either re-resolve the frame by id on each use, or scope the swap so the iframe element survives (hx-preserve / swap only the bar) and keep the src stamp as the reload trigger.
- Snippet mode must not survive a swap silently: if the frame reloads mid-snippet, reset snippetMode and the button's active class.
- The nudge (nudgePreview) and pane-switch state are page state, not artifact state — they stay in JS unless the fragment carries them explicitly.
- The render doc is Cache-Control: no-store, but the iframe still needs a changed src to refetch; the server-rendered fragment must emit a fresh stamp each time.

## Acceptance Criteria

After the agent calls update_artifact, the preview pane updates from a server-rendered fragment — no hand-built DOM in showArtifact() for the parts the template owns — and the iframe shows the new body without a full page reload. create_artifact takes the same path (first save populates the pane from the fragment). The storage-shim state bridge and snippet mode still work after a swap (verified: set localStorage in the preview, save again, state still round-trips; snippet an element after a swap). htmx is served from the app origin with no CDN reference. Docs updated: architecture.md §3.5/§3.7 and technical_stack.md §9 describe the mechanism.

