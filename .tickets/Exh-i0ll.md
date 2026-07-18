---
id: Exh-i0ll
status: open
deps: []
links: [Exh-yvhp]
created: 2026-07-12T03:30:17Z
type: epic
priority: 2
assignee: Max Omdal
---
# Agent as a standalone service: own repository, API-only integration with Exhibit

Repurposed from the plugin-ecosystem design (scope creep; former subtasks Exh-mety/Exh-bujj/Exh-dicw/Exh-7o9d closed). Goal: the agent surface built in Exh-yvhp moves to its own repository (exhibit-agent) as a separate Go service that integrates with Exhibit exclusively through the HTTP API — no store access, no compiled-in handlers or UI. Exhibit core keeps only what is genuinely core: the transcripts table + endpoints (artifact provenance), the snippet picker in the render surface, and a configured link-out to the agent UI. Plan in docs/agent.md §Extraction (branch feature/Exh-yvhp/pi-agent-poc). Order: Exh-v6v4 (transcripts through the API) → new Exhibit-side seams ticket → Exh-k75k (extraction + core removal).


## Complexity

XL
