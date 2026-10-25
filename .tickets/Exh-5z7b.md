---
id: Exh-5z7b
status: closed
deps: []
links: []
created: 2026-10-25T00:33:30Z
type: bug
priority: 1
assignee: Max Omdal
tags: [agent, ui]
---
# Show provider authentication failures in agent chat

Provider failures are delivered in assistant message_end events but agent.js ignores them. The deployed agent silently shows no reply when OpenRouter returns 401 User not found or OpenCode reports an expired key, so users suspect that saving keys or the backend is broken.

## Acceptance Criteria

Show one useful provider error in chat when a final assistant message fails; keep cancellation and retries correct; add focused browser-module tests; verify with an actual provider failure.


## Notes

**2026-10-25T01:03:13Z**

Reproduced the deployed artifact-edit failure: agent_start and exhibit_source_prepared are followed by an assistant error (OpenRouter 401 User not found), agent_end with willRetry=false, and agent_settled. The chat previously displayed only the preparation notice. The user confirmed the API key had expired.

Completed the interrupted provider-error fix. Final failed runs now display one error; rejected/expired/invalid keys explain how BYOK users can update their key, while platform users are directed to the instance owner. Other failures retain provider details. Automatic retries stay quiet until exhausted, successful retries stay clear, and cancellation does not appear as a provider failure. Settlement still controls the composer.

Validation: make assets; go build ./cmd/server; go test ./internal/api -run '^TestGalleryPageScriptSuite$' -count=1; git diff --check. Three provider-error regressions failed against the original implementation. A live browser using the real Pi 1.4.0 sidecar and a local HTTP 401 provider displayed the actionable message exactly once after preparation and settled normally. Screenshot: docs/screenshots/Exh-5z7b/provider-key-error.png. Production artifact/widget revisions were unchanged by the diagnostic. Production has not been redeployed.
