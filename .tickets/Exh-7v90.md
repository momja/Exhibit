---
id: Exh-7v90
status: open
deps: []
links: []
created: 2026-07-23T06:29:05Z
type: task
priority: 2
assignee: Max Omdal
---
# Strip out MockLLM which was only used for agent verification, and is not used for testing


## Notes

**2026-08-04T04:34:23Z**

PREMISE NO LONGER HOLDS — do not action as written. As of av-e0yj and av-hrtv (both closed on their branches, 2026-08-03), mockllm is extracted to internal/mockllm and is the backbone of the agent test suites: internal/api/agent_pipeline_test.go (hostile-title prompt-injection test) and internal/api/agent_update_test.go both mount its Handler() in-process. Stripping it would delete the only coverage of the scope enforcement and the saved-event path. Re-scope to 'cmd/mockllm the binary is unused' if that is still true after those branches land, or close.
