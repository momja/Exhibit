# Exhibit — Agent Integration (Pi harness)

The build-and-modify-with-AI surface (epic `Exh-yvhp`, grown out of `av-q3wo`),
and the main way artifacts are made. A chat page lets the user create new artifacts and modify existing
ones through an LLM agent, using **their own API key**, with everything the
agent saves flowing through the normal ingest path.

## How it fits the architecture

Pi (`pi-mono`, Mario Zechner's agent harness) runs as a **sidecar subprocess**,
one per chat session, spawned by the Go service as
`pi --mode rpc --session <scratch>/session.jsonl --no-builtin-tools -e exhibit.ts`. The image
ships `pi`; unlike the satellites in architecture §3.6 it is not optional. The
service talks strict JSONL over stdin/stdout (Pi's RPC mode) and fans events
out to the browser over SSE.

```
browser chat UI ──POST prompt──► Go service ──JSONL stdin──► pi (sidecar)
      ▲                              │                          │
      └────────── SSE events ◄───────┘◄───JSONL stdout──────────┘
                                                                │ tool calls
                        exhibit API (single write path) ◄───────┘
                        POST /api/artifacts · PATCH /api/artifacts/:id
```

The single write path is preserved: the agent's only tools are
`create_artifact` / `write_artifact` / `edit_artifact` / `get_artifact` for the document,
`get_state` / `set_state` / `delete_state` for the artifact's stored state
(av-lvi1), and `set_widget` / `edit_widget` / `get_widget` for the artifact's gallery tile
(av-fafu), plus `get_selection` for the elements the user picked in the preview —
all registered by a Pi extension (`internal/agent/ext/exhibit.ts`,
materialized to the data dir at startup) that calls back into the exhibit HTTP
API. Agent output is scanned like any other ingest; scanned origins are
**never** auto-approved — the chat UI tells the user when a saved artifact has
a network footprint awaiting approval.

## Scope: one session, one artifact

A session reaches exactly one artifact, and that is enforced twice — once for
ergonomics, once for real. `security.md` §5 is the full statement; the shape:

- **No tool takes an artifact id.** `create_artifact(title, body)`,
  `write_artifact(body[, title])`, `edit_artifact(edits)`, `get_artifact()`, `get_state()`,
  `set_state(key, value)`, `delete_state([key])`, `set_widget(body)`, `edit_widget(edits)`,
  `get_widget()`, `get_selection()`. A tool with no id parameter cannot be talked into a
  different target, which matters because artifact bodies and titles are
  untrusted text that reaches the model's context. The extension resolves the
  target from `EXHIBIT_ARTIFACT_ID`, or from the API's response to the first
  create.
- **Edits are surgical by default, rewrites by exception.** `edit_artifact`
  carries pi-style `edits[{oldText, newText}]` (exact match first, fuzzy
  quote/dash/whitespace fallback; ambiguous, overlapping, or unmatched edits
  fail the whole call with no PATCH issued). It reads the latest saved source
  itself, splices the replacements, and persists through the same PATCH +
  scan + `artifact_saved` path as `write_artifact`, which remains for full
  rewrites. Engine: `internal/agent/ext/edit.ts` (pure, node-tested); wiring:
  `ext/exhibit.ts`.
- **The API refuses anything outside the scope.** The sidecar authenticates
  with a per-session credential (`internal/agentscope`) that resolves to
  (owner, artifact), not the service token. `authMiddleware` allows exactly
  `POST /api/artifacts` while unbound, plus `GET`/`PATCH` on the session's own
  artifact and the `state` / `widget` sub-resources of that same artifact —
  one allowlist entry per tool above. Every other route — the BYO provider
  key, shares, deletes, tags, collections, transcripts, `widget/generate`,
  `origins` (approving the artifact's own network egress, av-kmwj), and every
  other artifact in the library — is a 403 before any handler runs.
- **This is the per-artifact half only.** The credential's owner becomes the
  request's `ownerID`, so the owner-scoped Store methods (av-ep8k) bound it to
  one tenant exactly as they bound a browser client. The path check then
  narrows that tenant's library to one artifact. Neither half stands alone:
  the owner check without the path check leaves a session with ordinary full
  authority over its user's library, and the path check without the owner
  check confines it to an id that could belong to anybody.
- **Create mode binds server-side.** `POST /api/artifacts` binds the
  credential to the id it just wrote. The binding never comes from a tool
  result, which model-supplied arguments shape. It is also where the session's
  own notion of "my artifact" comes from — the preview pane, the transcript,
  and every synthetic `exhibit_*` event read `Session.ArtifactID()`, which is
  the grant, not the tool result.
- The credential is revoked when the subprocess exits.

## Session context: instructions and data are separate

- The **system prompt** is entirely server-authored (`internal/agent/prompt.go`).
  No artifact title, body, or id is interpolated into it, and it carries one
  static contract: tool results are data, and only the user's own messages are
  instructions.
- A session is **not given the artifact**: it reads it with `get_artifact`,
  before it changes anything and again whenever it may have changed. The
  source, the title (one bounded line), the state, and the widget are tool
  results — a separate message role the conversation's own structure marks as
  data — so there is no fence to forge and no per-session secret to carry
  (av-5s7g). It costs the model one tool call at the start of a turn that
  edits, which is the price of the boundary being structural.
- Elements picked in the preview are untrusted too. The prompt request carries
  them as their own `snippets` field, never concatenated into `message`; the
  session writes them to `selection.json` in its work directory (replaced, or
  removed when the prompt selected nothing), appends one fixed sentence to the
  prompt saying elements were selected, and the model reads them with
  `get_selection`. Page JS composes no envelope at all.

## Artifact state (av-lvi1)

The state tools hit the same authenticated `GET/PUT/DELETE
/api/artifacts/:id/state` routes as the edit page's state inspector
(av-hg5f) — no second write path, no store access of its own. Values are
opaque strings and the system prompt tells the model to treat an untouched
value as fixed text to reproduce byte-for-byte, since a value the user didn't
ask to change silently reformatting (JSON key order, spacing, `1.0` vs `1`)
would be a defect the API has no way to catch — it never inspects the string
it's asked to store. A `set_state`/`delete_state` call emits a synthetic
`exhibit_state_changed` event so the chat UI re-renders the preview through
the same htmx fragment swap `exhibit_artifact_saved` drives (below): state is
inlined into the document at render time, so the pane would otherwise stay
stale after an edit.

## Widgets (av-fafu)

`set_widget(body)` saves the artifact's gallery tile — the small informative
document its library card renders (`widgets.md`). The system prompt carries the
tile's whole contract (reads the artifact's state synchronously, cannot write
it, never interactive, one fact large, ~272×132 fluid, always an empty state,
static-with-no-script for a stateless tool), so the agent builds one by default
and a tool arrives in the library with a face.

A widget save emits `exhibit_widget_saved` rather than reusing
`exhibit_artifact_saved`: the artifact body didn't change, only the tile beside
it, and the event carries the origins the *artifact's* allowlist doesn't cover
— already blocked at render, so the chat says so plainly instead of offering an
approval that isn't pending. Both events re-fetch the same preview fragment, and
the pane renders the tile so the default one is visible too — that being what
"no widget yet" looks like.

### One-shot sessions (the edit page's Generate button)

`POST /api/artifacts/:id/widget/generate` runs the agent as a *function* rather
than a chat: it creates a session with `CreateOpts.WidgetOnly`, sends one fixed
server-side prompt, and returns the session id. The caller subscribes to the
same SSE route the chat uses and waits for `exhibit_widget_saved`, so the whole
feature adds a route and no streaming machinery.

`WidgetOnly` exists because the ordinary modify-an-artifact scoping tells the
model to save with `write_artifact` — exactly what a generate-the-tile session
must never do. The two are mutually exclusive branches of `modePrompt`
(`internal/agent/prompt.go`) for that reason, and `internal/mockllm` plays the
widget branch so the path is covered end to end. Note the button's route is
*not* reachable by an agent credential: a session cannot start another session.

## BYO API key (encrypted at rest)

- `PUT/GET/DELETE /api/agent/key` — one configured provider key per owner
  (`agent_keys` table). The key crosses the wire once on PUT, is sealed with
  AES-256-GCM (`internal/secrets`) under the server secret (`EXHIBIT_SECRET`
  env, else a generated `data/secret.key`), and is never returned — GET yields
  only `sk-…1234`-style hints.
- At session spawn the key is decrypted and handed to the pi subprocess via a
  provider-specific env var (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`,
  `GEMINI_API_KEY`, `OPENCODE_API_KEY`, …) — never argv, never page JS. The
  subprocess env is built minimal from scratch so server credentials cannot
  leak into sessions, and `HOME` is pinned to the session workdir so pi cannot
  read the operator's `~/.pi/agent/auth.json` — a stored login there would
  otherwise take precedence over the BYO key and silently bill the operator's
  account.

The same env carries the exhibit-side contract, and nothing broader than the
session needs:

| Var | Value |
|-----|-------|
| `EXHIBIT_API_URL` | app origin the tools call back into |
| `EXHIBIT_TOKEN` | the session's **scoped** credential — not the service token |
| `EXHIBIT_ARTIFACT_ID` | the session's artifact (empty in create mode) |
| `EXHIBIT_SELECTION_FILE` | where `get_selection` reads the elements the user picked |
| `EXHIBIT_SESSION_ID` | this session's id |
| `EXHIBIT_MAX_BODY_BYTES` | the API's request body limit (`MAX_REQUEST_BODY_BYTES`). Every tool's write checks against it before sending, so an oversized write fails with a message the model can act on instead of an upload the API refuses (av-ombn) |

Supported providers: Anthropic, OpenAI, Google Gemini, OpenRouter, OpenCode
Go, plus `exhibit-mock` when `MOCK_LLM_URL` is set.

## Platform mode: the instance supplies the key (av-siqf)

BYOK above is the self-hosted path and the default. Setting `AGENT_API_KEY`
(with `AGENT_PROVIDER`, and optionally `AGENT_MODEL`) puts the instance in
**platform mode**: every session runs on that one credential. It exists for a
hosted deployment, where asking someone to open a provider account and paste a
key before they can use the headline feature is the step that loses them.

One variable chooses between two modes; a per-owner key does not take
precedence over an instance-wide fallback. That shape reads like the flexible
one and is worse in both directions — it silently mixes billing models, and it
leaves a key field on a surface whose whole point is that nobody needs one.

**Platform mode reports nothing: not the key, not the provider, not the
model.** Someone using AI to build a tool does not need to know what is under
the hood, and naming it invents a decision they cannot act on; anyone who wants
that control self-hosts, where BYOK gives it to them in full. Concretely:

- `agentSessionOpts` (`internal/api/agent.go`) resolves the platform key and
  never reads `agent_keys` — an owner's stored key is neither read nor deleted,
  so turning the variable off restores their BYOK session with that key intact.
- `GET`/`PUT`/`DELETE /api/agent/key` all `404`: the resource does not exist.
- The agent page renders no key button, no key modal, no provider `<select>`
  and no model input — absent, not disabled — and its bootstrap sets
  `BYOK = false` so the page never calls the key route.
- Pi's own identifiers are stripped from the event stream, and a stored
  conversation is served only as a projection that carries none (below).
- Availability is a separate, unchanged signal: a missing `pi` binary still
  disables the surface in either mode.

### What Pi emits, and what is filtered

Every assistant message Pi emits carries the model's identity, and one of this
service's publishing seams passes Pi's protocol through verbatim — the SSE
broadcast. (A stored conversation holds the same fields, but is served only as
a projection that names fields explicitly; see Conversations below.) Captured
from a real
`pi --mode rpc` turn (Pi 0.87.1, the version the Dockerfile pins):

```json
{"type":"turn_end","message":{"role":"assistant","content":[…],
 "api":"openai-completions","provider":"anthropic","model":"claude-sonnet-4-5",
 "usage":{…},"stopReason":"toolUse"}}
```

It appears on `message_start`, `message_end`, `turn_end` and `agent_end`. So
"the UI names no model" would have been a claim about one page while the
network tab said otherwise. In platform mode `internal/agent/redact.go` strips
`api`/`provider`/`model` from Pi's **message envelopes** — objects carrying a
`role`, and nothing else, so a `model` field inside artifact data or a tool
argument is left alone. BYOK is unfiltered: there the identifiers describe a
key the caller typed.

The `usage` block beside them (token counts and cost) is deliberately kept: it
names no model, and it is what metering reads (av-2yws, below).

### Token metering (av-2yws)

Every session's spend lands in the `agent_usage` ledger as rows arrive — one
row per recorded usage event, carrying the owner, session, provider, model,
input/output/cache-read/cache-write tokens separately, a cost estimate, and
two tags: **who pays** (`platform` for the instance's credential and guardrail
screening, `user` for a BYO-key session) and **what spent it** (`model`,
`tool`, `compaction`, `guardrail`). Per-session rows aggregate to per-owner
totals over a period; the reverse is not recoverable, so the rows are fine
grained on purpose.

What the rows come from, and when Pi reports it:

- **Assistant messages** — `message_end` carries the message's authoritative
  `usage`; `message_update` carries the same numbers cumulatively *during*
  the response (possibly zero until completion). Recorded as `model` spend.
- **Tool-reported usage** — rolled into toolResult messages (nested tool
  calls fold into their caller), recorded as `tool` spend. Usage sources that
  emit no event at all (Pi's cache warmer, anything else counted only in the
  totals) are caught at every settle: the session reconciles against
  `get_session_stats` — Pi's own cumulative total — and records the gap, so
  the ledger's sum equals Pi's total.
- **Compaction** — `compaction_end` carries the summary call's `usage`.
- **Guardrail screening** (av-gust) — *not* reported by Pi at all: model
  calls made inside hooks appear in no event and in no session total. When
  the guard's signal reaches Go it records the spend tagged `guardrail`.

Attribution survives aborts, idle-reap kills, and mid-turn deaths: rows land
as usage arrives, and whatever a killed response had reported so far is
flushed when the process exits. That is the point — those are the sessions
that spent money and produced nothing. Rows survive account deletion; the
money was spent either way. What is *not* metered is anything Pi itself does
not report (a provider that never returns usage for a failed request is
invisible here too).

**This is not a bill.** Tokens are the meter and are exact. The cost column is
Pi's own price-table estimate of what the spend cost — good enough to bound
spend and attribute rough cost, never shown to a user as an amount owed, and
zero for a model Pi has no price for (the tokens still say what happened).
The mapping from tokens to anything a user is sold is pricing's problem, and
pricing changes without a migration.

### The spend cap (av-99f4)

Layered ceilings over platform-paid spend, each failing differently:

| Env | Ceiling |
|-----|---------|
| `AGENT_SPEND_CAP_OWNER_CENTS` | per-owner budget, calendar month (UTC) — the product-level limit a plan will map onto (av-2p8z) |
| `AGENT_SPEND_CAP_SESSION_CENTS` | one conversation's lifetime — catches a loop without waiting for the month to drain |
| `AGENT_SPEND_CAP_INSTANCE_CENTS` | the operator's total exposure, guardrail screening included, calendar month — the backstop; logged loudly when reached |
| `AGENT_SPEND_CAP_TURN_SECONDS` | wall-clock per turn — the fallback that bounds one pathological response without needing token counts |

With none of them set, nothing is enforced and behaviour is exactly what it
was before this existed. But a platform credential (`AGENT_API_KEY`) with no
cap at all **fails at startup** — absence of the feature is unlimited,
absence of a limit the feature requires is a refusal to boot. A BYO-key
session is never limited by the operator's defaults on any instance: it
spends its owner's own tokens. Guardrail spend is the operator's money even
there; it counts against the instance ceiling only.

Enforcement is **between runs** (the product decision of 2026-09-30:
sessions finish). Before a session spawns, and before every prompt of a
running one, the limits are asked; a refusal is one canned message naming the
limit and when it resets, and the run in flight is never interrupted — it
runs to settle, and the next prompt is the one refused. Two hard stops remain
as failure backstops: the instance ceiling (the operator's money running out
cannot wait for anybody's turn) and `AGENT_SPEND_CAP_TURN_SECONDS`. A
stopped run leaves the artifact on its last successful save and its
transcript intact: stopping is recoverable in a way silently continuing is
not. Enforcement is local sums over local rows and local config — it holds
with every external service unreachable.

**The bound, stated rather than implied: at most one full run per open
session spends past the budget, so at most ten runs — one per session a
single owner can hold.** A run is one prompt through settle, including every
tool call inside it. Nothing enforces inside a run (and `before_provider_request`
could not anyway: it can rewrite a request but not cancel one), so the
`AGENT_SPEND_CAP_TURN_SECONDS` ceiling is the belt for the one case token
counts cannot see: a response whose provider reports usage only when it
ends, or one that simply never ends.

## Usage-policy guardrail (av-gust)

An operator who sets `GUARDRAIL_*` gets every user message screened for
usage-policy violations before the agent model sees it. It protects the
instance's provider account in platform mode, where a provider that sees
repeated violations can suspend the key every session runs on. It is not a
topic filter: off-topic use is allowed. Nor is it a spend control: what each
screen costs is recorded in the ledger tagged `guardrail` (below), and the
limits over it are the spend cap's (av-99f4, above).

- **A second Pi extension.** `internal/agent/ext/guard.ts` is embedded and
  materialized beside `exhibit.ts`, and loaded with its own `-e` only when a
  guardrail is configured. It imports nothing from `exhibit.ts`, and its whole
  contract is `EXHIBIT_GUARD_*` in the sidecar's environment, so it could leave
  this repository as a Pi package.
- **Its own model and key.** The key travels as a per-request `apiKey` option,
  which takes precedence over Pi's own credential lookup, so the guardrail and
  the agent can use the same vendor without sharing a key. The model resolves
  from Pi's catalog: a classifier model is asked one `bool` question through
  `modelRegistry.classify()` and blocks at p ≥ 0.5; anything else is a chat
  model asked to answer ALLOW or BLOCK, with the message JSON-encoded as data.
- **Pi's `input` hook.** The handler screens the text before the model runs. A
  block returns `handled`: no turn starts, the agent model is never called,
  and the message never enters its context, so a blocked persona prompt cannot
  prime later turns.
- **The user's words only.** A prompt carries the user's own words and nothing
  else: stored artifact source reaches the model as a tool result, never in the
  message (av-5s7g), so the whole message is what the screen judges and the
  agent and the screen see the same words. (`guard.ts` still accepts an
  optional `EXHIBIT_GUARD_DATA_FENCE` for a host that does attach data to a
  message; this one leaves it unset.)
- **The host owns the reply.** The extension signals a block with one
  `ctx.ui.notify` whose message starts `exhibit_guard:`. `Session.handleLine`
  swallows that line, logs its detail, and broadcasts `exhibit_guard_blocked`
  carrying `agent.GuardrailBlockedReply`. No extension or model text reaches
  the user on a block. The chat renders it as a system message; the edit
  page's Generate button reports it as a failure.
- **Every screen reports what it spent.** Pi meters nothing an extension
  hook spends, so after each screen that got a model answer (allowed,
  blocked, or unparseable) the extension sends `exhibit_guard:usage` with
  `{provider, model, usage}` in Pi's usage shape. `handleGuardSignal` matches
  the word after the prefix exactly: only `blocked` and `error` are refusals,
  and a usage report is swallowed and handed to `noteGuardUsage`. It is
  operator spend even in a BYO-key session, since the guardrail always runs
  on the instance's key.
- **Fail closed.** A screen that errors, times out (15 s), or answers anything
  unparseable blocks with the same reply, so the reply is no oracle for
  probing whether the screen is up.

Not screened: images, the agent's replies, and what it saves. Pi is pinned in
the Dockerfile because all of this depends on the `input` hook's `handled`
behaving as it does in the pinned version.

## Sessions, streaming, transcripts

- `POST /api/agent/sessions` (optional `artifact_id` scopes the session to an
  existing artifact for modify mode; the model reads it with `get_artifact`),
  `POST …/prompt` (`message` + optional base64 `images` + optional
  `snippets`, the element descriptors the model reads with `get_selection`),
  `POST …/abort`, `DELETE …`.
- `GET /api/agent/sessions/:id/events` — SSE. EventSource can't set headers, so
  this one route resolves its own credential, and which one depends on the
  instance. On one with an identity provider it is the **session cookie** the
  browser attaches to a same-origin stream on its own, since such a page is
  deliberately handed no token to pass (`security.md` §1.5). On a single-user
  instance, whose page *does* hold the static token, it is a **session SSE
  ticket** (`?ticket=`) rather than that token (av-rgp1): a token in a URL
  would be copied into the debug request log, the operator's proxy access log,
  and browser history, and the service token is the whole library. A ticket is
  a random value bound to one session, single-use, and valid for 30 seconds —
  minted by `POST /api/agent/sessions` (returned as `sse_ticket` beside the
  id), `POST /api/artifacts/:id/widget/generate`, or
  `POST /api/agent/sessions/:id/ticket`, all of them ordinary
  header-authenticated requests. Either credential resolves the same Principal
  an API-group request would carry, so this route applies the same owner
  authorization check as every other session route. Because a ticket is spent
  on connect, the chat page drives its own reconnect (mint, then reconnect,
  with backoff) instead of relying on EventSource's automatic retry; the
  session's event backlog replays on subscribe, so nothing is lost. An agent
  session's own scoped credential is
  accepted for neither: it is not a page credential.

- `internal/agent` tracks streaming state (prompts sent mid-stream become Pi
  steering messages), keeps an event backlog for late subscribers, and reaps
  idle sessions. On every settled turn it keeps the conversation (below).
- An owner holds at most **10 open sessions** (widget-generate sessions
  counted): at the limit the oldest idle one is evicted, and only when all
  ten are mid-run is a new one refused (429). The chat page closes its
  session on `pagehide`, so reloads do not stack sessions against the limit.
- When a save-tool call succeeds, the session emits a synthetic
  `exhibit_artifact_saved` event; a `set_state`/`delete_state` call emits the
  analogous `exhibit_state_changed` event, and `set_widget`/`edit_widget` the
  `exhibit_widget_saved` one. All three name the session's own artifact, read
  from the credential's scope rather than from the tool result. The chat UI
  uses any of them to re-render the live preview (see below).

## Conversations (av-y7td)

A conversation is kept as Pi itself records it. Each session is spawned with
`--session <workDir>/session.jsonl`, so Pi appends its own session file — every
entry, in order, compacted or not — and on every `agent_settled` the service
reads that file back and stores it in the conversation's `agent_transcripts`
row (`internal/agent` `persistTranscript`). Pi's session file is the one format
`pi --session` starts from, so keeping it is what makes a conversation
resumable; it is stored as it is rather than translated into anything of ours.

The row also records which version of the artifact the conversation was last
working against — the artifact's head version when the file was stored, read in
the same statement (`store.SaveTranscript`) — and a title, the first prompt
shortened. Storing is an upsert per (artifact, session), so a conversation is
one record that grows; a conversation that outgrows 64 MiB (each read of the
artifact is a full copy in the file) stops being kept rather than growing the
database without limit, and the live session carries on.

What leaves the server is a **projection**, never the file. A session file holds
the system prompt, every tool result in full, the model's reasoning and the
provider and model that answered. `agent.Messages` builds what a person saw —
what they said, what the assistant said, which tools it used — from named
fields, so a field Pi adds later is not published by default and platform mode's
unreported model stays unreported. Tool labels match the live chat's.

- `GET /api/artifacts/:id/transcripts` — the conversations, newest first, as
  summaries (`session_id`, `title`, `version_seq`, `resumable`, `updated_at`)
  with the artifact's `head_seq` beside them.
- `GET /api/artifacts/:id/transcripts/:sessionID` — one, with its `messages`.
- Both are owner-only, answer another owner's artifact as an empty list / a
  404, and are absent from `agentSubResources`: an agent session cannot read
  the conversations before it.

Conversations kept before this existed hold a message dump and no file. They
list and read like the rest (`resumable: false`, `version_seq: 0`) and can only
be read.

**Every session runs from `/`.** A session file's header records the directory it
ran in, and Pi refuses to resume a file whose directory is gone ("Stored session
working directory does not exist") — which a per-session directory never survives,
because it is removed when the session ends. A session has no use for a working
directory of its own: no built-in tools, no context files. Its scratch directory
(`HOME`, the session file, `selection.json`) is a different thing, and is deleted
once the process has exited, everything it printed has been read, and the last
settled turn is stored. The conversation's home is the database; a copy left on
disk would outlive the account that owned it.

## Chat UI

`GET /agent` (create) and `GET /agent?artifact=<id>` (modify; also linked from
the artifact detail toolbar as "Modify with agent"). Server-rendered like the
gallery: chat + streaming on the left, sandboxed preview iframe (same
`sandbox="allow-scripts"`, opaque origin, render-origin CSP) on the right. The
page also hosts the same `__avState` bridge as the detail page, so artifact
state written in the preview persists.

**History.** Once the page is about an artifact, the chat has a History button.
It opens a pane that takes the place of the messages and the composer — a
conversation you are only reading must not sit above a box that would talk to
the live one — listing the conversations kept with the artifact, each with its
title, when it last ran, and the version it was last working against ("Based on
v4 · current" while the artifact is still at it). Selecting one shows it
read-only, in the same bubbles and tool chips the live chat draws. Both views
are server-rendered fragments (`/partials/agent-history`,
`/partials/agent-transcript`) swapped into `#history` by htmx; `agent.js` only
opens and closes the pane.

**The brief handoff (nw-d1dd).** A create session usually arrives from `/new`,
whose Build-with-agent panel is now the page's default and is a *form* rather
than a prompt box: a title and a description today, more answerable fields
later. Two things about how it gets here.

It travels in `sessionStorage` under `exhibit:agent-brief`, never a query
string. It is the user's own content, and a URL is copied into the service's
request log, the operator's proxy access log and the browser's history — the
same argument that put the SSE credential in a single-use ticket instead of a
URL (av-rgp1), applied to content rather than a secret. Same origin, same tab,
so nothing else is needed to carry it.

And it is consumed exactly once. `agent.js`'s boot reads it and removes it in
the same breath; a brief left behind would open the *next* session on a stale
description. It applies only in create mode — a modify session already has a
subject. The answered fields are flattened into the opening message by
`briefToMessage`, driven by the `BRIEF_FIELDS` list, so a field added to the
form is one line here and nothing else.

Clicking Start building sends it; the next page is not a second submit button.
The text is put in the composer *before* it goes, which is what makes the no-key
case behave. With no key stored, boot fills the composer, opens the key modal
and remembers that a brief is waiting; saving a key sends it. So the user gets
their words back rather than watching them vanish with the page they typed them
on, and the brief is still sent by the click that started it. `send()` reads the
composer rather than the stored brief, so an edit made while the modal was up is
what gets sent, and cancelling the modal leaves the text there to send by hand.

**Preview re-render (av-6m3e).** The preview pane is a server-rendered
fragment, not DOM the page script assembles. `agent.tmpl`'s `agentPreview`
partial renders the bar (title, Open/Details links, snippet button) and the
frame well; `GET /partials/agent-preview?artifact=<id>` serves that same
partial standalone. The pane carries the htmx wiring
(`hx-get`/`hx-trigger="exhibit:artifact-saved from:body"`/`hx-swap="innerHTML"`),
so a `create_artifact`/`write_artifact`/`edit_artifact` save travels
`Session.noteArtifactSaved` → `exhibit_artifact_saved` over SSE → `agent.js`
dispatches `exhibit:artifact-saved` → htmx fetches the fragment → the pane
swaps. A `set_state`/`delete_state` call (av-lvi1) drives the identical
`exhibit:artifact-saved` dispatch via `Session.noteStateChanged` →
`exhibit_state_changed`, reusing the same swap rather than inventing a second
refresh path — it's the only way the pane picks up state edits, since state is
inlined into the document at render time and the running iframe has no other
way to learn it changed. Two consequences to keep in mind when touching this
page:

- The fragment's iframe `src` carries a fresh per-render stamp. The render
  document is `Cache-Control: no-store`, but the browser only reloads a frame
  whose `src` actually changed — the stamp is what makes the new body appear.
- Each swap creates a *new* iframe element, so `agent.js` resolves `#pv-frame`
  on use (`previewFrame()`) rather than caching it; a cached reference would
  break the `__avState` bridge and snippet mode after the first save. A swap
  also drops snippet mode, since the picked document no longer exists.

htmx is vendored from `web/htmx/` into the embedded assets and served from the
app origin (`/assets/htmx/htmx.min.js`) — never a CDN, same rule as the
Phosphor icons.

## Snippet mode (element → agent context)

The render surface injects a second inert script beside the render preamble
(`internal/render/snippet.go`). The host page activates it via postMessage
(Snippet button or **Ctrl+Shift+S**); the user hover-highlights and clicks an
element inside the artifact. The script captures:

- a structural descriptor — CSS selector path, tag/id/classes, trimmed
  `outerHTML`, visible text, size — and
- a screenshot of just that element, rasterized *inside* the sandbox via SVG
  `foreignObject` → canvas (the opaque-origin iframe can screenshot its own
  DOM; the host can't reach into it), computed styles frozen inline.

Both are posted to the host pinned to the app origin, shown as a removable
chip on the composer, and attached to the next prompt: the screenshot as a
multimodal image (Pi RPC `prompt.images`), the descriptor as text. "I want
this button to be green" plus a snippet resolves to the exact element.

Only the app-origin host can activate the picker (origin-checked), and the
capture leaves the sandbox only as data posted to that host.

## Configuration

| Env | Meaning |
|-----|---------|
| `PI_BIN` | pi executable (default `pi`). If missing, the server boots and every agent route answers `503`: a broken install, not a configuration |
| `EXHIBIT_SECRET` | optional server secret for key encryption (else `data/secret.key` is generated) |
| `MOCK_LLM_URL` | dev/test only: enables the `exhibit-mock` provider pointing at `cmd/mockllm` |
| `AGENT_API_KEY` | the instance's own provider key — set it to enable platform mode; unset is BYOK |
| `AGENT_PROVIDER` | which provider that key is for; required with `AGENT_API_KEY`, and an unknown one fails at startup |
| `AGENT_MODEL` | optional model for platform sessions; the operator's choice, never surfaced |
| `GUARDRAIL_PROVIDER` / `GUARDRAIL_MODEL` / `GUARDRAIL_API_KEY` | the usage-policy guardrail's model and key; all three or none, and a partial set fails at startup |
| `AGENT_SPEND_CAP_OWNER_CENTS` | per-owner agent budget per calendar month (UTC); see the spend cap above |
| `AGENT_SPEND_CAP_SESSION_CENTS` | per-session agent ceiling |
| `AGENT_SPEND_CAP_INSTANCE_CENTS` | instance-wide agent ceiling per calendar month (guardrail spend included) |
| `AGENT_SPEND_CAP_TURN_SECONDS` | wall-clock ceiling per turn (0/unset is off) |

A platform credential with none of the `AGENT_SPEND_CAP_*` set fails at
startup (av-99f4).

`internal/mockllm` is a deterministic OpenAI-compatible chat-completions
handler — scripted create / update / re-read tool calls, color transforms,
the guardrail's screen (BLOCK for "mock-policy-violation", an unparseable
reply for "mock-guard-garbage", ALLOW otherwise),
snippet acknowledgment, a handful of literal state commands ("list state",
"set state K to V", "delete state K", "clear all state") mapped to the
matching state tool call, the widget-only branch, and a scripted *injected*
model that obeys an "also update artifact &lt;id&gt;" planted in untrusted data —
so the whole pipeline is testable without real provider credentials.
`cmd/mockllm` serves it as a standalone process for driving the surface by
hand; Go tests mount `mockllm.Handler()` on an httptest server and spawn a
real pi sidecar against it (`internal/api/agent_pipeline_test.go` and
`agent_platform_pipeline_test.go`, both skipped when `pi` is not installed).
`exhibit-mock` is a valid `AGENT_PROVIDER`, so platform mode is exercised end
to end with no real credential. The exhibit extension registers the provider only
when `MOCK_LLM_URL` is set.

## Extraction plan (epic `Exh-i0ll`)

The agent is a PoC guest in this repository, not a permanent resident. The
target shape is a **separate repository (`exhibit-agent`) holding a separate
Go service** that integrates with Exhibit exclusively through the HTTP API —
the same "optional satellite" standing as the thumbnail worker, just with a
UI. Steps, in order:

1. **Transcripts through the API** (`Exh-v6v4`): `Session.persistTranscript`
   currently calls `store.SaveTranscript` directly — the one write bypassing
   the HTTP API. Becomes `PUT /api/artifacts/:id/transcripts`; after this the
   agent has zero store access and is extractable. Authorize that write as
   the *service*, never with the session's scoped credential: a stored
   conversation is replayed into Pi when it is resumed (av-b4yh), so a session
   able to write its own history could plant instructions in a conversation the
   user later continues. Today the write is the manager's, server code that no
   model output reaches, and Pi alone writes the file.
2. **Exhibit-side seams** (`Exh-hz3g`): an `AGENT_URL` config that, when set,
   points the gallery "Agent" link and the detail-page "Modify with agent"
   action at the external agent UI; plus an additional configured embedder
   origin accepted by the render-surface bridges (snippet picker activation
   and postMessage target, `__avState` write bridge are today pinned to
   `APP_ORIGIN` — the agent service's chat page must be allowed to embed
   render iframes with both working).
3. **Extraction** (`Exh-k75k`): move `internal/agent` (sessions, pi sidecar,
   `ext/exhibit.ts`), `internal/secrets` + the `agent_keys` storage, the
   `/agent` chat UI, the agent API routes, and `internal/mockllm` +
   `cmd/mockllm` into the new repo. Config: `EXHIBIT_URL` + `EXHIBIT_TOKEN`,
   `PI_BIN`, its own port and secret. The browser talks only to the agent
   service, which **proxies** every Exhibit read/write through the API — no
   CORS opened on Exhibit, no token in page JS. Then delete the agent code
   from Exhibit core.

What stays in Exhibit, because it is genuinely core: the
`internal/agentscope` registry and the scope check in `authMiddleware`
(authorization is Exhibit's to keep, so an extracted agent service would
obtain a session credential from Exhibit rather than mint one), the
`agent_transcripts` table and its endpoints (artifact provenance belongs to
the artifact), and the snippet picker (`internal/render/snippet.go` — a
render-surface capability any embedding host can drive, not agent code).

## Known PoC limits

- One configured key per owner (not per provider); model list is a datalist
  hint, not validated against Pi's registry.
- Sessions are in-memory: a server restart drops live chats (transcripts
  already persisted survive).
- The snippet rasterizer is best-effort (bounded at 300 nodes / 2000px,
  degrades to descriptor-only on failure).
- No runtime allowlist-approval prompt in the chat (the artifact page's
  editor remains the approval surface; `exhibit-fr7` tracks the prompt).
