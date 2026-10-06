---
id: av-y7td
status: closed
deps: [av-bavj, av-5s7g]
links: [av-bavj, av-5s7g, av-b4yh, Exh-v6v4]
created: 2026-10-03T06:20:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, versions]
---
# Agent conversations are kept as Pi records them, tied to an artifact version

An agent conversation is stored as Pi's own session file and tied to the artifact version it was last working against, and the chat page can list the conversations kept with an artifact and read one back.

Part of a stack with [[av-bavj]] (versions), [[av-5s7g]] (tool-only context) and [[av-b4yh]] (resume). This is the record; [[av-b4yh]] makes it resumable.

## Design

- Sessions run with `--session <workDir>/session.jsonl` instead of `--no-session`. On every `agent_settled` the service reads the file back and stores it in the conversation's `agent_transcripts` row (migration 032: `title`, `session_file`, `version_seq`). Pi's file is the format `pi --session` starts from, so storing it as it is is what makes a conversation resumable; nothing is translated.
- `version_seq` is the artifact's head version when the file is stored, read in the same statement as the upsert, so the pair cannot disagree. A conversation is one row that grows, not one row per turn.
- A session file holds the system prompt, every tool result in full, reasoning and the model's identity, so none of it leaves the server. `agent.Messages` projects what a person saw (what they said, what the assistant said, which tools it used) from named fields.
- Rows kept before this hold a message dump and no file. They list and read like the rest (`resumable: false`, `version_seq: 0`) and can only be read. `session_file` is NULL for them rather than '' so that "has a file" is answered from the record header (`typeof`) without reading a value that can be megabytes.
- `GET /api/artifacts/:id/transcripts` (summaries + `head_seq`) and `.../:sessionID` (one, with messages). Owner-only; absent from `agentSubResources`, so an agent session cannot read the conversations before it.
- Chat window: a History button (once there is an artifact) opens a pane that replaces the messages and composer; the list and the read-only view are server-rendered htmx fragments.
- Every session runs from `/`. A session file records the directory it ran in and Pi refuses to resume one whose directory is gone ("Stored session working directory does not exist"), which a per-session directory never survives. The scratch directory (HOME, the file, `selection.json`) is removed once the process has exited, its output has been read and the last settled turn is stored: the conversation's home is the database, and a copy left on disk would outlive the account that owned it.
- Pi is pinned to 0.87.1 in the Dockerfile, because its file format, `--session` and `agent_settled` are now part of this service's contract.

## Acceptance Criteria

- After a settled turn the conversation is stored with the artifact's head version and a title; further turns update the same record (`TestAConversationIsKeptWithTheVersionItLeftBehind`, `TestAConversationIsKeptInPlaceAsItGoesOn`, against a real Pi).
- The API and the page return only the projection: no system prompt, tool results, arguments, provider, model or working directory, including in platform mode (`TestPlatformModeStreamAndTranscriptNameNoModel`).
- The stored file's header records `/`, and a session's scratch directory is gone once it ends (`TestASessionsScratchDirectoryGoesWhenItEnds`).
- Another owner's artifact reads as having no conversations; an agent credential is refused.
- The history pane lists conversations with the version each was working against and whether the artifact has moved on, and shows one read-only.

## Notes

The file grows with the artifact as well as the talk, since each read of the artifact is a full copy in it. A conversation over 64 MiB stops being kept (logged); the live session is unaffected. Kept conversations are not counted in storage accounting and there is no way yet to delete one other than deleting the artifact.

Persistence still writes through the store directly, as before; [[Exh-v6v4]] (through the API) is unchanged by this and now carries a note on how it must be authorized.
