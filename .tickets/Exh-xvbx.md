---
id: Exh-xvbx
status: open
deps: []
links: [Exh-iust]
created: 2026-11-08T23:51:42Z
type: bug
priority: 2
assignee: Max Omdal
tags: [agent, test]
---
# Fix TestAConversationIsKeptWithTheVersionItLeftBehind after the prepared-source row was removed

internal/api/agent_record_test.go:105 asserts that a kept conversation's second message is {Role: tool, Text: "Prepared artifact source"}. Commit 80c48ae6 ("Resolve final council recovery and transcript copy findings", merged with the Exh-iust interface copy work, PR #274) removed the prepared_source case from project() in internal/agent/transcript.go, so agent.Messages no longer produces that row and the test fails on origin/main:

    agent_record_test.go:103: "3" is not greater than or equal to "4"
    [{user make the button purple} {tool Writing artifact} {assistant Done! ...}]

The test runs only where the pi binary is installed (newPiHarness), which is why it can pass in an environment without Pi. Found while verifying Exh-dabd (PR #275); it fails identically without that change.

## Design

Decide whether removing the row was intended (the transcript-copy finding suggests yes). If so, update the test to the projection that now exists: drop the msgs[1] assertion, expect Writing artifact at msgs[1], and lower the minimum length from 4 to 3. Keep the SessionFile assertion on customType exhibit_source_read, which still proves preparation ran and is persisted. If the row should come back, restore the case in project() and its transcript_test.go coverage instead.

## Acceptance Criteria

1. go test ./internal/api -run TestAConversationIsKeptWithTheVersionItLeftBehind passes with pi installed.
2. The test still asserts the prepared source read is persisted in the session file.
3. No other agent_record or transcript test regresses.

