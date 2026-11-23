# AI in artifacts (`window.exhibit.ai`)

Experimental, off by default. Epic [Exh-zue3](../.tickets/Exh-zue3.md).

An artifact can ask an AI model for prose or for a typed judgment from ordinary
JavaScript. The person using it needs no key, provider account or API
knowledge. Exhibit makes the call on the server and charges it to **the person
viewing the artifact**.

This is a capability bridge (PRD §6.3), not a connector and not network access:
the sandboxed frame never reaches a provider. The call is posted to the host
page like a state write, and the host makes the authenticated request to the
single write path. The artifact's CSP and origin allowlist are untouched.

## For artifact authors

```js
const ai = window.exhibit.ai;            // always defined
const { complete, decide } = await ai.available();

// Prose, or JSON validated against a schema.
const text = await ai.complete({ prompt: 'Summarize: ' + notes });
const plan = await ai.complete({
  system: 'You plan weekly runs.',
  messages: [{ role: 'user', content: goal }],
  schema: { type: 'object', properties: { days: { type: 'array', items: { type: 'string' } } }, required: ['days'] },
  maxTokens: 400,
});

// A typed judgment with a confidence; never text.
const a = await ai.decide({
  state: { message },
  questions: {
    urgent: { type: 'bool', instructions: 'Does this need a reply today?', criteria: { true: 'Blocked or time-critical', false: 'Can wait' } },
    area:   { type: 'choice', instructions: 'Which area?', criteria: { billing: 'Invoices and payments', bug: 'Something broken', other: 'Anything else' } },
    tone:   { type: 'score', instructions: 'How upset is the writer?', criteria: ['calm', 'annoyed', 'angry'] },
  },
});
// a.urgent.probability, a.area.choice / .probabilities / .confidence, a.tone.score / .confidence
```

| Method | Resolves to |
|---|---|
| `available()` | `{ complete: boolean, decide: boolean }` for this viewer, here |
| `complete({ prompt } \| { messages, system?, schema?, maxTokens? })` | a string; with `schema`, the parsed JSON value, validated |
| `decide({ state, questions })` | answers keyed by question name: `bool` → `probability`; `choice` → `choice`, `probabilities`, `confidence`; `score` → `score` (0 … levels−1), `confidence` |

`window.claude.complete(prompt)` is mapped onto `complete()` so artifacts written
for Claude's runtime run unchanged.

**Limits.** complete: 1–40 messages, 48 KB of text, `maxTokens` default 1024,
at most 4096. Schemas use `type`, `properties`, `required`, `items`, `enum`,
`const`, `additionalProperties` (boolean), `minItems`/`maxItems`,
`minimum`/`maximum`, `minLength`/`maxLength`, `description`, `title`; anything
else is refused before a call. An answer that does not match the schema gets
one repair attempt, then `invalid_output`; unvalidated output never reaches the
artifact. decide: at most 8 questions, 2–16 choices, 2–11 score levels, state
≤ 16 KB. decide never falls back to a chat model: without a classifier it is
unavailable, because a probability that means nothing is worse than none.

**Errors** reject with `err.name === 'ExhibitAIError'` and a stable `err.code`;
the message is safe to show.

| Code | Meaning |
|---|---|
| `unavailable` | AI is off here, or this is an export, public link, widget or directly opened tab, or the kind has no model |
| `not_approved` | The tool has not been allowed to use AI (or the person said Not now) |
| `no_key` | Bring-your-own-key instance and this person has no key |
| `limit_reached` | The monthly allowance (the person's, or the instance's) is spent |
| `rate_limited` | Too many calls too fast; try again shortly |
| `invalid_request` | The request broke a rule above |
| `provider_error` | The provider failed or answered something unusable |
| `invalid_output` | complete with a schema: the answer did not fit, after one repair |

**Write tools that degrade.** Check `available()` first. AI is unavailable on a
public link, in a widget, in a tab opened directly, and in every export — HTML
and ZIP, online or not. Disable AI actions with a sentence saying why, and keep
everything else working. Answers vary slightly between runs: threshold
probabilities on ranges, keep thresholds in your code, and treat low confidence
as an outcome. Never put saved data in a prompt without the person seeing what
is sent.

Keep private AI results in the current document. An artifact's localStorage
can be shared with other named viewers through shared-state mode. Ask before
writing a result to the tool's saved data, as the triage example does for its
summary drafts.

## Who may call, and who pays

| Viewer | Allowed? | Pays |
|---|---|---|
| Owner, tool approved | yes | owner |
| Named recipient, tool approved, recipient consented | yes | **the recipient**, never the owner |
| Recipient of a tool the owner never approved | no — explained, no button | — |
| Public link, anonymous / public-mode visitor | never | — |
| Widget, directly opened tab, export | never (stand-in) | — |
| Agent or MCP credential | never | — |
| Owner's version preview | yes, metered | owner |

A version preview keeps AI, unlike devices and location, which it is denied:
only the owner can open one, they open it on purpose, and it spends their own
allowance on code that was the live, approved artifact when it was current.

Two consents, one prompt each, both in Exhibit's own chrome and never in the
artifact's DOM:

- **The owner's approval** (`inference_approved`, one of
  `store.ApprovalColumns`). The first call raises "Let this tool use AI?",
  naming the tool, saying what it sends can include saved data, and saying who
  pays. Allow is also the owner's consent to spend their own allowance.
  Revocable under Edit → Permissions (AI: Ask first / Always allow) and in
  Settings.
