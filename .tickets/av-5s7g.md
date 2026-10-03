---
id: av-5s7g
status: closed
deps: [av-bavj]
links: [av-bavj, av-e0yj, av-y7td, av-b4yh]
created: 2026-10-03T06:10:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, security]
---
# Agent context: untrusted text reaches the model only as a tool result

A session no longer inlines the artifact into a prompt behind a nonce-fenced block. The model reads the artifact with `get_artifact`; the elements the user selected arrive through a new `get_selection` tool; both come back as tool results. The system prompt stays static and says that tool results are data.

Part of a stack with [[av-bavj]] (versions), [[av-y7td]] (chat record) and [[av-b4yh]] (resume). It is what makes a stored conversation resumable: a nonce belongs to one process and means nothing to the next one, while a tool result is a message role every provider's API already separates from instructions. Builds on the containment of [[av-e0yj]], which it does not touch.

## Design

- No per-session secret: `EXHIBIT_DATA_NONCE`, the fence markers, the redaction pass and `composePrompt` are gone. `buildSystemPrompt(override, opts)` is a pure function of the mode.
- The model is not handed the artifact. Every mode that has one says to read it with `get_artifact` first, and again whenever it may have changed (after its own save, or because the user edited it elsewhere).
- `get_artifact` returns the title as one bounded line (newlines flattened, 200 characters), and a save's result no longer reads the stored title back. The title is the most attacker-controllable field on a URL-ingested artifact.
- Selected elements: the prompt request still carries them as `snippets`. The session writes them to `selection.json` in its work directory (replaced each prompt, removed when none) and the prompt gains one fixed sentence; the extension reads the file at `EXHIBIT_SELECTION_FILE`.
- The mock LLM follows the same script a real model would: `get_artifact`, then `write_artifact`; `get_selection` first when the prompt says elements were selected.

## Acceptance Criteria

- The system prompt and the user message contain no artifact text, in any mode; the source, title and selection are tool results (asserted against a real Pi sidecar by `TestAgentSessionReadsTheArtifactItself`, `TestAgentSelectionReachesTheModelAsAToolResult`, and the hostile-title test).
- A hostile title that names another artifact still cannot move the session off its own artifact.
- `docs/security.md` §5.2, `docs/agent.md` and `architecture.md` §3.7 describe the tool-result boundary.

## Notes

Costs one tool call at the start of a turn that edits. Prompt caching makes the repeated read cheap on the provider side, and the extra round trip is the price of the boundary being structural.
