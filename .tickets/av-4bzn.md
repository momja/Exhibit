---
id: av-4bzn
status: open
deps: []
links: [av-wrbu]
created: 2026-08-03T05:02:38Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent, dos]
---
# Agent sessions have no resource bounds: unlimited subprocess spawn, unbounded prompts and event backlog

Every POST /api/agent/sessions forks a pi/node subprocess (internal/agent/agent.go:154-190) and there is no cap anywhere — not per owner, not per instance, no rate limit (internal/api/agent.go:142-200). Sessions are only released by an explicit close or by the idle reaper, which runs once a minute and skips anything mid-stream (agent.go:242-262, 30 minute default). N requests in a loop is N resident node processes for half an hour on a box whose whole design premise is "one small image, one process" (architecture.md §9). Node is not a cheap subprocess; a few hundred is an OOM or a fork failure, and a fork failure takes down the whole service, not just the agent surface.

Reachability is worse than "authenticated route" suggests: the token is embedded in the unauthenticated /agent page (internal/api/agentui.go:126, api.go:65-68, template agent.tmpl:85) — a project-wide pattern, not an agent bug, but it means anyone who can reach the app origin can read the token out of the HTML and then call this route.

Three more unbounded quantities on the same path:

- Prompt bodies are decoded with no limit (internal/api/agent.go:216) and images are passed through as base64 with no size or count check (:224-234), then written to the subprocess stdin and later persisted verbatim into agent_transcripts (agent.go:484-503). One request can commit an arbitrary blob to the database. (av-wrbu covers request-body policy service-wide; the transcript amplification is specific here.)
- readLoop uses ReadBytes on a bufio.Reader (agent.go:396-408), which grows without limit — a single unterminated line from the subprocess is held entirely in memory.
- Each session retains 4096 of those lines (maxBacklog, agent.go:288) with no byte budget, and every SSE subscriber gets a 1024-slot buffered channel (agent.go:296).

Neither HTTP server sets ReadTimeout/WriteTimeout (cmd/server/main.go:106-115), so long-lived connections to the SSE route are additionally free.

## Design

Bound each quantity at its own layer; none of these needs new architecture.

- Cap concurrent sessions per owner in Manager.Create (a small constant, e.g. 3) and reject over the cap with 429 plus a message the chat UI can show. Consider reaping the oldest idle session instead of refusing, so a stale tab does not lock the user out.
- Bound the prompt: a max message size, a max image count, and a max decoded image size, enforced in agentPrompt before anything reaches the subprocess. Validate the base64 and the declared mime type while there — im.MimeType is passed straight through today (internal/api/agent.go:228-233).
- Bound the reader: replace ReadBytes with a limited read (cap the line, drop or truncate over-long lines with a logged warning) so one bad line cannot be unbounded.
- Give the backlog a byte budget as well as a line count, and cap what a transcript may persist.
- Set ReadHeaderTimeout/WriteTimeout on both servers via explicit http.Server values; the SSE route needs a long or zero WriteTimeout, which is a good reason to construct the servers rather than use ListenAndServe.

Idle timeout of 30 minutes is the other half: with a per-owner cap it becomes a UX knob rather than the only backstop.

## Acceptance Criteria

1. Creating more than the configured number of concurrent sessions for one owner returns 429 and spawns no additional subprocess; a test asserts the process count does not grow.
2. Oversized prompts and oversized/too-many images are rejected with 400 before the subprocess is written to.
3. A subprocess line larger than the configured cap does not grow the reader unboundedly — covered by a test feeding an over-long line.
4. Per-session memory (backlog) has a byte ceiling, and a transcript write has a size ceiling.
5. Both HTTP servers are constructed with explicit timeouts, with the SSE route still able to stream indefinitely.
6. The chat UI surfaces the 429 as a readable message rather than a silent failure.

