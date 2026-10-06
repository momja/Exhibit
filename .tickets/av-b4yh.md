---
id: av-b4yh
status: closed
deps: [av-bavj, av-5s7g, av-y7td]
links: [av-bavj, av-5s7g, av-y7td, Exh-v6v4]
created: 2026-10-03T06:30:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, versions]
---
# Continue a past conversation, with a confirmed rollback and an escape

A conversation kept with an artifact ([[av-y7td]]) can be picked back up from the chat window. It is tied to the artifact version it was last working against, so continuing it is a decision about the artifact: roll back to that version first, or leave the artifact as it is.

Last of the stack: [[av-bavj]] (versions), [[av-5s7g]] (tool-only context), [[av-y7td]] (the record).

## Design

- `POST /api/agent/sessions` takes `resume_session_id`. The manager writes the stored session file into a fresh scratch directory and starts Pi from it (`--session`). The session keeps the conversation's id, so the same record keeps growing and the versions it writes carry the same session id.
- Pi sends the prompt it was spawned with rather than the one in the file, and takes `--provider`/`--model` from the command line over the file's recorded model, so the instructions and the owner's current key are in force. Measured against Pi 0.87.1, not assumed.
- The first prompt after a resume carries one fixed sentence: the artifact may have changed, read it again with `get_artifact` before changing anything. Sent whether or not the artifact was rolled back.
- A conversation that is already running is attached to (200), not started twice. Not the caller's or not there is a 404; kept before session files existed is a 409.
- Rolling back is the page's decision, carried out through the ordinary version restore. Under the transcript, a card puts the choice: when the artifact has moved on, **Roll back to vN and continue** is the primary button and states what it does beside it (restore as a new version, nothing lost, undoable from Versions); **Continue without rolling back** is a plain secondary button and never the primary one; Cancel returns to the list. When the artifact has not moved, one button. The page starts the conversation first and rolls back second, so a refusal costs nothing, and a failed rollback closes the session it started.

## Acceptance Criteria

- A resumed conversation hands the model its earlier turns under the current system prompt (exactly one), with the re-read sentence on the first prompt only (`TestAResumedConversationHasItsHistoryAndIsToldToReadAgain`, against a real Pi).
- The conversation stays one record: same id, title kept, version moves to what the resumed turns left behind.
- Without a rollback a person's later edit survives the resumed turn; after a rollback the agent builds on the restored version (`TestContinuingWithoutRollingBackKeepsWhatChangedSince`, `TestContinuingAfterARollbackBuildsOnTheRestoredVersion`).
- The page never rolls back without the card's explicit button, never makes the escape the default, and a refused start leaves the artifact untouched (`agent.history.test.mjs`).

## Notes

Not done, deliberately: a diff of the artifact between the conversation's version and now, previewing the old version before choosing, deleting a conversation, and counting conversations in storage accounting. The first two would make the choice better informed; the card shows the version numbers and the Versions list is one click away.
