---
id: av-hyo6
status: open
deps: []
links: []
created: 2026-08-18T05:56:00Z
type: epic
priority: 2
assignee: Max Omdal
tags: [hosted, agent, backend]
---
# Epic: User metrics — per-owner usage measurement and limits

Nothing in Exhibit measures what an owner consumes. Storage has no number ([[av-1in5]] covers that side), and the agent has none either: `internal/agent/agent.go` recognizes prompts, aborts and the three synthetic `exhibit_*` save events, and reads no token usage off Pi's stream at all. That was correct while the agent was strictly BYO key — the user's tokens were billed by their own provider, so there was nothing for Exhibit to count.

[[av-siqf]] changes the premise. Once an instance can supply the credential, every session bills to *its* provider account, and an instance that cannot attribute that spend to an owner cannot price it, cannot cap it, and cannot tell a runaway session from a heavy user.

This epic is the measurement and the limit. It is separate from [[av-1in5]] because it answers a different question — what someone *used*, a flow over time, rather than what they *hold*, a level — and because the two have different failure modes: an over-quota library refuses the next upload, while an uncapped agent session spends real money before anyone can react.

**Metering is not billing, and a cap is not metering.** Polar's usage billing invoices after the fact, which is exactly what does not help here: the loss has already happened by the time a meter reading becomes an invoice line. The cap has to be enforced ahead of the spend, in-process, and it must hold whether or not any billing provider is reachable.

