---
id: Exh-13k0
status: in_progress
deps: [Exh-prp1]
links: []
created: 2026-11-05T03:49:01Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-ivro
tags: [bazarre, remix]
---
# Reapply remix customizations on upstream update candidates

Remix is the default store action. Upstream updates to customized artifacts create a separate private candidate from the new upstream release. Current artifact, saved state, shares, conversations and connections remain untouched. The user reviews/edits an ordered customization brief; only deliberate user instructions are eligible, never automatic transcript/tool-output replay. Candidate records its parent copy, exact local baseline and target release digest; starts with no state/permissions/connections. Agent opens with the brief as an editable unsent user draft (not system instructions) and requires normal user Send. No automatic model run on discovery/GET/import. Candidate and original are separately accessible and the user explicitly chooses which to keep; nondeterministic results are not represented as guaranteed equivalent. A clean-copy explicit in-place update can remain a secondary path, guarded by baseline compare and confirmation. Candidate review must show old/current/upstream versions and preserve local artifact without any replacement. Persist a user-approved ordered customization brief locally for reuse on later candidates; never export it. An optional user-message-history suggestion must be explicitly selected and reviewed; no automatic complete conversation replay.

## Acceptance Criteria

Candidate creation idempotent/owner scoped; original unchanged even if agent fails; user-approved brief stored locally and prefilled but unsent; later candidates reuse ordered brief; exact baseline/digest binding; candidate has no saved state or approvals; tests cover local divergence, candidate failure/retry, ownership, no model execution and repeat updates.

