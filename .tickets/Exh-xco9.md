---
id: Exh-xco9
parent: Exh-099b
status: closed
deps: []
links: []
created: 2026-10-19T05:09:36Z
type: chore
priority: 2
assignee: Max Omdal
tags: [agent]
---
# Upgrade Pi to the latest stable release

Upgrade the pinned Pi sidecar from 0.99.1 to the latest stable release (verified npm latest: 1.3.0). Check release changes and maintain compatibility with Exhibit RPC, extensions, guardrail input handling, settlement, usage metering, and saved/resumed conversations. This is a prerequisite to the agent startup/source-preload work discussed in this thread; do not add model benchmarking or thinking summaries.

## Acceptance Criteria

Docker pins the verified latest stable Pi release; required Node runtime is compatible; real-Pi API pipeline, guardrail, conversation and resume tests pass; any compatibility changes and relevant documentation are included.


## Notes

**2026-10-19T05:23:50Z**

Upgraded Docker PI_VERSION from 0.99.1 to npm latest 1.3.0 (verified 2026-10-19); engine-strict enforces Node >=22.19.0. Runtime smoke passed in node:22-bookworm-slim (Node 22.23.3): exact Pi install and pi --version. Found and fixed Pi 1.0 startup incompatibility: --provider now requires --model; omit both for an empty model to preserve prior automatic authenticated selection. Added real-Pi BYOK/platform fresh+resume coverage, an unmodified 0.99.1 session fixture and cross-version resume test, guardrail usage assertions, and metering across turns/resume. Checks: assets build; full pre-fix baseline internal/api+internal/agent passed (only 3 unrelated bucket tests skipped); final internal/agent passed; final targeted API compatibility run passed all 16 cases without skips. Final broader API rerun stopped in favor of finite targeted checks after its relevant conversation cases passed. No RPC/tool/transcript adapter changes. Preload code must preserve conditional provider/model flags; any future before_agent_start handler reading prompt must check messageType=user. Full application Docker image was not built. No push or PR.
