---
id: Exh-ys5q
status: in_progress
deps: [Exh-mg8f]
links: []
created: 2026-10-22T06:50:51Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, agent]
---
# [Experimental] Agent: tell agents cross-origin fetch works; flag public CORS proxies

EXPERIMENTAL; part of Exh-g2vv; lands with phase 1.

- **Prompt:** replace 'prefer zero external references' at internal/agent/prompt.go:22 with the platform fact: fetch any API directly, and Exhibit relays it whether or not the API sends CORS headers; never use a public CORS proxy or mode: 'no-cors'; don't build a backend for this; each origin needs the user's approval (the tool result lists what will be asked). Same form as the existing localStorage fact. Not a trick: models are trained on frontends that avoid non-CORS APIs, and they otherwise reach for corsproxy.io/allorigins, backends, or no-cors.
- **Footprint warnings:** the ingest scan flags known public CORS-proxy hosts ('sends your requests through a third party; Exhibit doesn't need it') and mode: 'no-cors' (an opaque, unreadable response; it is deliberately not routed through the relay). Warnings appear in the approval UI and in the agent's create/write/edit tool results so it fixes them in the same turn.
- **Portability note:** single-file HTML exports list the origins that need a relay outside Exhibit.

## Acceptance Criteria

- An agent asked for an arXiv app writes a direct fetch with no proxy and no no-cors (spot-check with mockllm fixtures plus one live run).
- A body containing corsproxy.io or no-cors produces the warning in the footprint and the tool result.

