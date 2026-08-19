---
id: av-siqf
status: closed
deps: []
links: [av-fafu, av-99f4]
created: 2026-08-18T05:37:03Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-1in5
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

**Platform mode reports nothing — not the key, not the provider, not the model.** The credential lives in the process environment, is never written to `agent_keys`, and never reaches a response; `agentKeyStatus`'s masked hint is right for a secret the caller typed and wrong for one they did not, since on a hosted instance that hint is the operator's credential partially disclosed to every customer.

But the provider and model go with it. Someone who wants to build a tool with AI does not need to know which model is behind it, and surfacing it invents a decision they cannot act on and a vocabulary they did not ask to learn — the answer for anyone who *does* want that control is to self-host, where BYOK already gives it to them in full. So the key resource does not exist in platform mode (`404`), and the agent page renders no key button, no key modal, no provider `<select>` and no model input — `agent.tmpl:27` and `agent.tmpl:62-83` are absent rather than disabled. `AGENT_MODEL` is the operator's choice and is invisible to the user. Availability is a separate signal that already exists and is unaffected: a missing `pi` binary still 503s from `ro.cfg.Agent == nil`.

**The abstraction is bigger than the config surface, and this is the part to scope carefully.** `internal/agent/agent.go:565` broadcasts every line Pi emits to SSE subscribers verbatim, and `agent_transcripts.messages` persists Pi's own message list. Neither is filtered. So whether the model identifier reaches the browser is a property of Pi's protocol, not of anything decided here — and "the user never learns what is under the hood" is a claim about that stream, not about one JSON route. Establish what Pi actually emits before treating this as done. If it does carry provider or model identifiers, the options are to filter at the broadcast seam or to accept that the network tab tells a curious user what the UI does not; that is a real decision and it belongs in this ticket rather than in a surprise later.

**Unset is exactly today.** Same shape as `OIDC_ISSUER` — absent means the feature does not exist, the config surface is unchanged, and every existing self-hosted instance keeps the behavior it has. `EXHIBIT_SECRET` and the `agent_keys` table stay as they are; platform mode simply does not consult them.

**`exhibit-mock` resolves through the same path**, so tests exercise platform mode without a real credential.

**What this deliberately does not include: metering, or a spend cap.** Enabling platform mode makes every agent session bill to the instance's provider account with nothing bounding it. Nothing in `internal/agent` reads token usage off Pi's event stream today — the events it recognizes are prompts, aborts, and the three synthetic `exhibit_*` save events — so an instance in platform mode can neither attribute spend to an owner nor stop a session that runs away. A runaway session is the operator's loss before any invoice exists, and usage billing meters after the fact, so billing does not close this either.

That is a deliberate scope line, not an oversight: the configuration is useful on its own for a controlled instance, and metering is a larger piece of work with its own design. But **platform mode must not meet public traffic before the cap exists.** Hence the startup warning in the acceptance criteria. Agent token metering per owner, and a per-owner budget checked before a session spawns and again between turns, are the follow-up this depends on for public use; neither is filed yet.

## Acceptance Criteria

- With `AGENT_API_KEY` unset, behavior is identical to today: BYOK entry works, an owner with no key gets `412`, and no new variable appears in any rendered page.
- With it set, an agent session starts for an owner who has no `agent_keys` row.
- In platform mode the key value never appears in an API response, a log line, or a rendered page.
- In platform mode the key resource does not exist: `GET` of the agent key route `404`s, and `PUT`/`DELETE` are refused. Nothing reports the provider or the model.
- In platform mode the agent page renders no key button, key modal, provider select or model input, and makes no key-status call. A user cannot tell from the UI which provider or model is running.
- What Pi emits on its event stream is checked against that claim: the raw SSE passthrough (`internal/agent/agent.go:565`) and the persisted transcript do not hand the browser a provider or model identifier, or the ticket records the decision to accept that they do.
- `AGENT_API_KEY` set without `AGENT_PROVIDER`, or with a provider `agent.KnownProvider` rejects, fails at startup rather than at the first session — the same posture as a malformed `LOGIN_PASSWORD_HASH`.
- An existing `agent_keys` row is neither read nor deleted when platform mode is on; turning the variable off restores BYOK with that key intact.
- Startup logs that platform mode is enabled and that agent sessions bill the instance's provider account, naming the absent spend cap.
- `exhibit-mock` works as the platform provider, so the path is testable with no real credential.
- `docs/deployment.md` documents the three variables, and states that platform mode without a spend cap should not be exposed to untrusted signups.


## Notes

**2026-08-18T05:53:13Z**

Platform mode reports nothing at all — not provider, not model. Someone using AI to build a tool does not need to know what is under the hood; a power user who wants that control self-hosts, where BYOK already provides it. Widened from 'status route omits the key hint' to 'the key resource and the whole agent-settings UI are absent'. Also flagged the real scope: the SSE stream is an unfiltered passthrough of Pi's protocol and transcripts persist its messages, so full abstraction depends on what Pi emits — verify before calling this done.

**2026-08-18T18:26:20Z**

Pi's protocol was checked, not assumed. A real `pi --mode rpc` turn (v0.84.1) emits
`{"role":"assistant","api":"openai-completions","provider":…,"model":…}` on
message_start, message_end, turn_end and agent_end, and the same envelope is what
`agent_transcripts.messages` persists. Both seams were unfiltered, so the decision
recorded here is to **filter**, not to accept: `internal/agent/redact.go` strips
`api`/`provider`/`model` from Pi's message envelopes — objects carrying a `role`, so
artifact data with a `model` field is untouched — at the broadcast seam and before
SaveTranscript, and only in platform mode. BYOK streams are unchanged.

Two things kept deliberately:
- The `usage` block (tokens, cost). It names no model and is what av-hyo6's metering
  will read; dropping it would cost a future feature to hide nothing.
- The server-side `agent session started` log line still names provider and model.
  That is the operator's own log, and the operator set both; the AC's "never in a log
  line" is about the key *value*, which appears nowhere.

Found only by running it in a browser, not by the unit tests: `POST /api/agent/sessions`
returned `provider`/`model` in its JSON, and `send()` in agent.js gated on the module
`keyConfigured` flag, which platform mode never set — the composer silently refused to
send. Both fixed and pinned by tests.
