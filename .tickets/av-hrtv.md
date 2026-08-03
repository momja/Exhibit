---
id: av-hrtv
status: open
deps: []
links: [av-e0yj]
created: 2026-08-03T05:01:45Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent, api, allowlist]
---
# Agent body rewrites inherit prior network approvals and never surface the new footprint

PATCH /api/artifacts/:id deliberately keeps the artifact's existing decision='allow' rows when the body is rewritten (internal/api/artifacts.go:397-405). For a human edit that is right: the edit page re-runs the approval gate off network_footprint / footprint_changed in the response (:414-433). For an agent edit nothing re-runs it — the gate is UI-side, and the agent path has no UI in that loop.

Consequence: an artifact that was once approved for some origin keeps that approval across an unlimited number of agent rewrites of its source. The user approved origin X for the code they saw; the code is now different and X is still open. Combined with av-e0yj (the agent can be steered to rewrite an artifact by injected content) this is a working exfiltration channel — take data the model has in context, emit a body that POSTs it to the already-approved origin, save. The CSP does not stop it because the origin is genuinely on the allowlist.

It is also silent. internal/agent/ext/exhibit.ts:115-121 returns details WITHOUT a footprint field on update (create_artifact does include one, :89-96), so Session.noteArtifactSaved marshals footprint:null (internal/agent/agent.go:470-478) and the chat UI's "it references external origins ..." note is skipped (web/gallery/agent.js:287-289). A user watching the chat is told a new artifact wants the network; they are told nothing when a modified one does.

Adjacent correctness bug found while tracing this, same code path: update_artifact reads a.id / a.title from the PATCH response, but that response is updateArtifactResponse — {artifact:{...}, network_footprint, footprint_changed} (internal/api/artifacts.go:443-447). There is no top-level id or title, so both are undefined. The model is told 'Updated artifact undefined ("undefined")', details.artifactId is undefined, and noteArtifactSaved returns early on the empty id (agent.go:463-466) — so NO exhibit_artifact_saved event is emitted on update, and the htmx preview refresh (av-6m3e) never fires for a modification. Only the create path works. Nothing tests this: cmd/mockllm exercises update but no test asserts the event. Likely a contributor to Exh-m3bg.

## Design

Two separable fixes; do the enforcement one first.

Enforcement — an approval must not outlive the code it was granted for. When a body rewrite changes the footprint (footprint_changed is already computed, artifacts.go:422-424), the allow rows for origins that are no longer justified by the new body should be dropped, and origins the user has not seen must never be inherited. Simplest defensible rule: on any body rewrite, keep only the intersection of the prior allow set and the new scan, and re-gate anything else. That preserves the common "same origins, edited code" case with no prompting while closing "approved for the old body, reused by the new one". Decide whether an agent-originated rewrite should be stricter still (drop all allow rows and force re-approval) — the agent path has no human reading the diff, which is the difference that matters.

Reporting — make the update path say what the create path says: add footprint (and footprint_changed) to update_artifact's details in exhibit.ts, plumb them through noteArtifactSaved, and let the existing agent.js branch render the note. Fix the response-shape bug in the same change (read r.artifact.id / r.artifact.title) — until it is fixed, details.artifactId is undefined and the preview never refreshes after a modification.

## Acceptance Criteria

1. An artifact with origin X approved, rewritten through update_artifact into a body that contacts X, is NOT network-enabled for X without a fresh explicit approval when the footprint changed.
2. A body rewrite whose footprint is unchanged does not re-prompt and does not lose approvals (no regression on the existing edit flow).
3. update_artifact returns the artifact id, title, footprint, and footprint_changed; a test asserts the values are defined, not undefined.
4. A successful update emits exhibit_artifact_saved and the agent preview pane swaps — covered by a test on the update path, not only the create path.
5. The chat UI shows the pending-approval note after a modification introduces a new origin, as it already does after a creation.

