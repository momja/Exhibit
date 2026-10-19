---
id: Exh-1f4m
status: closed
deps: [Exh-xco9, av-n0gl, Exh-dgno]
links: []
created: 2026-10-19T05:12:10Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-099b
tags: [agent]
---
# Eagerly prepare the agent session and artifact context after deliberate typing

After a few non-whitespace characters remain for a short debounce (suggested three characters and 300 ms), prepare one session and prefetch source. Reuse in-flight startup on Send; expire abandoned preparations and handle paste, navigation, errors, session switching and stale revisions. Include a valid synthetic assistant get_artifact call and matching real tool-result source before the first model request, without a separate inference call. Preserve structured roles, scope, history/resume/compaction, transcript integrity and request identity. No nonce-fenced source in user/system messages.

## Acceptance Criteria

Deliberate typing starts at most one preparation; Send reuses it; first provider request contains current source as tool-result data and can edit immediately; stale snapshots refresh or conflict safely; no draft text is sent to a model; meaningful lifecycle, security and real-Pi tests pass.


## Notes

**2026-10-19T05:38:21Z**

Implemented eager browser preparation (three non-whitespace characters / 300ms), single-flight Send claim, expiry, key/history/navigation invalidation, late-response cleanup, accepted text/selection capture, stale SSE protection, and native ES module conversion with template bridges.

Source preparation uses the shared readArtifact authorized loader and artifactSourceSnapshot formatter. Pi custom DATA entries persist source reads and the context hook projects synthetic assistant get_artifact calls plus matching tool results after input acceptance. Revision validation refreshes external edits, fallback preserves ordinary get_artifact, retained known source avoids duplicate reads/projections, and resume/compaction preserve roles and truthful transcript labels. Updated resumedNotice to use fresh preparation or explicitly read only when unavailable.

Server preparations expire after 90 seconds independently of browser lifetime and normal idle timeout. In-flight prompt admission is protected at expiry; accepted prompts/agent_start claim normal lifetime, guard refusals never renew the deadline.

Parent integration: keep readArtifact free of cache adoption. adoptPreparedSource(snapshot) must rememberSource(snapshot.artifact), seed widget revision, and retain latest metadata/asset summary. Supply current() returning artifactSourceSnapshot using the edit cache's advanced body/revision and latest metadata/asset summary; return null after invalidation. Successful saves advance that shared known snapshot. No actual edit-tool behavior changed here.

Validation: sh scripts/build-assets.sh; final go test ./... with real Pi 1.3.0; go test -race ./internal/agent; latest real-Pi lifecycle/materialization and extension suite; 45 browser tests; 11 source-context tests; git diff --check. Real Pi verifies no provider request during preparation, first-request source only in matching tool results, stale source refresh, compaction, and current/legacy resume. No push or PR; parent owns stack assembly.
