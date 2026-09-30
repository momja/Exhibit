---
id: av-2yws
status: in_progress
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

Measurement comes before any of it — before pricing, before caps ([[av-99f4]]), before any usage-based pricing. Right now there is not even a number to be wrong about.

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

## Notes

**2026-09-29 — What Pi reports, and when (verified against the installed Pi
docs and the shipped TypeScript types, at the version this lands pinned to)**

The schema below is settled against these facts; re-verify if the pin moves.

1. **Per assistant message, streamed.** `message_update` events carry a
   `usage` block — `input`, `output`, `cacheRead`, `cacheWrite`,
   `totalTokens`, and a `cost` breakdown — that is the *latest cumulative*
   provider-reported usage for the current assistant response. It can stay
   zero until completion when a provider does not report usage while
   streaming. `message_end` carries the authoritative final `usage` on the
   message. This is the finest granularity the protocol has, and it is finer
   than a turn: usage is observable mid-response.
2. **Tool-reported usage** is summed into the session totals (`get_session_stats`
   counts "usage reported by tools"). Nested tool calls roll up into the
   calling tool's result `usage`. It may appear on the wire in a
   `tool_execution_end` result's `usage` field when a tool sets one; our own
   exhibit tools set none today.
3. **Compaction and branch summaries**: `compaction_end` carries
   `result.usage`; the `compact` RPC response reports the summary call's
   usage. Both count toward session totals.
4. **Some usage emits no event at all.** Pi's cache warmer appends `usage`
   entries straight to the session ledger; extensions cannot do the same
   (the extension `ctx.sessionManager` is read-only — `appendUsage` is not
   reachable from a hook), so a nested model call made inside a hook —
   av-gust's guardrail screen — shows up *nowhere* in session usage unless
   it is reported to us out of band. Guardrail spend is therefore metered by
   Go, not by Pi: `guard.ts` must forward the screen call's usage over the
   same exhibit callback channel it signals blocks on, and Go records it
   tagged `guardrail`. Until av-gust lands that signal, guardrail spend is
   unmetered — the known seam, kept open here rather than papered over.
5. **`get_session_stats` (RPC)** returns cumulative `tokens` and `cost` for
   the whole session — assistant messages + tool-reported usage + compaction
   — at any time while the process lives, plus current context-window
   usage. It is the authoritative total and the cross-check for everything
   above.
6. **When numbers arrive.** Per response, cumulative and possibly delayed to
   completion; per settle, session totals via RPC; nothing at all after the
   process dies. So: meter from the event stream as it arrives (rows land
   per assistant message, per tool result carrying usage, per compaction),
   flush whatever is in flight when the process exits — which is what
   covers aborts, idle-reap kills, and mid-turn deaths — and reconcile
   against `get_session_stats` at every settle so the ledger equals Pi's own
   total even for usage sources that emit no event.
7. **`cost` is Pi's price table, not a bill.** Pi costs a response "at the
   model's catalog price". Tokens are the meter and are recorded exactly;
   cost is kept beside them as `cost_micros` for what it is good for —
   spend-cap enforcement and rough attribution — and must never be shown to
   a user as an amount owed. A model Pi has no price for costs 0 in that
   column; the token columns still say what happened.

**Attribution.** Rows carry owner, session, provider, model, and who pays
(`platform` for instance-credential and guardrail spend, `user` for BYO-key
session spend). Provider and model stay on the row even though platform mode
strips them from every user-visible surface (av-siqf): that is a presentation
decision, and cost per token differs by model. The ledger is accounting — it
is not deleted with an account, because the money was spent either way.

**No enforcement here.** This ticket counts. [[av-99f4]] refuses.