- **A recipient's consent** (`artifact_inference_consents`, one row per
  artifact and viewer). On an approved tool shared with them, a recipient is
  asked once whether it may use *their* allowance. Revocable in Settings.

The endpoint enforces both; the host page's checks are only the UX. An agent
or MCP credential cannot set `inference_approved` (400): unlike the other
approvals it spends money, so it is only ever the owner's choice in Exhibit's
chrome. "Not now" holds for the page load, so a tool that calls again at once
is refused without the dialog returning; reloading asks again.

## Where people see it

- **Artifact page / agent preview / version preview**: the prompt above; a
  banner when the owner has not allowed AI (recipient), when there is no key
  (link to add one) or when the allowance is spent (with its reset date).
- **Edit → Permissions**: an AI row beside Downloads … Location, shown when
  the instance offers AI.
- **Posture receipt and gallery card**: a sparkle glyph and "AI — Can send
  text, including saved data, to an AI service; each person pays for their own
  use".
- **Settings → AI in artifacts**: whether AI is available and who pays; in
  platform mode this month's allowance as a share used (shared with the agent),
  with the tools' share of it and the reset date — never money, provider or
  model; requests per tool and kind this month (tokens too when the person pays
  with their own key); and every tool allowed to use AI at their expense, each
  with Stop.
- **Exports**: the download says AI is off in exported copies; the ZIP README
  repeats it ([Exh-i902](../.tickets/Exh-i902.md)).

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `INFERENCE_ENABLED` | `false` | Turns `window.exhibit.ai` on |
| `INFERENCE_MODEL` | *(unset)* | Chat model for `complete()`, a Pi catalog id for `AGENT_PROVIDER` (e.g. `openai/gpt-4.1-nano` on OpenRouter). **Required in platform mode**; must resolve or startup fails |
| `INFERENCE_DECISION_MODEL` | *(unset)* | Classifier model for `decide()` (e.g. `typesafe/jev-1.14` on OpenRouter). Optional; unset, decide is unavailable. Set, it must resolve to a classifier or startup fails |
| `INFERENCE_MOCK_URL` | *(unset)* | Tests and demos only: send every provider request to this mock server (`cmd/mockllm`) |

Providers follow the agent. With `AGENT_API_KEY` + `AGENT_PROVIDER` the
instance pays (platform mode); otherwise each viewer's own stored agent key
pays, using `INFERENCE_MODEL` / `INFERENCE_DECISION_MODEL` when they resolve
for that viewer's provider and a per-provider default otherwise. Platform-paid
calls and agent calls share the same spend totals. The configured
`AGENT_SPEND_CAP_OWNER_CENTS` applies separately to each viewer, and
`AGENT_SPEND_CAP_INSTANCE_CENTS` bounds the instance total. Account-specific
cap overrides are deferred. Platform mode requires at least one of these caps.
User-paid calls are outside the operator's caps. With `GUARDRAIL_*` set,
Exhibit screens the full input before each platform-paid call, including
schema repairs. Each screen makes one provider request. A refusal, invalid
verdict, or failed usage write prevents the next paid step in that request.

Per-call belts, whoever pays: 30 calls a minute per viewer and 60 per artifact
(bursts of 10), 4 in flight per viewer, 60 s for complete and 20 s for decide.

## Runtime

One warm Pi process (`internal/inference/worker.go`, extension
`ext/inference.ts`) serves every call through Pi's model registry and its own
provider adapters — the decision of spike [Exh-urrg](../.tickets/Exh-urrg.md).
It is separate from the agent's session manager. Each call carries its own key,
deadline and abort; the worker holds no credentials, runs no tools, sessions or
user extensions, and every prompt it receives is a JSON envelope its input hook
consumes, so nothing ever becomes a model turn. Supervision: restart with
exponential backoff, a circuit breaker after five failed starts, bounded
admission (16 in flight), a per-call cancel that kills the generation if
ignored, and **no replay** of a call whose outcome is unknown, because it may
already have been billed.

Metering runs before error mapping: one `agent_usage` row per provider call
(`source = artifact_inference`, with `artifact_id`, `kind` and `cost_source`),
including failed calls that reported usage. A call that reported nothing is
recorded with `cost_source = none` — unknown spend, not zero. `owner_id` on an
inference row is the paying viewer.

## What is verified, and what is not

Automated: the real worker (pinned Pi Bolt 1.0.8 / Pi 1.4.0, and the `pi` on
`PATH`) against `internal/mockllm` through Pi's real OpenRouter adapters —
complete, classify with all three question types, overlapping calls with
distinct keys, independent cancel, upstream errors redacted, restart without
replay; the core with a fake runtime; the endpoint's principal, approval and
consent rules; the render placement of the stand-in and bridge; and the
triage example running in Chromium through the real worker and a local mock
provider, with both AI controls disabled in each export format.

**Not verified: live provider compatibility.** No paid provider call was made.
In particular Pi's OpenRouter classifier route (`/api/v1/systemone`) has not
been checked against OpenRouter's current decisions endpoint
(`/api/alpha/decisions`). Before enabling in production, run one manual
complete and one decide against the real provider with a real key and confirm
answers and usage arrive.
