# AI in artifacts (`window.exhibit.ai`)

Experimental, off by default. Epic [Exh-zue3](../.tickets/Exh-zue3.md).

An artifact can ask an AI model for prose or for a typed judgment from ordinary
JavaScript, and can show `complete()` images to read
([Exh-v81w](../.tickets/Exh-v81w.md)). The person using it needs no key, provider account or API
knowledge. Exhibit makes the call on the server and charges it to **the person
viewing the artifact**.

This is a capability bridge (PRD §6.3), not a connector and not network access:
the sandboxed frame never reaches a provider. The call is posted to the host
page like a state write, and the host makes the authenticated request to the
single write path. The artifact's CSP and origin allowlist are untouched.

## For artifact authors

```js
const ai = window.exhibit.ai;            // always defined
const { complete, decide, images } = await ai.available();

// Prose, or JSON validated against a schema.
const text = await ai.complete({ prompt: 'Summarize: ' + notes });
const plan = await ai.complete({
  system: 'You plan weekly runs.',
  messages: [{ role: 'user', content: goal }],
  schema: { type: 'object', properties: { days: { type: 'array', items: { type: 'string' } } }, required: ['days'] },
  maxTokens: 400,
});

// Images: a user message's content becomes a list of parts. data is base64,
// or the data: URL a canvas returns. Downscale first (below). Load a picked
// File with createImageBitmap: the render CSP's img-src has no blob:, so an
// <img> cannot show URL.createObjectURL(file).
const photo = await createImageBitmap(input.files[0]);
const canvas = document.createElement('canvas');
const scale = Math.min(1, 1568 / Math.max(photo.width, photo.height));
canvas.width = Math.round(photo.width * scale);
canvas.height = Math.round(photo.height * scale);
canvas.getContext('2d').drawImage(photo, 0, 0, canvas.width, canvas.height);
const total = await ai.complete({
  messages: [{ role: 'user', content: [
    { type: 'text', text: 'What is the total on this receipt?' },
    { type: 'image', mimeType: 'image/jpeg', data: canvas.toDataURL('image/jpeg', 0.85) },
  ] }],
  schema: { type: 'object', properties: { total: { type: 'number' } }, required: ['total'] },
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
| `available()` | `{ complete: boolean, decide: boolean, images: boolean }` for this viewer, here; `images` is whether `complete` accepts image parts |
| `complete({ prompt } \| { messages, system?, schema?, maxTokens? })` | a string; with `schema`, the parsed JSON value, validated |
| `decide({ state, questions })` | answers keyed by question name: `bool` → `probability`; `choice` → `choice`, `probabilities`, `confidence`; `score` → `score` (0 … levels−1), `confidence` |

`window.claude.complete(prompt)` is mapped onto `complete()` so artifacts written
for Claude's runtime run unchanged.

**Limits.** complete: 1–40 messages, 48 KB of text, `maxTokens` default 1024,
at most 4096.
Images have bounds of their own and do not count against the text: PNG, JPEG
or WebP only (GIF is refused, because Gemini cannot read it; draw other formats
onto a canvas first), at most 4 per call, 2 MB each and 4 MB in total, decoded;
only user messages carry them; a message holds at most 16 parts. `mimeType`
must name what the bytes really are (they are sniffed), and a `data:` URL's
type must agree with it. A string `content` works exactly as before. Schemas use `type`, `properties`, `required`, `items`, `enum`,
`const`, `additionalProperties` (boolean), `minItems`/`maxItems`,
`minimum`/`maximum`, `minLength`/`maxLength`, `description`, `title`; anything
else is refused before a call. An answer that does not match the schema gets
one repair attempt, then `invalid_output`; unvalidated output never reaches the
artifact. The repair does not send images again: it fixes the format of an
answer already written, so each image is billed once, and the model is told an
image was left out of the retry. `maxTokens` caps everything the model generates, hidden reasoning
included, so a small budget can leave no room for an answer: an empty reply is
`invalid_output`, never `""` (and is not repaired), so give a reasoning model
room. decide: at most 8 questions, 2–16 choices, 2–11 score levels, state
≤ 16 KB. decide never falls back to a chat model: without a classifier it is
unavailable, because a probability that means nothing is worse than none.

**Errors** reject with `err.name === 'ExhibitAIError'` and a stable `err.code`;
the message is safe to show.

| Code | Meaning |
|---|---|
| `unavailable` | AI is off here, or this is an export, public link, widget or directly opened tab, or the kind has no model, or the call carries an image and `available().images` is false |
| `not_approved` | The tool has not been allowed to use AI (or the person said Not now) |
| `no_key` | Bring-your-own-key instance and this person has no key |
| `limit_reached` | The monthly allowance (the person's, or the instance's) is spent |
| `rate_limited` | Too many calls too fast; try again shortly |
| `invalid_request` | The request broke a rule above |
| `provider_error` | The provider failed or answered something unusable |
| `invalid_output` | complete: the model wrote no answer (it ran out of `maxTokens`), or with a schema, the answer did not fit, after one repair |

**Images leave the device.** An image is sent to the provider as it was given
to `complete()`. Exhibit does not strip metadata from it, so a photo straight
from a camera can carry where and when it was taken. Drawing it onto a canvas
and sending `canvas.toDataURL()` re-encodes the pixels only, which drops that
metadata, and also cuts bytes and cost; the agent guidance teaches tools to do
this, and the consent prompt says images leave the device.

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
  text and images, including saved data, to an AI service; each person pays for
  their own use".
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
The screen reads text only, so platform-paid calls on an instance with
`GUARDRAIL_*` set carry no images (`available().images` is false and an image
call is `unavailable`) rather than send them unscreened. BYOK calls are never
screened, so there images follow the viewer's own model whatever
`GUARDRAIL_*` says.

An image call's request body is up to about 6 MB, inside the default
`MAX_REQUEST_BODY_BYTES` (32 MiB). An instance that lowers it below that refuses
larger image calls before the inference handler; the artifact then sees
`invalid_request` saying the request is too large.

The optional MCP service learns what to teach connected agents from
`EXHIBIT_AI_KINDS`. Add `images` beside `complete` (`complete,images`) when the
core offers images; `images` without `complete` is refused at startup. The
built-in agent needs no setting: the core works this out from the catalog.

**Which models read images.** Pi's catalog lists each model's input types, and
`available().images` follows it: in platform mode for `INFERENCE_MODEL`, in
BYOK mode for the model each viewer's provider resolves to. A text-only model
refuses an image call before any provider request. There is no setting to
override the catalog.

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

An image call's envelope can reach about 6 MB; the worker's envelope bound is
8 MB. With 16 calls in flight that is on the order of a few hundred MB held
briefly at the worst, which per-viewer concurrency (4) and the rate belts keep
rare; size a small instance's memory with it in mind. Image bytes are never logged: not in the worker, the job, an error
message or the ledger, which records tokens and cost only.

Metering runs before error mapping: one `agent_usage` row per provider call
(`source = artifact_inference`, with `artifact_id`, `kind` and `cost_source`),
including failed calls that reported usage. Image input is billed by the
provider as tokens and arrives in that usage, so an image call is metered and
counts against the spend caps like any other: the caps are checked against
what has been spent, before each call, never estimated from the request's size,
and the rate belts count calls, not bytes. A call that reported nothing is
recorded with `cost_source = none` — unknown spend, not zero. `owner_id` on an
inference row is the paying viewer.

## What is verified, and what is not

Automated: the real worker (pinned Pi Bolt 1.3.1 / Pi 1.6.0, and the `pi` on
`PATH`) against `internal/mockllm` through Pi's real OpenRouter adapters —
complete, classify with all three question types, overlapping calls with
distinct keys, independent cancel, upstream errors redacted, restart without
replay; the core with a fake runtime; the endpoint's principal, approval and
consent rules; the render placement of the stand-in and bridge; and the
triage example running in Chromium through the real worker and a local mock
provider, with both AI controls disabled in each export format.

**Verified live, once, for `complete` only.** On 2026-11-25 a platform-mode
dev instance on OpenCode Go (`deepseek-v4.1-flash`) answered a plain `complete`
and a schema-constrained `complete` from an artifact, through the host bridge
and the warm worker, and metered both with `cost_source = catalog`. That check
found a bug no mock could: OpenCode Go refuses a request without a session id
(`400 MissingSessionID`), and the worker's direct registry calls sent none, so
every call failed as `provider_error`. The worker now gives each provider call
a fresh random `sessionId`.

A second, independent pass on a fresh instance (19 real calls) also exercised
the owner's approval prompt (allow and Not now), a recipient's consent prompt
with the usage row charged to the recipient and never the owner, an artifact
whose owner never allowed AI (no consent offered), the public link, widget,
directly opened tab and export (all unavailable, no provider call), the 48 KB
limit, the in-flight belt (`rate_limited`), and a leak search of everything the
artifact could see. It found that a reasoning model could spend all of
`maxTokens` thinking and return `""` as a success; that is now `invalid_output`.
Not run live: a connected-agent credential (it needs `MCP_URL`), and the
per-minute token bucket.

**Images, verified live once** ([Exh-v81w](../.tickets/Exh-v81w.md)). On
2026-11-27 a platform-mode dev instance on OpenCode Go with the pinned Pi Bolt
1.3.1 / Pi 1.6.0, `INFERENCE_MODEL=deepseek-v4.1-flash` (image input in Pi's
catalog): `available()` reported `images: true`; a JPEG with a red circle left
of a blue square was described correctly as plain text, and as schema output
(`[{shape: circle, color: red}, {shape: square, color: blue}]`) when sent as a
`data:` URL; a string `prompt` on the same instance answered as before. Image
calls were metered as ordinary `complete` rows (`cost_source = catalog`, about
290–360 input tokens against 79 for the text call). In a browser, a tool that
loaded a picked file with `createImageBitmap`, downscaled it in a canvas and
sent it raised the owner's prompt with the new image line, then showed the
model's answer. That pass found that `URL.createObjectURL(file)` cannot be shown
in an `<img>` under the render CSP, so the guidance teaches `createImageBitmap`.
Restarted with `INFERENCE_MODEL=glm-5.3` (text only), `images` was false and an
image call was `unavailable` with no usage row. The server log held no image
data. Not run live: a schema repair of an image call (the model answered
validly first time; the repair is covered against a fake runtime), a recipient's
consent on an image tool, BYOK, a guardrail-screened instance (covered by
tests), and OpenRouter vision models.

**Not verified: `decide`, and OpenRouter.** No classifier model was available
on the provider tried, so `decide` has not run against any real provider. In
particular Pi's OpenRouter classifier route (`/api/v1/systemone`) has not been
checked against OpenRouter's current decisions endpoint
(`/api/alpha/decisions`). Before enabling `decide`, run one manual `decide`
against the real provider with a real key and confirm answers and usage arrive.
A provider's own error text is deliberately never logged, so a provider that
rejects every call shows only `provider_error`; run the call once through a
build that logs it, as the check above did.
