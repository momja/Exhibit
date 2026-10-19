---
id: Exh-4h60
status: in_progress
deps: []
links: []
created: 2026-10-19T19:40:27Z
type: task
priority: 2
assignee: Max Omdal
parent: av-xkq4
tags: [design, agent, demo]
---
# Design demos: seamless modify mode — glow/inset entrance, direct annotation (select, box, marker)

Interactive HTML demos exploring how to cut the context switch between using and editing an artifact. Entering modify mode insets the live artifact with a glowing, sparkling border (several effect options). In modify mode the user annotates the artifact directly instead of describing it in chat: element select, BOWSER_SNAPS-style box, and a freehand marker whose strokes resolve to bounding boxes and intersected elements. Notes ride along with a compact composer rather than a chat sidebar.

## Design

Standalone demo under docs/demos/modify-mode/. The mock artifact runs in a sandboxed srcdoc iframe and answers hit-test queries over postMessage, mirroring the real opaque-origin constraint (render/snippet.go, av-c23y protocol).

## Acceptance Criteria

A self-contained demo page with at least three switchable entrance effects; select/box/marker tools producing numbered annotations with notes; a preview of the payload the agent would receive; screenshots in docs/screenshots/<id>/.

