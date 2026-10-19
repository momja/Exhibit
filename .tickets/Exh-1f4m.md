---
id: Exh-1f4m
status: in_progress
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

