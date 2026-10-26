---
id: Exh-8vjo
status: closed
deps: []
links: []
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

**2026-10-26T02:42:45Z**

Recovered the entire preserved stack at 3dff8a1288a3377edb586ac81777f33c2af8dfc9 onto live origin/main 6ef98c94c42b23aa4ae40a6daba4e9e597086efa with a history-preserving merge. Reconciled server startup, worker Manager and docs while retaining current local/BYOK defaults, Clerk lifecycle, provider-error handling, thinking preferences and event-byte ownership. Shared extension bytes and Store/migrations match current main.

Validation passed: make assets; all Go packages with -count=1/-timeout=15m (serial batches completed after monolithic processes received SIGTERM, including all 648 API and 196 Store top-level tests); affected-package vet; complete execution/Sprites/authority/scope race suites plus Agent/Server/API race regressions; 46 History/import script tests and real Chromium UI tests; native arm64 Node 22.22.0/Pi 1.3.0 packaged-release checks; core Docker build and no-Node/Pi container save/History/resume/version smoke; default local/BYOK save/resume and clean-environment authentication smoke. Made canceled-startup cleanup deterministic, allowed cold Pi startup in its test, and waited for terminal checkpoint cleanup before comparing unchanged History. All original History assertions remain.

Live Sprite rollout, suspension/billing, timings and org limits remain unverified (no Sprite credential). Optional S3 fixture tests skipped because EXHIBIT_TEST_S3_ENDPOINT is unset. No deployment or real provider use. Original main stays clean/on main; unrelated ticket-only commit 78c3a40 is excluded.
