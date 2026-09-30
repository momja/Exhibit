---
id: av-gust
status: closed
deps: []
links: [av-99f4]
created: 2026-08-09T16:18:19Z
type: bug
priority: 1
assignee: Max Omdal
tags: [hosted, agent, security]
---
# agent has no guardrails

On a platform-mode instance the agent spends the operator's provider account, and nothing stops a user from getting it to generate content that breaks the provider's usage policy. A provider that sees repeated violations can suspend the account, which takes the agent away from every user at once.

Found by pasting a system prompt from a roleplaying site: the agent took on the role immediately. The system prompt's "do not engage with off-topic queries" line does not hold against a prompt written to override it, and it only exists in modify sessions anyway (`internal/agent/prompt.go` `modePrompt`); create and widget-only sessions get nothing.

## Scope decision (2026-09-29)

**Off-topic use is allowed.** Users pay for the service and can use the agent as they like; cost is bounded by the spend cap ([[av-99f4]]), not by a topic rule. This ticket enforces a **usage policy** (prohibited content), not topicality.

**This lands the scaffold, not every layer.** One screen, on user input, configured by the operator. Screening saves and replies, strikes, and dedicated decision-model vendors (Jev, OpenAI Decisions API) can reuse the same seam later if traffic shows they are needed.

## Design

- **Operator-configured guardrail model.** `GUARDRAIL_PROVIDER`, `GUARDRAIL_MODEL`, `GUARDRAIL_API_KEY`, mirroring `AGENT_*`. All unset: no guardrail, today's behavior. Partly set: fatal at startup, the posture `LOGIN_USERNAME` without `LOGIN_PASSWORD_HASH` already takes.
- **A separate Pi extension, `internal/agent/ext/guard.ts`,** loaded beside `exhibit.ts` with its own `-e`, embedded and materialized the same way. It imports nothing from `exhibit.ts` so it could move to its own package later.
- **Its own Pi provider.** The extension registers the guardrail model under its own provider name with its own key, so it never collides with the agent's credential even on the same vendor. A future decision-model vendor is a custom `streamSimple` provider in the same file.
- **Input screen.** Pi's `input` hook screens the user's own words (not the fenced artifact source) before the model runs. On a block it returns `handled`, so the main model never sees the prompt, and the extension signals Go, which shows a fixed canned reply defined in Go.
- **Fail closed.** With a guardrail configured, a screen that errors or times out blocks the turn.
- **Pin Pi** in the Dockerfile, since the gate depends on the `input` hook's `handled` semantics.

## Acceptance Criteria

- With no `GUARDRAIL_*` set, agent behavior is identical to today.
- A partial `GUARDRAIL_*` configuration fails at startup naming the missing variable.
- With a guardrail configured, a prompt the screen blocks never reaches the agent model and the chat shows the canned reply.
- An allowed prompt runs exactly as before.
- A screen failure blocks the turn with the canned reply.
- Tests drive allow, block, and failure through the mock provider.
- Pi is pinned in the Dockerfile.
- `docs/agent.md` and `docs/security.md` §5 describe the guardrail and what it does not cover.
