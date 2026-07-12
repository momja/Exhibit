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
`create_artifact` / `update_artifact` / `get_artifact`, registered by a Pi
extension (`internal/agent/ext/exhibit.ts`, materialized to the data dir at
startup) that calls back into the exhibit HTTP API with the service token.
Agent output is scanned like any other ingest; scanned origins are **never**
auto-approved — the chat UI tells the user when a saved artifact has a network
footprint awaiting approval.

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

Supported providers: Anthropic, OpenAI, Google Gemini, OpenRouter, OpenCode
Go, plus `exhibit-mock` when `MOCK_LLM_URL` is set.

## Sessions, streaming, transcripts

- `POST /api/agent/sessions` (optional `artifact_id` binds the session to an
  existing artifact for modify mode), `POST …/prompt` (message + optional
  base64 images), `POST …/abort`, `DELETE …`.
- `GET /api/agent/sessions/:id/events` — SSE. EventSource can't set headers,
  so this one route authenticates the same bearer token via `?token=`.
- `internal/agent` tracks streaming state (prompts sent mid-stream become Pi
  steering messages), keeps an event backlog for late subscribers, reaps idle
  sessions, and on every settled turn persists the full Pi message list to
  `agent_transcripts` keyed by artifact — colophon-style provenance
  (`GET /api/artifacts/:id/transcripts`), the foundation for future remixing.
- When a save-tool call succeeds, the session emits a synthetic
  `exhibit_artifact_saved` event; the chat UI uses it to load/reload the live
  preview iframe.

## Chat UI

`GET /agent` (create) and `GET /agent?artifact=<id>` (modify; also linked from
the artifact detail toolbar as "Modify with agent"). Server-rendered like the
gallery: chat + streaming on the left, sandboxed preview iframe (same
`sandbox="allow-scripts"`, opaque origin, render-origin CSP) on the right. The
page also hosts the same `__avState` bridge as the detail page, so artifact
state written in the preview persists.

## Snippet mode (element → agent context)

The render surface injects a second inert script beside the storage shim
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

`cmd/mockllm` is a deterministic OpenAI-compatible server (scripted
create → read → update tool calls, color transforms, snippet acknowledgment)
so the whole pipeline is testable end to end without real provider
credentials; the exhibit extension registers it as a Pi custom provider only
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
   `/agent` chat UI, the agent API routes, and `cmd/mockllm` into the new
   repo. Config: `EXHIBIT_URL` + `EXHIBIT_TOKEN`, `PI_BIN`, its own port and
   secret. The browser talks only to the agent service, which **proxies**
   every Exhibit read/write through the API with its token — no CORS opened
   on Exhibit, no token in page JS. Then delete the agent code from Exhibit
   core.

What stays in Exhibit, because it is genuinely core: the
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
