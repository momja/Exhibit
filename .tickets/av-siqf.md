---
id: av-siqf
status: open
deps: []
links: [av-fafu]
created: 2026-08-18T05:37:03Z
type: feature
priority: 2
assignee: Max Omdal
tags: [hosted, agent, backend, security]
---
# Platform-level agent API key for hosted deployments

The agent is strictly BYO key, with no way for an instance to supply the credential itself. `agentSessionOpts` (`internal/api/agent.go:148`) reads the owner's `agent_keys` row, decrypts it with the server secret, and returns `412 no API key configured` when there is none. `internal/api/agent.go:76` is the only path that ever writes one, and it writes what a user typed.

That is correct for self-hosting and wrong for the hosted version. The PRD targets a general audience; asking someone to create an Anthropic account and paste an API key before they can use the headline feature is the step that loses them. The hosted version therefore supplies the key itself and does not offer BYOK at all.

This ticket is the deployment configuration for that: an instance that runs the agent on its own platform credential. BYOK remains the self-hosted path and is untouched when the new variables are unset.

## Design

**One resolution point.** `agentSessionOpts` is already the only place a key is decrypted, and both session creators — the chat surface and the widget generator ([[av-fafu]]) — go through it. The platform key resolves there and nowhere else, so the two surfaces cannot drift apart on which credential they run.

**One knob, not two.** `AGENT_API_KEY`, `AGENT_PROVIDER`, `AGENT_MODEL`. Setting `AGENT_API_KEY` puts the instance in *platform mode*: every session runs on it, the BYOK entry surface is not rendered, and `PUT`/`DELETE` of the per-owner key are refused.

The rejected alternative is a per-owner key taking precedence with the platform key as fallback. It reads like the flexible choice and is worse in both directions: on a hosted instance it silently mixes billing models, since an owner who pasted their own key stops being metered while the interface says nothing; and it leaves a key field on a surface whose whole point is that nobody needs one. Two modes with one variable choosing between them is a support question nobody has to ask. A future paid tier that lets a customer bring their own key to escape metering is a real product idea, but it is a *tier*, and it needs the metering to exist before "not metered" means anything — see below.

**The platform key is never readable back.** It lives in the process environment, is never written to `agent_keys`, and never reaches a response. `agentKeyStatus` currently returns a masked hint of the caller's own key, which is right for a secret they typed and wrong for one they did not: on a hosted instance that hint is the operator's credential, partially disclosed to every customer. In platform mode the status route reports provider and model, and no hint at all.

**Unset is exactly today.** Same shape as `OIDC_ISSUER` — absent means the feature does not exist, the config surface is unchanged, and every existing self-hosted instance keeps the behavior it has. `EXHIBIT_SECRET` and the `agent_keys` table stay as they are; platform mode simply does not consult them.

**`exhibit-mock` resolves through the same path**, so tests exercise platform mode without a real credential.

**What this deliberately does not include: metering, or a spend cap.** Enabling platform mode makes every agent session bill to the instance's provider account with nothing bounding it. Nothing in `internal/agent` reads token usage off Pi's event stream today — the events it recognizes are prompts, aborts, and the three synthetic `exhibit_*` save events — so an instance in platform mode can neither attribute spend to an owner nor stop a session that runs away. A runaway session is the operator's loss before any invoice exists, and usage billing meters after the fact, so billing does not close this either.

That is a deliberate scope line, not an oversight: the configuration is useful on its own for a controlled instance, and metering is a larger piece of work with its own design. But **platform mode must not meet public traffic before the cap exists.** Hence the startup warning in the acceptance criteria. Agent token metering per owner, and a per-owner budget checked before a session spawns and again between turns, are the follow-up this depends on for public use; neither is filed yet.

## Acceptance Criteria

- With `AGENT_API_KEY` unset, behavior is identical to today: BYOK entry works, an owner with no key gets `412`, and no new variable appears in any rendered page.
- With it set, an agent session starts for an owner who has no `agent_keys` row.
- In platform mode the key value never appears in an API response, a log line, or a rendered page. The key status route reports provider and model only, with no masked hint.
- In platform mode the BYOK entry surface is not rendered, and `PUT`/`DELETE` of the per-owner agent key are refused.
- `AGENT_API_KEY` set without `AGENT_PROVIDER`, or with a provider `agent.KnownProvider` rejects, fails at startup rather than at the first session — the same posture as a malformed `LOGIN_PASSWORD_HASH`.
- An existing `agent_keys` row is neither read nor deleted when platform mode is on; turning the variable off restores BYOK with that key intact.
- Startup logs that platform mode is enabled and that agent sessions bill the instance's provider account, naming the absent spend cap.
- `exhibit-mock` works as the platform provider, so the path is testable with no real credential.
- `docs/deployment.md` documents the three variables, and states that platform mode without a spend cap should not be exposed to untrusted signups.

