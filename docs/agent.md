# Exhibit — Agent Integration (Pi harness)

Proof-of-concept build-and-modify-with-AI surface (epic `Exh-yvhp`, grown out of
`av-q3wo`). A chat page lets the user create new artifacts and modify existing
ones through an LLM agent, using **their own API key**, with everything the
agent saves flowing through the normal ingest path.

## How it fits the architecture

Pi (`pi-mono`, Mario Zechner's agent harness) runs as a **sidecar subprocess**,
one per chat session, spawned by the Go service as
`pi --mode rpc --no-session --no-builtin-tools -e exhibit.ts` — the same
"optional satellite" pattern as the thumbnail worker (architecture §3.6). The
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
`create_artifact` / `update_artifact` / `get_artifact` for the document,
`get_state` / `set_state` / `delete_state` for the artifact's stored state
(av-lvi1), and `set_widget` / `get_widget` for the artifact's gallery tile
(av-fafu) — all registered by a Pi extension (`internal/agent/ext/exhibit.ts`,
materialized to the data dir at startup) that calls back into the exhibit HTTP
API. Agent output is scanned like any other ingest; scanned origins are
**never** auto-approved — the chat UI tells the user when a saved artifact has
a network footprint awaiting approval.

## Scope: one session, one artifact

A session reaches exactly one artifact, and that is enforced twice — once for
ergonomics, once for real. `security.md` §5 is the full statement; the shape:

- **No tool takes an artifact id.** `create_artifact(title, body)`,
  `update_artifact(body[, title])`, `get_artifact()`, `get_state()`,
  `set_state(key, value)`, `delete_state([key])`, `set_widget(body)`,
  `get_widget()`. A tool with no id parameter cannot be talked into a
  different target, which matters because artifact bodies and titles are
  untrusted text that reaches the model's context. The extension resolves the
  target from `EXHIBIT_ARTIFACT_ID`, or from the API's response to the first
  create.
- **The API refuses anything outside the scope.** The sidecar authenticates
  with a per-session credential (`internal/agentscope`) that resolves to
  (owner, artifact), not the service token. `authMiddleware` allows exactly
  `POST /api/artifacts` while unbound, plus `GET`/`PATCH` on the session's own
  artifact and the `state` / `widget` sub-resources of that same artifact —
  one allowlist entry per tool above. Every other route — the BYO provider
  key, shares, deletes, tags, collections, transcripts, `widget/generate`, and
  every other artifact in the library — is a 403 before any handler runs.
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
  No artifact title, body, or id is interpolated into it.
- The artifact's **current source is inlined** into the session's opening
  user-role message, so a modify session does not spend a tool call reading
  what the server was already holding. `get_artifact` stays registered for the
  re-read after the agent's own save or a concurrent human edit.
- The source, the title, and any snippet descriptor arrive inside a fenced
  block whose delimiter carries a **per-session random nonce**
  (`-----BEGIN EXHIBIT UNTRUSTED DATA <nonce>-----`), so injected text cannot
  close the fence and pose as an instruction. The system prompt states the
  contract; the nonce is redacted from block content. `get_artifact` and
  `get_widget` return their bodies through the same fence.
- Snippet descriptors reach the API as their own `snippets` field on the
  prompt request, not concatenated into `message` — page JS never composes the
  envelope, and never needs the nonce.

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
model to save with `update_artifact` — exactly what a generate-the-tile session
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
| `EXHIBIT_DATA_NONCE` | fence id for untrusted tool output |
| `EXHIBIT_SESSION_ID` | this session's id |

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
- Pi's own identifiers are stripped from the event stream and the persisted
  transcript (below).
- Availability is a separate, unchanged signal: a missing `pi` binary still
  disables the surface in either mode.

### What Pi emits, and what is filtered

Every assistant message Pi emits carries the model's identity, and both of this
service's publishing seams pass Pi's protocol through verbatim — the SSE
broadcast and `agent_transcripts.messages`. Captured from a real
`pi --mode rpc` turn (v0.84.1):

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
argument is left alone — at both seams. BYOK is unfiltered: there the
identifiers describe a key the caller typed.

The `usage` block beside them (token counts and cost) is deliberately kept: it
names no model, and it is what metering will read (av-hyo6).

### No spend cap

Platform mode makes every session bill the instance's provider account with
nothing bounding it — `internal/agent` reads no token usage off Pi's stream, so
an instance can neither attribute spend to an owner nor stop a session that
runs away. The startup log says so. Metering and a per-owner budget are
av-hyo6; until they exist, do not put a platform-mode instance in front of
untrusted signups.

## Sessions, streaming, transcripts

- `POST /api/agent/sessions` (optional `artifact_id` scopes the session to an
  existing artifact for modify mode, and inlines its source into the opening
  message), `POST …/prompt` (`message` + optional base64 `images` + optional
  `snippets`, the element descriptors the server fences as data),
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
  an API-group request would carry, so the owner check every other session
  route makes applies here too. Because a ticket is spent on connect, the chat
  page drives its own reconnect (mint, then reconnect, with backoff) instead of
  relying on EventSource's automatic retry; the session's event backlog replays
  on subscribe, so nothing is lost. An agent session's own scoped credential is
  accepted for neither: it is not a page credential.

- `internal/agent` tracks streaming state (prompts sent mid-stream become Pi
  steering messages), keeps an event backlog for late subscribers, reaps idle
  sessions, and on every settled turn persists the full Pi message list to
  `agent_transcripts` keyed by artifact — colophon-style provenance
  (`GET /api/artifacts/:id/transcripts`), the foundation for future remixing.
- When a save-tool call succeeds, the session emits a synthetic
  `exhibit_artifact_saved` event; a `set_state`/`delete_state` call emits the
  analogous `exhibit_state_changed` event, and `set_widget` the
  `exhibit_widget_saved` one. All three name the session's own artifact, read
  from the credential's scope rather than from the tool result. The chat UI
  uses any of them to re-render the live preview (see below).

## Chat UI

`GET /agent` (create) and `GET /agent?artifact=<id>` (modify; also linked from
the artifact detail toolbar as "Modify with agent"). Server-rendered like the
gallery: chat + streaming on the left, sandboxed preview iframe (same
`sandbox="allow-scripts"`, opaque origin, render-origin CSP) on the right. The
page also hosts the same `__avState` bridge as the detail page, so artifact
state written in the preview persists.

**Preview re-render (av-6m3e).** The preview pane is a server-rendered
fragment, not DOM the page script assembles. `agent.tmpl`'s `agentPreview`
partial renders the bar (title, Open/Details links, snippet button) and the
frame well; `GET /partials/agent-preview?artifact=<id>` serves that same
partial standalone. The pane carries the htmx wiring
(`hx-get`/`hx-trigger="exhibit:artifact-saved from:body"`/`hx-swap="innerHTML"`),
so a `create_artifact`/`update_artifact` save travels
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
| `PI_BIN` | pi executable (default `pi`; agent surface disabled if missing) |
| `EXHIBIT_SECRET` | optional server secret for key encryption (else `data/secret.key` is generated) |
| `MOCK_LLM_URL` | dev/test only: enables the `exhibit-mock` provider pointing at `cmd/mockllm` |
| `AGENT_API_KEY` | the instance's own provider key — set it to enable platform mode; unset is BYOK |
| `AGENT_PROVIDER` | which provider that key is for; required with `AGENT_API_KEY`, and an unknown one fails at startup |
| `AGENT_MODEL` | optional model for platform sessions; the operator's choice, never surfaced |

`internal/mockllm` is a deterministic OpenAI-compatible chat-completions
handler — scripted create / update / re-read tool calls, color transforms,
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
   agent has zero store access and is extractable.
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
