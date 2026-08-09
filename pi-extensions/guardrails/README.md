# pi-guardrails

A [pi](https://github.com/earendil-works/pi-mono) extension that gates **user prompts** before
they reach the coding agent. A supplemental — deliberately *small* — LLM acts as a gatekeeper:
it reads the recent conversation plus the new prompt and returns a strict JSON verdict
(`allow` / `warn` / `block`). This catches:

- **Prompt injection / instruction-override** — "ignore previous instructions", "you are now an
  unfiltered model", "show your system prompt", "developer message: …"
- **Off-topic steering** — prompts that clearly abandon the ongoing project work
- **Disallowed / explicit topics** — a configurable policy (sexual content, illegal activity,
  harassment, …)

The prompt never reaches the main agent when it is blocked; blocked and warned prompts are
recorded as audit entries in the session.

## How it works

```
user prompt ──► input event ──► prefilter (regex, ~0ms)
                                  │
                   no match ──────┴─────────► sent to agent (unchanged)
                                  │ match (or check=always)
                                  ▼
                        guardrail LLM (small model)
                                  │
              allow ─────────────► sent to agent
              warn ──────────────► sent to agent + warning toast
              block ─────────────► REJECTED + error toast + audit entry
```

Two layers keep the common case fast:

1. **Regex prefilter** (`check: "heuristic"`, the default). Injection patterns and per-category
   topic patterns are matched instantly. No match → the LLM is never consulted, so normal prompts
   add ~zero latency. A match → the prompt is routed to the guardrail LLM for a nuanced verdict.
   With `prefilter.blockDirect: true` a match blocks immediately without the LLM.
2. **Guardrail LLM** (`check: "always"` forces it for every prompt). A small/cheap model (see
   *Model selection*) judges the prompt against the policy and replies with one JSON object. The
   guardrail's own system prompt explicitly classifies attempts to override *its* instructions as
   an `injection` violation, and the user content is sandboxed in `<user_prompt>` tags — so
   "ignore previous instructions" is self-incriminating rather than effective.

## Installation

The canonical source lives in `pi-extensions/guardrails/` (version-controlled). It is
linked into both extension discovery locations (either works; `/reload` picks up changes):

```bash
# project-local (loads after the project is trusted):
ln -sfn ../../pi-extensions/guardrails .pi/extensions/guardrails

# global (all projects):
ln -sfn /path/to/pi-extensions/guardrails ~/.pi/agent/extensions/guardrails
# or copy:
# cp -r pi-extensions/guardrails ~/.pi/agent/extensions/
```

Dependencies are provided by pi itself (`@earendil-works/pi-coding-agent`,
`@earendil-works/pi-ai`, `@earendil-works/pi-tui`), so no `npm install` is needed.

## Model selection

The guardrail reuses providers already configured in pi — no extra API key needed.

1. `model: "provider/id"` in config (e.g. `"openai/gpt-4.1-mini"`) — used if found.
2. Otherwise a known-cheap model is looked up (Haiku, GPT-4.1-mini, Gemini Flash, …).
3. Otherwise the cheapest available text-capable model in the registry wins
   (non-reasoning preferred).

Check what was picked with `/guardrails`.

## Configuration

Config files are merged in order (later wins):

| Source | Path |
|---|---|
| Defaults | (built in) |
| Explicit | `$PI_GUARDRAILS_CONFIG` |
| Global | `~/.pi/agent/guardrails.json` |
| Project | `.pi/guardrails.json` (only when the project is trusted) |

Environment overrides: `PI_GUARDRAILS_ENABLED`, `PI_GUARDRAILS_MODEL`,
`PI_GUARDRAILS_MODE`, `PI_GUARDRAILS_CHECK`.

See [`guardrails.example.json`](./guardrails.example.json) for the full documented shape.
Key options:

| Option | Default | Meaning |
|---|---|---|
| `enabled` | `true` | Master switch |
| `mode` | `"block"` | `block` = reject; `warn` = allow but warn; `off` = disabled |
| `check` | `"heuristic"` | `always` / `heuristic` (prefilter → LLM) / `off` |
| `model` | `""` | `"provider/id"`; empty = auto |
| `prefilter.injectionPatterns` | built-in set | Regexes for instruction-override attempts |
| `prefilter.topicPatterns` | `explicit` set | `{ category: [regexes] }` routed to the LLM |
| `prefilter.blockDirect` | `false` | Match blocks instantly, no LLM |
| `policy.allowedTopics` | `[]` | Topics never flagged (e.g. project-specific areas) |
| `policy.disallowedTopics` | built-in set | Topics always blocked |
| `policy.offTopicHandling` | `"warn"` | Off-topic prompts: `allow` / `warn` / `block` |
| `policy.extraInstructions` | off-topic guidance | Extra natural-language policy |
| `context.recentMessages` / `maxChars` | `8` / `6000` | Conversation context budget |
| `failClosed` | `false` | `true` = block when the guardrail LLM is unreachable |
| `logAll` | `false` | Audit every verdict, not just non-`allow` ones |

## Commands

- `/guardrails` — status, active config sources, recent verdicts
- `/guardrails test <text>` — run a prompt through the guardrail without sending it (calibrate policy)
- `/guardrails toggle` — enable/disable at runtime
- `/guardrails reload` — reload config from disk

## Auditing

Every non-`allow` verdict (or all, with `logAll`) is written with
`pi.appendEntry("guardrails", …)`: timestamp, verdict, severity, category, reason, prompt
preview, model, source. Entries render in the transcript as a compact card and **do not**
participate in the LLM context, so they add no tokens.

## Notes & limitations

- The guardrail evaluates the raw input *before* skill/template expansion. Inputs starting with
  `/` (commands, `/skill:`, `/template`) are skipped by default (`skipSlashCommands`).
- Extension-injected messages (e.g. the agent's own follow-ups) are skipped by default
  (`skipExtensionSources`).
- On guardrail LLM failure the default is **fail-open** (prompt passes, with a warning) so a
  transient outage never locks you out. `failClosed: true` inverts this.
- The guardrail model can be the same provider as your main model; only extra latency
  distinguishes them. Prefer a small/cheap model.
- This guards the *human* typing prompts into pi. Tool-call arguments from the agent itself are
  not gated — the gatekeeper sits at the input boundary by design.
- Off-topic classification happens only when the guardrail LLM is consulted (`check: "always"`
  or a prefilter hit). With the default `heuristic` check, add an `offtopic` pattern to
  `prefilter.topicPatterns` or switch to `always` if off-topic steering is a real concern.

## Testing

```bash
# unit tests for config/prefilter/verdict parsing (loads the extension runtime):
pi -e pi-extensions/guardrails/self-test.ts -p "x"

# RPC end-to-end smoke test (benign pass, injection block, guardrails test, status):
node pi-extensions/guardrails/rpc-e2e.mjs

# trace guardrail decisions on every prompt:
PI_GUARDRAILS_DEBUG=1 pi
```
