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

Show one useful provider error in chat when a final assistant message fails; hosted mode uses generic failures in chat and the event stream with no API key or provider details, while BYOK mode can explain key recovery; preserve the full Pi-reported provider error in server logs with session correlation; keep cancellation and retries correct; add focused tests; verify with an actual provider failure.


## Notes

**2026-10-25T01:03:13Z**

Reproduced the deployed artifact-edit failure: agent_start and exhibit_source_prepared are followed by an assistant error (OpenRouter 401 User not found), agent_end with willRetry=false, and agent_settled. The chat previously displayed only the preparation notice. The user confirmed the API key had expired.

Completed the interrupted provider-error fix. Final failed runs now display one error; rejected/expired/invalid keys explain how BYOK users can update their key, while platform users are directed to the instance owner. Other failures retain provider details. Automatic retries stay quiet until exhausted, successful retries stay clear, and cancellation does not appear as a provider failure. Settlement still controls the composer.

Validation: make assets; go build ./cmd/server; go test ./internal/api -run '^TestGalleryPageScriptSuite$' -count=1; git diff --check. Three provider-error regressions failed against the original implementation. A live browser using the real Pi 1.4.0 sidecar and a local HTTP 401 provider displayed the actionable message exactly once after preparation and settled normally. Screenshot: docs/screenshots/Exh-5z7b/provider-key-error.png. Production artifact/widget revisions were unchanged by the diagnostic. Production has not been redeployed.

**2026-10-25T01:29:54Z**

Hosted-mode follow-up: every final provider failure now displays only "The agent could not complete your request. Please try again." in chat. No provider error detail or API-key explanation is shown in hosted failure messages; BYOK retains actionable key recovery and provider diagnostics. Updated acceptance criteria and documentation.

Validation: full gallery page-script suite passed, server rebuilt, and a real-Pi hosted session against the local HTTP 401 provider showed exactly one generic error with no key control. Added regression cases for authentication, expired keys, arbitrary provider text containing credential details, and missing detail. Saved hosted-generic-error.png alongside the existing BYOK screenshot. Production remains undeployed.

**2026-10-25T14:34:32Z**

Observability and hosted privacy follow-up: provider errors previously had no dedicated normal server log and raw errorMessage fields still reached browser events. Each failed assistant response now logs its complete Pi-reported error once at ERROR with session_id, provider and model, before hosted redaction. Logs contain the error rather than the conversation payload. Hosted message, retry, compaction and extension error fields are replaced with generic text before entering the event backlog or stream; BYOK diagnostics and private Pi session data remain intact.

Validation: focused internal/agent and internal/api suites passed, including redaction, complete error logging, cancellation, usage metering, real-Pi prepared-source lifecycle, platform API/stream behavior and the gallery suite; server rebuilt; git diff --check passed. A live real-Pi hosted HTTP 401 failure produced exactly one normal ERROR log with the complete provider error. Its browser event stream contained none of the provider failure detail, while the private SQLite transcript retained the original message. No production deployment.
