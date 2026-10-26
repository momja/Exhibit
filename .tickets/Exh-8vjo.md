---
id: Exh-8vjo
status: closed
deps: []
links: [Exh-ituu, Exh-i0ll]
created: 2026-10-26T01:52:28Z
type: bug
priority: 1
assignee: Max Omdal
tags: [agent, workers, integration]
---
# Recover standalone-worker stack onto main

PRs #189-#192 were marked merged, but the stack stopped on feature branches. #192 merged into worker-runtime at 00:29:06 UTC on 2026-10-21; #189 foundation merged to main at 00:29:40; #190 Sprites merged into foundation at 00:29:50; #191 runtime merged into Sprites at 00:30:01. Only #189 is an ancestor of main. The complete implementation survives at 3dff8a1288a3377edb586ac81777f33c2af8dfc9 on feature/Exh-fuxp/sprites-backend (tree identical to #192 merge 13d743d1a666ed34d038652eb191d51bfd286f2c). Recover the full tested worker stack onto current main through a new integration PR, preserving subsequent fixes.

## Acceptance Criteria

Restore the complete worker runtime, Sprites backend, server execution routing, extension materialization, and associated tests/docs from the preserved stack. Resolve integration conflicts while preserving current main behavior, including default self-hosted operation, hosted authentication, thinking preferences, provider-error handling, and event byte ownership. Validate the recovered stack with appropriate full Go, race, browser, packaging and build checks; report unavailable external validations. Open a PR targeting main. Never merge or push main.


## Notes

**2026-10-26T06:00:37Z**

Closed: de-scoped 2026-10-26. There is no scaling problem yet, so execution stays local. The authority layer that reached main through #189 is removed by Exh-ituu; recovery PR #215 is closed unmerged. The complete tested stack (#189-#192) survives at 3dff8a1288a3377edb586ac81777f33c2af8dfc9. If scaling becomes real, first measure per-session load on the local path and evaluate a single multi-conversation harness (Pi Durable) before reviving remote workers.
