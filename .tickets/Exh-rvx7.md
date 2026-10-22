---
id: Exh-rvx7
status: in_progress
deps: [Exh-spg4]
links: []
created: 2026-10-22T06:50:52Z
type: feature
priority: 4
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, widgets]
---
# [Experimental] Phase 6: declarative scheduled fetches

EXPERIMENTAL; part of Exh-g2vv.

The relay's main gap versus a backend is work while no page is open: digests, price alerts, fresh widgets. First answer: an artifact declares 'every N hours (random minute), fetch this allowlisted URL, using this connection, into this localStorage key'. The relay runs it under the same gate, grants and quotas; the widget reads the key. No artifact code runs on the server.

- Respect provider guidance, e.g. SimpleFIN's daily fetch at a random minute with 5-day overlapping windows; transforming windows may need recipe help.
- If declarative isn't enough, the next option is running the artifact itself on a schedule in the existing headless Chromium sidecar (CHROMIUM_CDP_URL, with request confinement as in security.md §1.10), not reviving a QuickJS backend.

## Acceptance Criteria

- A widget shows data refreshed on schedule with no page open.
- Schedules count against quotas and entitlements.

