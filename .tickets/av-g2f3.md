---
id: av-g2f3
status: open
deps: [av-zzbg]
links: []
created: 2026-07-22T03:17:44Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-xkq4
tags: [agent, api, snippet]
---
# Agent as a capture consumer: rewire agentui.go onto the capture client + describeSnapMetadata + prompt + docs

Parent epic: [[av-xkq4]]. Depends on [[av-zzbg]] (the reusable capture client) which
depends on [[av-c23y]] (the producer + protocol). This story makes the agent a **thin
consumer** of the capture client and adds the agent-specific presentation of the SDK
metadata — the actual UX win of the epic.

## Design

### 1. Rewire `internal/api/agentui.go` onto the capture client

- Delete the inlined capture machinery now living in the module ([[av-zzbg]]):
  `toggleSnippet`, the `captured`/`cancelled` message handler, `renderSnippetChips`,
  `clearSnippets`, and the raw `pendingSnippets` bookkeeping the client now owns.
- Load the client asset ([[av-zzbg]]) on the agent page and mount it:
  `const capture = createCaptureClient(frameEl, { appOrigin, chipContainer: document.getElementById('snippet-chips'), onCaptured, onCancelled });`
  Bind the existing UX: the Snip button (`#snip-btn`) and Ctrl+Shift+S call
  `capture.toggle()`; Esc calls `capture.deactivate()` (or `capture.bindKeys(document)`
  if that helper is provided).
- Keep the agent-only bits here: the "Snippet mode: drag a region…" system message, the
  `#snip-btn` disabled/active state tied to `showArtifact`, and focus handling.

### 2. Agent-specific metadata presentation

- Replace `describeSnippet(descriptor)` (~line 522) with `describeSnapMetadata(metadata)`
  producing a compact, agent-readable block from the SDK metadata:
  - page path (from `metadata.page`),
  - `frameworks`,
  - an elements list — selector · tag · component name · trimmed text — **capped**
    (the region can carry up to ~40 elements in the SDK's smallest-area-first order;
    render the top ~8),
  - `consoleErrors` if any (often the actual bug),
  - the trimmed `domSnippet`.
  Keep it terse. This is the main "far more context than a single element" win.
- `send()` (~line 493): the `images` mapping is unchanged (one PNG per capture →
  `{data, mime_type}`); the message text uses `describeSnapMetadata(capture.metadata)`
  over `capture.pending()`. Chips shown in the user bubble come from the capture the
  client handed over.

### 3. Agent system prompt `internal/agent/agent.go`

Update the snippet paragraph (~line 115) from "an attached screenshot plus an element
descriptor with selector/outerHTML" to reflect the region snap: a screenshot of a
selected region plus metadata listing the elements in it (selectors, component names),
detected frameworks, console errors, and a DOM snippet. Instruct the agent to locate the
intended element(s) via those selectors/component names and to treat console errors as
likely-relevant to the request.

### 4. Docs + tests

- **Docs (this PR):** `docs/agent.md` "Snippet mode" section — rewrite for the region
  snapper: what it captures now, the agent-side presentation, that the agent is one
  consumer of the shared capture client, and that click-to-pick is a future producer-side
  ([[av-c23y]]) addition needing no consumer change.
- **Tests:**
  - `internal/api` agentui test for `describeSnapMetadata` over a representative SDK
    metadata object (elements + frameworks + a console error) asserting the compact text
    shape and the element cap.
  - The mock-LLM E2E path (`cmd/mockllm`, which already scripts snippet acknowledgment)
    drives an artifact edit end-to-end with the new region payload (`verify` skill /
    `docs/agent.md` §Configuration harness). Confirm images + metadata reach the prompt.

## Acceptance Criteria

- In the agent preview (`/agent?artifact=<id>`), activating snippet mode (Snip button /
  Ctrl+Shift+S) shows the SDK drag-select overlay; dragging a region attaches a chip with
  a cropped region screenshot; the next prompt carries that screenshot as a multimodal
  image plus a compact metadata block (elements + selectors + component names,
  frameworks, console errors, DOM snippet). Esc cancels cleanly.
- `agentui.go` no longer contains the capture machinery moved to [[av-zzbg]]; it *uses*
  `createCaptureClient` and only adds agent-specific presentation (`describeSnapMetadata`)
  and prompt assembly.
- `go build ./...` + `go test ./...` (after `make assets`) pass; the mock-LLM flow drives
  an artifact edit via a region snap end-to-end.
- `docs/agent.md` updated in this PR; the future click-to-pick addition is noted as
  riding the same producer/contract with no consumer change.

## Notes

- Branch per project rules via a supacode worktree, e.g.
  `feature/av-g2f3/agent-capture-consumer`; never develop on `main`.
- This story proves the epic's boundary: the agent is now just a subscriber to
  `onCaptured` plus its own prompt formatting — swapping drag-select for click-to-pick
  later touches none of this file.
