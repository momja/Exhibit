---
id: av-l31x
status: closed
deps: []
links: []
created: 2026-08-10T05:40:50Z
type: bug
priority: 1
assignee: Max Omdal
tags: [agent, ui, htmx, backend]
---
# update_artifact does not trigger the agent preview's htmx partial reload

In the agent view (/agent), a successful update_artifact tool call leaves the preview pane stale: the tool chip completes, the artifact IS persisted server-side, but the preview iframe keeps showing the old body until a manual reload. The chat even shows the tell-tale result text 'Updated artifact undefined ("undefined").'

Expected (same path create_artifact takes, av-6m3e): update_artifact returns details {exhibit:'artifact_saved', action:'updated', artifactId, title} -> Session.noteArtifactSaved (internal/agent/agent.go) broadcasts exhibit_artifact_saved over SSE -> agent.js dispatches 'exhibit:artifact-saved' on body -> htmx (hx-trigger on #pane-preview) re-fetches GET /partials/agent-preview and swaps the fragment with a fresh cache-busted iframe.

Root cause: internal/agent/ext/exhibit.ts:133-137 — update_artifact reads a.id / a.title off the PATCH response root, but PATCH /api/artifacts/:id returns updateArtifactResponse (internal/api/artifacts.go:443-447), which nests the artifact under 'artifact'. So a.id/a.title are undefined: JSON.stringify drops the undefined details keys, the Go hook's type-asserted details['artifactId'] is '' and noteArtifactSaved early-returns (agent.go:522-524) — no SSE event, no refresh. (create_artifact reads r.artifact.id correctly; get_artifact is unaffected because GET /api/artifacts/:id?body=true flattens via an embedded *store.Artifact.)

## Acceptance Criteria

After a successful update_artifact call in the agent view, the preview pane re-renders from the server fragment without a full page reload (iframe src carries a fresh ?r= stamp and shows the new body). The tool result text reads 'Updated artifact <id> ("<title>").' with real values. The exhibit_artifact_saved SSE event is emitted for action 'updated' and the htmx swap fires exactly once per save. create_artifact still works. Regression check: modify-mode session (?artifact=<id>) where the first save is an update_artifact still binds the session and refreshes the pane.

