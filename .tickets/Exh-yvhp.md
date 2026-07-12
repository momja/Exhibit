---
id: Exh-yvhp
status: closed
deps: []
links: [av-q3wo, Exh-i0ll]
created: 2026-07-11T05:18:29Z
type: epic
priority: 1
assignee: Max Omdal
---
# Pi agent harness PoC: BYO-key agent chat that creates and modifies artifacts

Proof-of-concept integration of Pi (pi-mono, Mario Zechner's agent harness) as a sidecar spawned per chat session in RPC mode (pi --mode rpc, JSONL over stdio). Builds on av-q3wo. Scope: (1) BYO API key entered in browser, stored encrypted (AES-GCM, server secret) per owner_id; (2) agent chat UI that creates new artifacts through the normal ingest path via a pi extension tool calling the exhibit API; (3) modify-existing-artifact flow from the artifact detail page; (4) snippet mode: hotkey element-picker inside the sandboxed iframe that captures a screenshot (SVG foreignObject rasterization) + element descriptors and attaches them as multimodal context to the agent prompt.


## Notes

**2026-07-11T16:56:40Z**

PoC complete on branch feature/Exh-yvhp/pi-agent-poc (commit e051753, pushed). All five subtasks implemented and verified end-to-end in Chrome against localhost (ports 18642/18643, mock LLM on 18644): key entry->encryption->masked reads; agent create flow (streamed SSE, tool chips, live preview); modify flow bound to an artifact; snippet mode (hover-pick element in the sandboxed iframe, SVG-foreignObject screenshot + selector descriptor attached to the prompt as multimodal context) drove a button recolor; transcripts persisted per artifact; storage-shim state bridge works from the agent page. Deterministic testing uses cmd/mockllm + the exhibit-mock pi provider (enabled only when MOCK_LLM_URL is set); the real-provider path is identical plumbing (key via env var to the pi subprocess). Docs: docs/agent.md + README/architecture/tech-stack updates.
