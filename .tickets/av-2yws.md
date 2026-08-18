---
id: av-2yws
status: open
deps: []
links: []
created: 2026-08-18T05:58:04Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-hyo6
tags: [hosted, agent, backend]
---
# Agent token metering: record per-owner usage from the Pi session

`internal/agent` records no usage. `readLoop` broadcasts every line Pi emits to SSE subscribers verbatim (`internal/agent/agent.go:565`) and persists the message list into `agent_transcripts`, but nothing extracts token counts, and no table holds them. So an instance running [[av-siqf]]'s platform credential pays a provider bill it cannot attribute to anyone.

Measurement comes before any of it — before pricing, before caps ([[av-99f4]]), before Polar's metered billing. Right now there is not even a number to be wrong about.

## Design

**Establish what Pi actually reports before designing the schema.** Its RPC protocol is the source, and this is a genuine unknown rather than a detail: whether usage arrives per assistant message, per turn, or only at session end determines whether a mid-turn cap ([[av-99f4]]) is even possible. If Pi does not report usage at all, that is the finding, and the fallback — counting from the provider's own API, or estimating from message content — is materially worse and should be chosen deliberately rather than discovered late.

**Record per owner and per session, at the finest granularity available.** Per-session rows aggregate to per-owner totals; the reverse is not recoverable. Keep provider and model on the row even though the user never sees them ([[av-siqf]] hides them from the UI, which is a presentation decision, not an accounting one) — cost per token differs by model, so a total without it cannot become money.

**Tokens are the meter; they are probably not the unit sold.** Input and output price differently, models differ by an order of magnitude, and a consumer facing a bill that varies with a model choice they were never shown has been handed the vendor's problem. Record tokens exactly here and leave the mapping to credits or included messages to pricing, which can change without a migration.

**Attribution survives the session.** The owner is on the session already (`Manager.Get(ownerID, id)`); usage must be attributed even when a session is aborted, times out, or the process dies mid-turn — those are exactly the sessions that spent money and produced nothing.

**BYO key sessions are metered too, and flagged as such.** A self-hosted instance gets a usage number it currently lacks, and the hosted version can tell instance-paid tokens from user-paid ones — which is the distinction any future bring-your-own-key tier is priced on.

**No enforcement here.** This ticket counts. [[av-99f4]] refuses.

## Acceptance Criteria

- What Pi reports, and when, is documented in the ticket before the schema is settled.
- Token usage is recorded per session with the owner, provider and model, and aggregates to a per-owner total over a period.
- Input and output tokens are recorded separately.
- Usage is attributed for sessions that are aborted, idle-timed-out, or killed mid-turn.
- BYO-key sessions are recorded and distinguishable from platform-key sessions.
- No request is refused as a result of this ticket.
- `docs/agent.md` records what is measured and what it is not (it is not a bill).

