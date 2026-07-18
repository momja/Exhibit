---
id: av-q3wo
status: open
deps: []
links: [av-q30x, Exh-yvhp]
created: 2026-07-06T22:03:48Z
type: epic
priority: 2
assignee: Max Omdal
---
# Add support for agents with BYO API key using Pi harness for building artifacts directly in the tool with eventual goals to support artifact remixing and refactoring

Pi is Mario Zechner's agent harness (pi-mono; also used by OpenClaw). It runs as a **separate sidecar process** that interoperates with the exhibit service — same pattern as the thumbnail worker: an optional satellite composed around the core image, never inside the single Go binary (architecture §3.6). BYO API key means the user supplies their own OpenAI-compatible key.

## Scope decisions

- **MVP: create a new artifact from a prompt.** A build-a-tool chat surface whose output enters the library through the normal ingest path — scan, footprint approval, single write path. No special-cased writes.
- **Conversation transcript is persisted with the artifact** (colophon-style provenance, like tools.simonwillison.net). This is the foundation for the eventual remix/refactor goals: remixing needs the history, so capture it from day one even though remix UI is out of scope.
- **Key handling: server-side, encrypted at rest, configured per user.** The key is used by exhibit (the sidecar), never by artifact code, and never exposed to page JS after entry. XSS exposure is bounded: the artifact sandbox already can't reach the app origin; the realistic surface is the key at rest on the server, hence encryption.

## Open questions

- Per-user key storage has no home yet: the schema has no multi-tenant concept beyond `owner_id` and no row-level security. Key storage should attach to the auth/user seam (see linked av-q30x) — likely a `user_settings`-style table keyed by owner_id, encrypted with a server secret.
- Whether Pi supports a long-lived server mode with multiple concurrent sessions, and what the exhibit↔Pi API contract looks like (spawn-per-job vs persistent process). Research established patterns for multi-user BYO-key agent backends (OpenWebUI/LibreChat are prior art) before inventing one.
- Where the chat UI lives (gallery page vs dedicated route) and how streaming output reaches the browser through the Go service.


## Notes

**2026-07-11T16:56:40Z**

PoC of this epic's MVP scope (and beyond: modify flow + snippet element-context) landed under Exh-yvhp on branch feature/Exh-yvhp/pi-agent-poc. Answers the open questions: pi supports a long-lived RPC server mode (JSONL over stdio, spawn-per-session); exhibit<->pi contract is the RPC protocol + a TS extension whose tools call the exhibit API; key storage landed as agent_keys keyed by owner_id, AES-256-GCM under a server secret (EXHIBIT_SECRET or generated data/secret.key); chat UI is a dedicated /agent route with SSE streaming through the Go service.

## Complexity

XL
